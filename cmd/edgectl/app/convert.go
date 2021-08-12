/*
Copyright 2021 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	openyurtv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/constants"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

const (
	kubeletConfigRegularExpression      = "\\-\\-kubeconfig=.*kubelet\\.conf"
	kubeletHealthzPortRegularExpression = "\\-\\-healthz\\-port=[0-9]+"
	clientCARegularExpression           = "\\-\\-client\\-ca\\-file=.*ca\\.crt"

	fileMode = 0666
	dirMode  = 0755

	defaultHealthzCheckFrequency = 5 * time.Second

	kubeadmFlagsEnvBackupFileName = "kubeadm-flags.env.backup"
)

// convertOptions has the information required by sub command convert node to edge
type convertOptions struct {
	nodeName           string
	yurtDir            string
	kubePKIDir         string
	kubeletConfigPath  string
	apiServerAddress   string
	kubeletHealthzPort int
	force              bool
	healthCheckTimeout time.Duration
}

func newDefaultConvertOptions() *convertOptions {
	hostname, _ := os.Hostname()
	return &convertOptions{
		nodeName:           hostname,
		yurtDir:            "/var/lib/openyurt",
		kubePKIDir:         "/etc/kubernetes/pki",
		kubeletConfigPath:  "/etc/kubernetes/kubelet.conf",
		kubeletHealthzPort: 10248,
		healthCheckTimeout: time.Minute * 5,
	}
}

func addConvertFlags(flagSet *flag.FlagSet, opt *convertOptions) {
	flagSet.StringVar(&opt.nodeName, "node-name", opt.nodeName, "The node name of the current kubernetes node.")
	flagSet.StringVar(&opt.yurtDir, "yurt-dir", opt.yurtDir, "The directory on edge node containing yurt config files.")
	flagSet.StringVar(&opt.kubePKIDir, "pki-dir", opt.kubePKIDir, "The directory on edge node containing pki files.")
	flagSet.StringVar(&opt.kubeletConfigPath, "kubelet-config-path", opt.kubeletConfigPath, "The path to kubelet kubeconfig file.")
	flagSet.StringVar(&opt.apiServerAddress, "apiserver-address", opt.apiServerAddress, "The kubernetes api server to connect.")
	flagSet.IntVar(&opt.kubeletHealthzPort, "kubelet-healthz-port", opt.kubeletHealthzPort, "The port for kubelet healthz check.")
	flagSet.DurationVar(&opt.healthCheckTimeout, "health-check-timeout", opt.healthCheckTimeout, "The timeout to check kubelet/edge-hub health status.")
	flagSet.BoolVar(&opt.force, "force", opt.force, "Force to run convert even node is not ready")
}

func newCmdConvert(options *convertOptions) *cobra.Command {
	if options == nil {
		options = newDefaultConvertOptions()
	}

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert node to edge node",
		RunE: func(cmd *cobra.Command, args []string) error {
			// init client
			restConfig, err := kclient.GetConfig(util.GetAPIServerAddress(options.apiServerAddress))
			if err != nil {
				return err
			}
			kclient.InitializeKubeClient(restConfig)

			return options.runConvert(context.Background())
		},
	}

	addConvertFlags(cmd.Flags(), options)

	return cmd
}

func (opt *convertOptions) runConvert(ctx context.Context) (reterr error) {
	// get YurtCluster instance
	yurtCluster, err := util.LoadYurtCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to load YurtCluster instance from cluster")
	}

	// Initialize the patch helper.
	patchHelper, err := patcher.NewHelper(yurtCluster, kclient.CtlClient())
	if err != nil {
		return err
	}

	defer func() {
		patchOpts := []patcher.Option{}
		if err := patchHelper.Patch(ctx, yurtCluster, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// update YurtCluster node condition based on the convert result
	defer func() {
		nodeConvertCondition := openyurtv1alpha1.NodeConvertCondition{
			Status:  "True",
			Reason:  "NodeConvert",
			Message: "Node was converted to edge successfully",
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
			AckLastUpdateTime: yurtCluster.Status.LastUpdateTime,
		}
		if reterr != nil {
			nodeConvertCondition = openyurtv1alpha1.NodeConvertCondition{
				Status:  "False",
				Reason:  "NodeConvert",
				Message: reterr.Error(),
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
				AckLastUpdateTime: yurtCluster.Status.LastUpdateTime,
			}
		}
		setNodeConvertCondition(yurtCluster, opt.nodeName, nodeConvertCondition)
	}()

	// get local node from cluster
	node := &corev1.Node{}
	key := types.NamespacedName{Name: opt.nodeName}
	err = kclient.CtlClient().Get(ctx, key, node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get node %q from cluster", opt.nodeName)
	}

	// skip if node not ready
	if !opt.force && !util.IsNodeReady(node) {
		return errors.Errorf("skip convert node %v to edge, because it is not ready now", opt.nodeName)
	}

	manifestsTemplate, err := util.LoadManifestsTemplate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to load operator manifests template")
	}

	// prepare kubelet cmdLine
	cmdLine, err := getKubeletCmdLine()
	if err != nil {
		return errors.Wrap(err, "failed to get cmd line of kubelet")
	}

	if err := opt.setupYurtHub(ctx, manifestsTemplate, cmdLine); err != nil {
		return errors.Wrap(err, "failed to setup yurt-hub")
	}
	if err := opt.convertKubelet(manifestsTemplate, cmdLine); err != nil {
		return errors.Wrap(err, "failed to setup kubelet")
	}

	// restart local pods
	if yurtCluster.Spec.YurtHub.AutoRestartNodePod != nil && *yurtCluster.Spec.YurtHub.AutoRestartNodePod {
		if err := restartLocalPods(ctx, true); err != nil {
			klog.Warningf("post restart containers with error %v", err)
		}
	}

	klog.Infof("convert node %v as edge node successfully", opt.nodeName)
	return nil
}

func (opt *convertOptions) setupYurtHub(ctx context.Context, manifestsTemplate *corev1.ConfigMap, cmdLine string) (reterr error) {
	// parse kubelet.conf path from kubelet cmdline
	kubeletConfPath, err := util.GetSingleContentPreferLastMatchFromString(cmdLine, kubeletConfigRegularExpression)
	if err == nil {
		args := strings.Split(kubeletConfPath, "=")
		if len(args) == 2 {
			klog.Infof("detected kubelet config file located at %q", args[1])
			kubeletConfPath = args[1]
		} else {
			klog.Warningf("failed to split --kubeconfig arg from cmdLine %v, current flag %q, fall back to %v", cmdLine, kubeletConfPath, opt.kubeletConfigPath)
			kubeletConfPath = opt.kubeletConfigPath
		}
	} else {
		klog.Warningf("failed to find --kubeconfig arg from cmdLine %v, fall back to %v", cmdLine, opt.kubeletConfigPath)
		kubeletConfPath = opt.kubeletConfigPath
	}

	apiServerAddr, err := util.ParseAPIServerAddressFromKubeConfigFile(kubeletConfPath)
	if err != nil {
		return errors.Errorf("failed to parse API Server address from kubelet kubeconfig file")
	}
	klog.Infof("using APIServer address: %v", apiServerAddr)

	// parse pki dir from kubelet cmdline
	pkiDir, err := parsePkiDir(cmdLine)
	if err != nil {
		klog.Warningf("failed to parse --client-ca-file param from cmdline %v, fallback to %v", cmdLine, opt.kubePKIDir)
		pkiDir = opt.kubePKIDir
	}

	// get YurtCluster configuration
	yurtCluster, err := util.LoadYurtCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to load YurtCluster instance from cluster")
	}

	// prepare values and template
	values := map[string]string{
		"apiServerAddress":       apiServerAddr,
		"enableResourceFilter":   strconv.FormatBool(*yurtCluster.Spec.YurtHub.EnableResourceFilter),
		"accessServerThroughHub": strconv.FormatBool(*yurtCluster.Spec.YurtHub.AccessServerThroughHub),
		"kubePkiDir":             pkiDir,
		"yurtHubImage":           util.GetYurtComponentImageByName(yurtCluster, "yurthub"),
	}

	yurtHubPodTemplate, ok := manifestsTemplate.Data[constants.YurtHubPodKey]
	if !ok {
		return errors.Errorf("failed to find key %v from operator manifests template %v", constants.YurtHubPodKey, klog.KObj(manifestsTemplate))
	}

	// render yurthub template with values
	yurtHubYaml, err := util.RenderTemplate(yurtHubPodTemplate, values)
	if err != nil {
		return errors.Wrap(err, "failed to render yurt-hub manifests")
	}

	// diff with local yurt-hub yaml
	// we assume the manifests dir is the parent dir of pkiDir
	yurtHubManifestPath := filepath.Join(filepath.Dir(pkiDir), "manifests", "yurt-hub.yaml")
	backupYurtHubFilePath := filepath.Join(opt.yurtDir, "yurt-hub.yaml.backup")
	if exists, err := util.FileExists(yurtHubManifestPath); err == nil && exists {
		existsYurtHubManifests, err := ioutil.ReadFile(yurtHubManifestPath)
		if err == nil && yurtHubYaml == string(existsYurtHubManifests) {
			klog.Info("skip create yurt-hub yaml, no changes")
			return nil
		}
		// backup old yurt-hub manifests before overwrite
		err = util.CopyFile(yurtHubManifestPath, backupYurtHubFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to backup yurt-hub.yaml")
		}
		klog.Infof("backup yurt-hub.yaml in %v", backupYurtHubFilePath)
	}

	defer func() {
		if reterr != nil {
			klog.Warningf("yurt-hub failed to start, clean rubbish...")
			os.RemoveAll(yurtHubManifestPath)
			// restore old yurt-hub if available
			if exists, err := util.FileExists(backupYurtHubFilePath); err == nil && exists {
				err = util.CopyFile(backupYurtHubFilePath, yurtHubManifestPath)
				if err != nil {
					klog.Errorf("failed to restore yurt-hub.yaml %v", err)
					return
				}
				klog.Info("restore yurt-hub.yaml successfully")
			}
		}
	}()

	err = ioutil.WriteFile(yurtHubManifestPath, []byte(yurtHubYaml), fileMode)
	if err != nil {
		return errors.Wrapf(err, "failed to write yurt-hub yaml into %v", yurtHubManifestPath)
	}

	klog.Infof("create the %s", yurtHubManifestPath)

	// wait yurt-hub to be ready
	if err := waitYurtHubReady(opt.healthCheckTimeout); err != nil {
		return errors.Wrapf(err, "yurt-hub is not ready within %v", opt.healthCheckTimeout)
	}

	return nil
}

// convertKubelet sets kubelet to connect apiserver via yurthub
func (opt *convertOptions) convertKubelet(manifestsTemplate *corev1.ConfigMap, cmdLine string) (reterr error) {
	kubeletHealthzPort, err := parseKubeletHealthzPort(cmdLine)
	if err != nil {
		klog.Warningf("failed to parse --healthz-port param from cmdline %v, fallback to %v", cmdLine, opt.kubeletHealthzPort)
		kubeletHealthzPort = opt.kubeletHealthzPort
	}

	// generate openyurt kubelet config file
	yurtKubeletConfigContent, ok := manifestsTemplate.Data[constants.YurtKubeletConfigKey]
	if !ok {
		return errors.Errorf("failed to find key %v from operator manifests template %v", constants.YurtKubeletConfigKey, klog.KObj(manifestsTemplate))
	}
	err = os.MkdirAll(opt.yurtDir, dirMode)
	if err != nil {
		return errors.Wrapf(err, "failed to create yurt dir %v", opt.yurtDir)
	}
	kubeletConfigForYurt := filepath.Join(opt.yurtDir, "kubelet.conf")
	err = ioutil.WriteFile(kubeletConfigForYurt, []byte(yurtKubeletConfigContent), fileMode)
	if err != nil {
		return err
	}
	klog.Infof("write kubelet.conf in %v", kubeletConfigForYurt)

	// backup origin kubeadm-flags.env file
	originKubeadmFlagsEnvFilePath := "/var/lib/kubelet/kubeadm-flags.env"
	exists, err := util.FileExists(originKubeadmFlagsEnvFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to check and backup kubeadm-flags.env file")
	}

	backupKubeadmFlagsEnvFilePath := filepath.Join(opt.yurtDir, kubeadmFlagsEnvBackupFileName)
	if exists {
		if exists, _ := util.FileExists(backupKubeadmFlagsEnvFilePath); exists {
			klog.Infof("backup kubeadm-flags.env already exists, skip backup")
		} else {
			err = util.CopyFile(originKubeadmFlagsEnvFilePath, backupKubeadmFlagsEnvFilePath)
			if err != nil {
				return errors.Wrap(err, "failed to backup kubeadm-flags.env")
			}
			klog.Infof("backup kubeadm-flags.env in %v", backupKubeadmFlagsEnvFilePath)
		}
	} else {
		klog.Infof("origin kubeadm-flags.env not found, skip backup process")
	}

	defer func() {
		if reterr != nil {
			// using backup config
			klog.Infof("error encountered, rollback using old kubelet configuration")
			exists, err := util.FileExists(backupKubeadmFlagsEnvFilePath)
			if err == nil && exists {
				err = util.CopyFile(backupKubeadmFlagsEnvFilePath, originKubeadmFlagsEnvFilePath)
				if err != nil {
					klog.Errorf("failed to rollback kubeadm-flags.env for kubelet, %v", err)
					return
				}
				klog.Infof("rollback to old kubelet config successfully")
				if err := util.RestartService("kubelet"); err != nil {
					klog.Errorf("failed to restart kubelet after rollback config file, %v", err)
					return
				}
				klog.Infof("restart kubelet service successfully")
				if err := waitKubeletReady(kubeletHealthzPort, opt.healthCheckTimeout); err != nil {
					klog.Errorf("failed to wait kubelet become ready, %v", err)
				}
				klog.Infof("kubelet service is ready now")
			}
		}
	}()

	// update kubeadm-flags.env to use openyurt kubelet.conf
	kubeadmFlagsEnvContent := fmt.Sprintf(`KUBELET_KUBEADM_ARGS="--bootstrap-kubeconfig= --kubeconfig=%s"`, kubeletConfigForYurt)
	if !exists {
		err = ioutil.WriteFile(originKubeadmFlagsEnvFilePath, []byte(kubeadmFlagsEnvContent), fileMode)
		if err != nil {
			return errors.Wrap(err, "failed to overwrite kubeadm-flags.env")
		}
	} else {
		curContent, err := ioutil.ReadFile(originKubeadmFlagsEnvFilePath)
		if err != nil {
			return errors.Wrap(err, "cannot read from kubeadm-flags.env file")
		}
		if !strings.Contains(string(curContent), "KUBELET_KUBEADM_ARGS") {
			newContent := kubeadmFlagsEnvContent
			if len(curContent) > 0 {
				newContent = fmt.Sprintf("%s\n%s", string(curContent), kubeadmFlagsEnvContent)
			}
			err = ioutil.WriteFile(originKubeadmFlagsEnvFilePath, []byte(newContent), fileMode)
			if err != nil {
				return errors.Wrap(err, "failed to overwrite kubeadm-flags.env")
			}
		} else {
			lines := strings.Split(string(curContent), "\n")
			for i, line := range lines {
				if strings.Contains(line, "KUBELET_KUBEADM_ARGS") {
					pruneFlagsStr := strings.ReplaceAll(line, "KUBELET_KUBEADM_ARGS=", "")
					pruneFlagsStr = strings.Trim(pruneFlagsStr, "\"")
					pruneFlags := strings.Split(pruneFlagsStr, " ")
					argList := []string{}
					isSetKubeconfigFlag := false
					for _, flag := range pruneFlags {
						keyValue := strings.Split(flag, "=")
						if len(keyValue) == 0 {
							continue
						}
						if keyValue[0] == "--bootstrap-kubeconfig" {
							continue
						}
						if keyValue[0] == "--kubeconfig" {
							argList = append(argList, fmt.Sprintf("--kubeconfig=%s", kubeletConfigForYurt))
							isSetKubeconfigFlag = true
							continue
						}
						argList = append(argList, flag)
					}

					argList = append(argList, "--bootstrap-kubeconfig=") // always add this to turn off bootstrap

					if !isSetKubeconfigFlag {
						argList = append(argList, fmt.Sprintf("--kubeconfig=%s", kubeletConfigForYurt))
					}
					lines[i] = fmt.Sprintf(`KUBELET_KUBEADM_ARGS="%s"`, strings.Join(argList, " "))
				}
			}

			// write lines
			newContent := strings.Join(lines, "\n")
			if newContent == string(curContent) {
				klog.Info("skip update kubeadm-flags.env file, no changes")
				return nil
			}
			err = ioutil.WriteFile(originKubeadmFlagsEnvFilePath, []byte(newContent), fileMode)
			if err != nil {
				return errors.Wrap(err, "failed to overwrite kubeadm-flags.env")
			}
		}
	}

	// restart kubelet
	if err := util.RestartService("kubelet"); err != nil {
		return errors.Wrap(err, "failed to restart kubelet service")
	}

	// wait for kubelet ready
	return waitKubeletReady(kubeletHealthzPort, opt.healthCheckTimeout)
}

// waitYurtHubReady waits yurt-hub to be ready
func waitYurtHubReady(timeout time.Duration) error {
	serverHealthzURL, err := url.Parse("http://127.0.0.1:10267")
	if err != nil {
		return err
	}
	serverHealthzURL.Path = "/v1/healthz"

	start := time.Now()
	return wait.PollImmediate(defaultHealthzCheckFrequency, timeout, func() (bool, error) {
		_, err := checkHealthz(http.DefaultClient, serverHealthzURL.String())
		if err != nil {
			klog.Infof("yurt-hub is not ready, ping cluster healthz with result: %v", err)
			return false, nil
		}
		klog.Infof("yurt-hub healthz is OK after %f seconds", time.Since(start).Seconds())
		return true, nil
	})
}

// waitKubeletReady waits kubelet to be ready
func waitKubeletReady(port int, timeout time.Duration) error {
	serverHealthzURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	serverHealthzURL.Path = "/healthz"

	start := time.Now()
	return wait.PollImmediate(defaultHealthzCheckFrequency, timeout, func() (bool, error) {
		_, err := checkHealthz(http.DefaultClient, serverHealthzURL.String())
		if err != nil {
			klog.Infof("kubelet is not ready, ping healthz with result: %v", err)
			return false, nil
		}
		klog.Infof("kubelet healthz is OK after %f seconds", time.Since(start).Seconds())
		return true, nil
	})
}

func checkHealthz(client *http.Client, addr string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("http client is invalid")
	}
	resp, err := client.Get(addr)
	if err != nil {
		return false, err
	}
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("failed to read response of cluster healthz, %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status code is %d", resp.StatusCode)
	}
	if strings.ToLower(string(b)) != "ok" {
		return false, fmt.Errorf("cluster healthz is %s", string(b))
	}
	return true, nil
}

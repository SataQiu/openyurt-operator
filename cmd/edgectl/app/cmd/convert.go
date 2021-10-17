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

package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/edgectl/app/options"
	apputil "github.com/openyurtio/openyurt-operator/cmd/edgectl/app/util"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/templates"
	"github.com/openyurtio/openyurt-operator/pkg/templates/yurthub"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

const (
	fileMode = 0666
	dirMode  = 0755

	yurtHubYamlName       = "yurt-hub.yaml"
	yurtHubYamlBackupName = "yurt-hub.yaml.backup"

	yurtKubeletConfigName = "kubelet.conf"

	kubeadmFlagsEnvFilePath   = "/var/lib/kubelet/kubeadm-flags.env"
	kubeadmFlagsEnvBackupName = "kubeadm-flags.env.backup"

	defaultHealthCheckFrequency = 1 * time.Second
)

// NewCmdConvert returns *cobra.Command for node convert
func NewCmdConvert() *cobra.Command {
	opt := options.NewConvertOptions()

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert node to cloud/edge node",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opt.Validate(); err != nil {
				return err
			}

			// init client
			restConfig, err := kclient.GetConfigWithAPIServerAddress(opt.APIServerAddress)
			if err != nil {
				return err
			}
			kclient.InitializeKubeClient(restConfig)

			return runConvert(context.Background(), opt)
		},
	}

	opt.AddAllFlags(cmd.Flags())
	return cmd
}

func runConvert(ctx context.Context, opt *options.ConvertOptions) (reterr error) {
	yurtCluster := &operatorv1alpha1.YurtCluster{}
	namespacedName := types.NamespacedName{Name: operatorv1alpha1.SingletonYurtClusterInstanceName}
	err := kclient.CtlClient().Get(ctx, namespacedName, yurtCluster)
	if err != nil {
		return err
	}

	// initialize the patch helper.
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
		nodeConvertCondition := operatorv1alpha1.NodeCondition{
			Status:  "True",
			Reason:  getConvertReasonByType(opt.NodeType),
			Message: fmt.Sprintf("Node was converted into %v successfully", opt.NodeType),
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
			ObservedGeneration: yurtCluster.Generation,
		}
		if reterr != nil {
			nodeConvertCondition = operatorv1alpha1.NodeCondition{
				Status:  "False",
				Reason:  getConvertReasonByType(opt.NodeType),
				Message: reterr.Error(),
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			}
		}
		apputil.UpdateNodeCondition(yurtCluster, opt.NodeName, nodeConvertCondition)
	}()

	// skip if yurt-hub is not enabled
	if !isYurtHubEnabled(opt.NodeType, yurtCluster) {
		klog.Infof("the YurtHub is not enabled on this node, skip convert")
		return nil
	}

	// get local node from cluster
	node := &corev1.Node{}
	namespacedName = types.NamespacedName{Name: opt.NodeName}
	err = kclient.CtlClient().Get(ctx, namespacedName, node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get node %q from cluster", opt.NodeName)
	}

	// skip if node not ready
	if !opt.Force && !util.IsNodeReady(node) {
		return errors.Errorf("skip convert node %v to %v, because it is not ready now", opt.NodeType, opt.NodeName)
	}

	// fetch latest yurt-hub template
	tpl, err := templates.LoadTemplate(ctx, yurthub.NamespacedName)
	if err != nil {
		return err
	}

	kubeletConfiguration, err := apputil.LoadKubeletConfiguration(&opt.BaseOptions)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opt.YurtWorkDir, dirMode); err != nil {
		return errors.Wrapf(err, "failed to create yurt work dir %v", opt.YurtWorkDir)
	}

	if err := setupYurtHub(yurtCluster, tpl, kubeletConfiguration, opt.YurtWorkDir, opt.NodeType, opt.HealthCheckTimeout); err != nil {
		return errors.Wrap(err, "failed to setup yurt-hub")
	}

	if err := setupKubelet(yurtCluster, tpl, kubeletConfiguration, opt.YurtWorkDir, opt.HealthCheckTimeout); err != nil {
		return errors.Wrap(err, "failed to setup kubelet")
	}

	klog.Infof("convert node %v as %v successfully", opt.NodeName, opt.NodeType)
	return nil
}

func isYurtHubEnabled(nodeType string, yurtCluster *operatorv1alpha1.YurtCluster) bool {
	if nodeType == string(operatorv1alpha1.CloudNode) && *yurtCluster.Spec.YurtHub.Cloud.Enabled {
		return true
	}

	if nodeType == string(operatorv1alpha1.EdgeNode) && *yurtCluster.Spec.YurtHub.Edge.Enabled {
		return true
	}

	return false
}

func getConvertReasonByType(nodeType string) string {
	reason := operatorv1alpha1.NodeConditionReasonCloudNodeConvert
	if nodeType == string(operatorv1alpha1.EdgeNode) {
		reason = operatorv1alpha1.NodeConditionReasonEdgeNodeConvert
	}
	return reason
}

func setupYurtHub(yurtCluster *operatorv1alpha1.YurtCluster,
	tpl *corev1.ConfigMap,
	kubeletConfiguration *apputil.KubeletConfiguration,
	yurtWorkDir, nodeType string,
	healthCheckTimeout time.Duration) (reterr error) {
	values := map[string]string{
		"image":            util.GetYurtComponentImageByName(yurtCluster, "yurthub"),
		"apiServerAddress": kubeletConfiguration.APIServerAddress,
	}

	tplKey := yurthub.StaticCloudPod
	if nodeType == string(operatorv1alpha1.EdgeNode) {
		tplKey = yurthub.StaticEdgePod
	}

	yurtHubPodTemplate, ok := tpl.Data[tplKey]
	if !ok {
		return errors.Errorf("failed to find key %v from template %v", tplKey, klog.KObj(tpl))
	}

	// render yurt-hub template with values
	yurtHubFileContent, err := util.RenderTemplate(yurtHubPodTemplate, values)
	if err != nil {
		return errors.Wrap(err, "failed to render yurt-hub manifests")
	}

	// diff with local yurt-hub yaml
	yurtHubFilePath := filepath.Join(kubeletConfiguration.PodManifestsPath, yurtHubYamlName)
	yurtHubBackupFilePath := filepath.Join(yurtWorkDir, yurtHubYamlBackupName)
	if exists, err := util.FileExists(yurtHubFilePath); err == nil && exists {
		existsYurtHubFileContent, err := ioutil.ReadFile(yurtHubFilePath)
		if err == nil && yurtHubFileContent == string(existsYurtHubFileContent) {
			klog.Info("skip create yurt-hub yaml, no changes")
			// wait yurt-hub to be ready
			if err := waitYurtHubReady(healthCheckTimeout); err != nil {
				return errors.Wrapf(err, "yurt-hub is not ready within %v", healthCheckTimeout)
			}
			return nil
		}
		// backup old yurt-hub manifests before overwrite
		err = util.CopyFile(yurtHubFilePath, yurtHubBackupFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to backup yurt-hub.yaml")
		}
		klog.Infof("backup yurt-hub.yaml in %v", yurtHubBackupFilePath)
	}

	defer func() {
		if reterr != nil {
			klog.Warningf("yurt-hub failed to start, clean rubbish...")
			os.RemoveAll(yurtHubFilePath)
			// restore old yurt-hub if available
			if exists, err := util.FileExists(yurtHubBackupFilePath); err == nil && exists {
				err = util.CopyFile(yurtHubBackupFilePath, yurtHubFilePath)
				if err != nil {
					klog.Errorf("failed to restore yurt-hub.yaml %v", err)
					return
				}
				klog.Info("restore yurt-hub.yaml successfully")
			}
		}
	}()

	err = ioutil.WriteFile(yurtHubFilePath, []byte(yurtHubFileContent), fileMode)
	if err != nil {
		return errors.Wrapf(err, "failed to write yurt-hub yaml into %v", yurtHubFilePath)
	}

	klog.Infof("create the %s", yurtHubFilePath)

	// wait yurt-hub to be ready
	if err := waitYurtHubReady(healthCheckTimeout); err != nil {
		return errors.Wrapf(err, "yurt-hub is not ready within %v", healthCheckTimeout)
	}

	return nil
}

func setupKubelet(yurtCluster *operatorv1alpha1.YurtCluster,
	tpl *corev1.ConfigMap,
	kubeletConfiguration *apputil.KubeletConfiguration,
	yurtWorkDir string,
	healthCheckTimeout time.Duration) (reterr error) {
	yurtKubeletConfigContent, ok := tpl.Data[yurthub.YurtKubeletConfig]
	if !ok {
		return errors.Errorf("failed to find key %v from template %q", yurthub.YurtKubeletConfig, klog.KObj(tpl))
	}

	yurtKubeletConfigFilePath := filepath.Join(yurtWorkDir, yurtKubeletConfigName)
	if err := ioutil.WriteFile(yurtKubeletConfigFilePath, []byte(yurtKubeletConfigContent), fileMode); err != nil {
		return err
	}

	klog.Infof("write kubelet.conf in %v", yurtKubeletConfigFilePath)

	kubeadmFlagsEnvBackupFilePath := filepath.Join(yurtWorkDir, kubeadmFlagsEnvBackupName)

	// backup origin kubeadm-flags.env file
	exists, err := util.FileExists(kubeadmFlagsEnvFilePath)
	if err == nil && exists {
		if exists, _ := util.FileExists(kubeadmFlagsEnvBackupFilePath); exists {
			klog.Infof("backup kubeadm-flags.env already exists, skip backup")
		} else {
			err = util.CopyFile(kubeadmFlagsEnvFilePath, kubeadmFlagsEnvBackupFilePath)
			if err != nil {
				return errors.Wrap(err, "failed to backup kubeadm-flags.env")
			}
			klog.Infof("backup kubeadm-flags.env in %v", kubeadmFlagsEnvBackupFilePath)
		}
	}

	defer func() {
		if reterr != nil {
			// using backup config
			klog.Infof("error encountered, rollback using old kubelet configuration")
			if exists, err := util.FileExists(kubeadmFlagsEnvBackupFilePath); err == nil && exists {
				err = util.CopyFile(kubeadmFlagsEnvBackupFilePath, kubeadmFlagsEnvFilePath)
				if err != nil {
					klog.Errorf("failed to rollback kubeadm-flags.env for kubelet, %v", err)
					return
				}
				klog.Infof("rollback to old kubelet config successfully")
			} else {
				os.RemoveAll(kubeadmFlagsEnvFilePath)
			}

			if err := util.RestartService("kubelet"); err != nil {
				klog.Errorf("failed to restart kubelet after rollback config file, %v", err)
				return
			}
			klog.Infof("restart kubelet service successfully")
			if err := waitKubeletReady(kubeletConfiguration.HealthPort, healthCheckTimeout); err != nil {
				klog.Errorf("failed to wait kubelet become ready, %v", err)
			}
			klog.Infof("kubelet service is ready now")
		}
	}()

	// update kubeadm-flags.env to use openyurt kubelet.conf
	kubeadmFlagsEnvContent := fmt.Sprintf(`KUBELET_KUBEADM_ARGS="--bootstrap-kubeconfig= --kubeconfig=%s"`, yurtKubeletConfigFilePath)
	if !exists {
		err = ioutil.WriteFile(kubeadmFlagsEnvFilePath, []byte(kubeadmFlagsEnvContent), fileMode)
		if err != nil {
			return errors.Wrap(err, "failed to create kubeadm-flags.env")
		}
	} else {
		curContent, err := ioutil.ReadFile(kubeadmFlagsEnvFilePath)
		if err != nil {
			return errors.Wrap(err, "cannot read from kubeadm-flags.env file")
		}
		if !strings.Contains(string(curContent), "KUBELET_KUBEADM_ARGS") {
			newContent := kubeadmFlagsEnvContent
			if len(curContent) > 0 {
				newContent = fmt.Sprintf("%s\n%s", string(curContent), kubeadmFlagsEnvContent)
			}
			err = ioutil.WriteFile(kubeadmFlagsEnvFilePath, []byte(newContent), fileMode)
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
							argList = append(argList, fmt.Sprintf("--kubeconfig=%s", yurtKubeletConfigFilePath))
							isSetKubeconfigFlag = true
							continue
						}
						argList = append(argList, flag)
					}

					argList = append(argList, "--bootstrap-kubeconfig=")

					if !isSetKubeconfigFlag {
						argList = append(argList, fmt.Sprintf("--kubeconfig=%s", yurtKubeletConfigFilePath))
					}
					lines[i] = fmt.Sprintf(`KUBELET_KUBEADM_ARGS="%s"`, strings.Join(argList, " "))
				}
			}

			// write lines
			newContent := strings.Join(lines, "\n")
			if newContent == string(curContent) {
				klog.Info("skip update kubeadm-flags.env file, no changes")
				return waitKubeletReady(kubeletConfiguration.HealthPort, healthCheckTimeout)
			}
			err = ioutil.WriteFile(kubeadmFlagsEnvFilePath, []byte(newContent), fileMode)
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
	return waitKubeletReady(kubeletConfiguration.HealthPort, healthCheckTimeout)
}

// waitYurtHubReady waits yurt-hub to be ready
func waitYurtHubReady(timeout time.Duration) error {
	serverHealthzURL, err := url.Parse("http://127.0.0.1:10267")
	if err != nil {
		return err
	}
	serverHealthzURL.Path = "/v1/healthz"

	start := time.Now()
	return wait.PollImmediate(defaultHealthCheckFrequency, timeout, func() (bool, error) {
		_, err := checkHealth(http.DefaultClient, serverHealthzURL.String())
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
	return wait.PollImmediate(defaultHealthCheckFrequency, timeout, func() (bool, error) {
		_, err := checkHealth(http.DefaultClient, serverHealthzURL.String())
		if err != nil {
			klog.Infof("kubelet is not ready, ping healthz with result: %v", err)
			return false, nil
		}
		klog.Infof("kubelet healthz is OK after %f seconds", time.Since(start).Seconds())
		return true, nil
	})
}

func checkHealth(client *http.Client, addr string) (bool, error) {
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

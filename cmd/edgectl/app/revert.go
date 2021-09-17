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
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	openyurtv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/cri"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

// revertOptions has the information required by sub command revert node to normal
type revertOptions struct {
	nodeName           string
	yurtDir            string
	kubePKIDir         string
	kubeletConfigPath  string
	apiServerAddress   string
	kubeletHealthzPort int
	force              bool
	healthCheckTimeout time.Duration
}

func newDefaultRevertOptions() *revertOptions {
	hostname, _ := os.Hostname()
	return &revertOptions{
		nodeName:           hostname,
		yurtDir:            "/var/lib/openyurt",
		kubePKIDir:         "/etc/kubernetes/pki",
		kubeletConfigPath:  "/etc/kubernetes/kubelet.conf",
		kubeletHealthzPort: 10248,
		healthCheckTimeout: time.Minute * 5,
	}
}

func addRevertFlags(flagSet *flag.FlagSet, opt *revertOptions) {
	flagSet.StringVar(&opt.nodeName, "node-name", opt.nodeName, "The node name of the current kubernetes node.")
	flagSet.StringVar(&opt.yurtDir, "yurt-dir", opt.yurtDir, "The directory on edge node containing yurt config files.")
	flagSet.StringVar(&opt.kubePKIDir, "pki-dir", opt.kubePKIDir, "The directory on edge node containing pki files.")
	flagSet.StringVar(&opt.kubeletConfigPath, "kubelet-config-path", opt.kubeletConfigPath, "The path to kubelet kubeconfig file.")
	flagSet.StringVar(&opt.apiServerAddress, "apiserver-address", opt.apiServerAddress, "The kubernetes api server to connect.")
	flagSet.IntVar(&opt.kubeletHealthzPort, "kubelet-healthz-port", opt.kubeletHealthzPort, "The port for kubelet healthz check.")
	flagSet.DurationVar(&opt.healthCheckTimeout, "health-check-timeout", opt.healthCheckTimeout, "The timeout to check kubelet/edge-hub health status.")
	flagSet.BoolVar(&opt.force, "force", opt.force, "Force to run revert even node is not ready")
}

func newCmdRevert(options *revertOptions) *cobra.Command {
	if options == nil {
		options = newDefaultRevertOptions()
	}

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Revert node to normal node",
		RunE: func(cmd *cobra.Command, args []string) error {
			// init client
			restConfig, err := kclient.GetConfig(util.GetAPIServerAddress(options.apiServerAddress))
			if err != nil {
				return err
			}
			kclient.InitializeKubeClient(restConfig)

			return options.runRevert(context.Background())
		},
	}

	addRevertFlags(cmd.Flags(), options)

	return cmd
}

func (opt *revertOptions) runRevert(ctx context.Context) (reterr error) {
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

	// update YurtCluster node condition based on the revert result
	defer func() {
		nodeConvertCondition := openyurtv1alpha1.NodeConvertCondition{
			Status:  "True",
			Reason:  "NodeRevert",
			Message: "Node was reverted to normal successfully",
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
			AckLastUpdateTime: yurtCluster.Status.LastUpdateTime,
		}
		if reterr != nil {
			nodeConvertCondition = openyurtv1alpha1.NodeConvertCondition{
				Status:  "False",
				Reason:  "NodeRevert",
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
		return errors.Errorf("skip revert node %v to edge, because it is not ready now", opt.nodeName)
	}

	// prepare kubelet cmdLine
	cmdLine, err := getKubeletCmdLine()
	if err != nil {
		return errors.Wrap(err, "failed to get cmd line of kubelet")
	}

	if err := opt.revertKubelet(cmdLine); err != nil {
		return errors.Wrap(err, "failed to revert kubelet configuration")
	}

	if err := opt.removeYurtHub(cmdLine); err != nil {
		return errors.Wrap(err, "failed to remove yurt-hub")
	}

	klog.Info("run tunnel server iptables rules cleanup")
	if out, err := util.CleanTunnelServerIPTablesRules(); err != nil {
		return errors.Wrapf(err, "failed to clean tunnel server iptables rules, output: %v", out)
	}

	// restart local pods
	if yurtCluster.Spec.YurtHub.AutoRestartNodePod != nil && *yurtCluster.Spec.YurtHub.AutoRestartNodePod {
		if err := cri.StopAllReadyPodsExceptYurt(ctx); err != nil {
			klog.Warningf("post stop Pods with error %v", err)
		}
	}

	klog.Infof("revert node %v as normal node successfully", opt.nodeName)
	return nil
}

// revertKubelet reverts kubelet to connect apiserver directly
func (opt *revertOptions) revertKubelet(cmdLine string) (reterr error) {
	kubeletHealthzPort, err := parseKubeletHealthzPort(cmdLine)
	if err != nil {
		klog.Warningf("failed to parse --healthz-port param from cmdline %v, fallback to %v", cmdLine, opt.kubeletHealthzPort)
		kubeletHealthzPort = opt.kubeletHealthzPort
	}

	backupKubeadmFlagsEnvFilePath := filepath.Join(opt.yurtDir, kubeadmFlagsEnvBackupFileName)
	if exists, err := util.FileExists(backupKubeadmFlagsEnvFilePath); err != nil || !exists {
		klog.Warning("skp kubelet revert, no backup kubeadm-flags.env found")
		return nil
	}

	// tmp backup current kubelet kubeadm-flags.env file
	originKubeadmFlagsEnvFilePath := "/var/lib/kubelet/kubeadm-flags.env"
	tmpBackupKubeadmFlagsEnvFilePath := filepath.Join(opt.yurtDir, "kubeadm-flags.env.revert.tmp")
	err = util.CopyFile(originKubeadmFlagsEnvFilePath, tmpBackupKubeadmFlagsEnvFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to backup current kubeadm-flags.env")
	}
	defer func() {
		os.RemoveAll(tmpBackupKubeadmFlagsEnvFilePath)
	}()

	defer func() {
		if reterr != nil {
			// using backup config
			klog.Infof("error encountered, rollback using old kubelet configuration")
			err = util.CopyFile(tmpBackupKubeadmFlagsEnvFilePath, originKubeadmFlagsEnvFilePath)
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
	}()

	if same, err := util.FileSameContent(backupKubeadmFlagsEnvFilePath, originKubeadmFlagsEnvFilePath); err == nil && same {
		klog.Info("skip revert kubelet kubeadm-flags.env file , no changes")
		return nil
	}

	err = util.CopyFile(backupKubeadmFlagsEnvFilePath, originKubeadmFlagsEnvFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to restore kubeadm-flags.env")
	}
	klog.Infof("restore kubeadm-flags.env in %v", originKubeadmFlagsEnvFilePath)

	// restart kubelet
	if err := util.RestartService("kubelet"); err != nil {
		return errors.Wrap(err, "failed to restart kubelet service")
	}

	// wait for kubelet ready
	return waitKubeletReady(kubeletHealthzPort, opt.healthCheckTimeout)
}

func (opt *revertOptions) removeYurtHub(cmdLine string) (reterr error) {
	pkiDir, err := parsePkiDir(cmdLine)
	if err != nil {
		klog.Warningf("failed to parse --client-ca-file param from cmdline %v, fallback to %v", cmdLine, opt.kubePKIDir)
		pkiDir = opt.kubePKIDir
	}

	yurtHubManifestPath := filepath.Join(filepath.Dir(pkiDir), "manifests", "yurt-hub.yaml")
	if exists, err := util.FileExists(yurtHubManifestPath); err == nil && exists {
		return os.RemoveAll(yurtHubManifestPath)
	}
	return nil
}

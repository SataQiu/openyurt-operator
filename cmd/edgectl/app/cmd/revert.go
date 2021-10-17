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
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/edgectl/app/options"
	apputil "github.com/openyurtio/openyurt-operator/cmd/edgectl/app/util"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

// NewCmdRevert returns *cobra.Command for node revert
func NewCmdRevert() *cobra.Command {
	opt := options.NewRevertOptions()

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Revert node to normal node",
		RunE: func(cmd *cobra.Command, args []string) error {
			// init client
			restConfig, err := kclient.GetConfigWithAPIServerAddress(opt.APIServerAddress)
			if err != nil {
				return err
			}
			kclient.InitializeKubeClient(restConfig)

			return runRevert(context.Background(), opt)
		},
	}

	opt.AddAllFlags(cmd.Flags())
	return cmd
}

func runRevert(ctx context.Context, opt *options.RevertOptions) (reterr error) {
	yurtCluster := &operatorv1alpha1.YurtCluster{}
	namespacedName := types.NamespacedName{Name: operatorv1alpha1.SingletonYurtClusterInstanceName}
	err := kclient.CtlClient().Get(ctx, namespacedName, yurtCluster)
	if err != nil {
		return err
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
		nodeRevertCondition := operatorv1alpha1.NodeCondition{
			Status:  "True",
			Reason:  operatorv1alpha1.NodeConditionReasonNodeRevert,
			Message: "Node was reverted successfully",
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
			ObservedGeneration: yurtCluster.Generation,
		}
		if reterr != nil {
			nodeRevertCondition = operatorv1alpha1.NodeCondition{
				Status:  "False",
				Reason:  operatorv1alpha1.NodeConditionReasonNodeRevert,
				Message: reterr.Error(),
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			}
		}
		apputil.UpdateNodeCondition(yurtCluster, opt.NodeName, nodeRevertCondition)
	}()

	// get local node from cluster
	node := &corev1.Node{}
	key := types.NamespacedName{Name: opt.NodeName}
	err = kclient.CtlClient().Get(ctx, key, node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get node %q from cluster", opt.NodeName)
	}

	// skip if node not ready
	if !opt.Force && !util.IsNodeReady(node) {
		return errors.Errorf("skip revert node %v, because it is not ready now", opt.NodeName)
	}

	kubeletConfiguration, err := apputil.LoadKubeletConfiguration(&opt.BaseOptions)
	if err != nil {
		return err
	}

	if err := revertKubelet(kubeletConfiguration, opt.YurtWorkDir, opt.HealthCheckTimeout); err != nil {
		return errors.Wrap(err, "failed to revert kubelet configuration")
	}

	if err := removeYurtHub(kubeletConfiguration); err != nil {
		return errors.Wrap(err, "failed to remove yurt-hub")
	}

	klog.Infof("revert node %v as normal node successfully", opt.NodeName)
	return nil
}

// revertKubelet reverts kubelet to connect kube-api-server directly
func revertKubelet(kubeletConfiguration *apputil.KubeletConfiguration,
	yurtWorkDir string, healthCheckTimeout time.Duration) (reterr error) {
	kubeadmFlagsEnvBackupFilePath := filepath.Join(yurtWorkDir, kubeadmFlagsEnvBackupName)
	if exists, err := util.FileExists(kubeadmFlagsEnvBackupFilePath); err != nil || !exists {
		klog.Warning("skp kubelet revert, no backup kubeadm-flags.env found")
		return nil
	}

	// backup current kubelet kubeadm-flags.env file temporarily
	kubeadmFlagsEnvTmpBackupFilePath := filepath.Join(yurtWorkDir, "kubeadm-flags.env.revert.tmp")
	if err := util.CopyFile(kubeadmFlagsEnvFilePath, kubeadmFlagsEnvTmpBackupFilePath); err != nil {
		return errors.Wrap(err, "failed to backup current kubeadm-flags.env")
	}
	defer func() {
		os.RemoveAll(kubeadmFlagsEnvTmpBackupFilePath)
	}()

	defer func() {
		if reterr != nil {
			// using backup config
			klog.Infof("error encountered, rollback using old kubelet configuration")
			if err := util.CopyFile(kubeadmFlagsEnvTmpBackupFilePath, kubeadmFlagsEnvFilePath); err != nil {
				klog.Errorf("failed to rollback kubeadm-flags.env for kubelet, %v", err)
				return
			}
			klog.Infof("rollback to old kubelet config successfully")
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

	if same, err := util.FileSameContent(kubeadmFlagsEnvBackupFilePath, kubeadmFlagsEnvFilePath); err == nil && same {
		klog.Info("skip revert kubelet kubeadm-flags.env file , no changes")
		return nil
	}

	if err := util.CopyFile(kubeadmFlagsEnvBackupFilePath, kubeadmFlagsEnvFilePath); err != nil {
		return errors.Wrap(err, "failed to restore kubeadm-flags.env")
	}
	klog.Infof("restore kubeadm-flags.env in %v", kubeadmFlagsEnvFilePath)

	// restart kubelet
	if err := util.RestartService("kubelet"); err != nil {
		return errors.Wrap(err, "failed to restart kubelet service")
	}

	// wait for kubelet ready
	return waitKubeletReady(kubeletConfiguration.HealthPort, healthCheckTimeout)
}

func removeYurtHub(kubeletConfiguration *apputil.KubeletConfiguration) error {
	yurtHubFilePath := filepath.Join(kubeletConfiguration.PodManifestsPath, yurtHubYamlName)
	if exists, err := util.FileExists(yurtHubFilePath); err == nil && exists {
		return os.RemoveAll(yurtHubFilePath)
	}
	return nil
}

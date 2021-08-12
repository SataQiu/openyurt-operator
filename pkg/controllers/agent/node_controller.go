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

package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/agent/options"
	"github.com/openyurtio/openyurt-operator/pkg/constants"
	controllersutil "github.com/openyurtio/openyurt-operator/pkg/controllers"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/predicates"
	"github.com/openyurtio/openyurt-operator/pkg/projectinfo"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

const (
	defaultRequeueAfterTime = time.Second * 5
)

// DefaultBackoff is the recommended backoff for a conflict where a client
// may be attempting to make an unrelated modification to a resource under
// active management by one or more controllers.
var DefaultBackoff = wait.Backoff{
	Steps:    20,
	Duration: 10 * time.Millisecond,
	Factor:   5.0,
	Jitter:   0.1,
}

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Options *options.Options
}

var nodeReconcilerLock sync.Mutex

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// quick return
	if req.Name != r.Options.NodeName {
		return ctrl.Result{}, nil
	}

	nodeReconcilerLock.Lock()
	defer nodeReconcilerLock.Unlock()

	log := r.Log.WithValues("Node", req.NamespacedName)

	node := &corev1.Node{}
	if err := r.Client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("reconcile node", "NamespacedName", req.NamespacedName)

	yurtCluster, err := util.LoadYurtCluster(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// sleep one minute, then re-check
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, err
	}

	// ensure normal node
	if !controllersutil.IsEdgeNode(node) {
		if controllersutil.IsNodeAlreadyReverted(yurtCluster, node.Name) {
			return ctrl.Result{}, nil
		}

		// if the YurtCluster is deleting, we should wait for tunnel server deleted before do revert
		// to ensure the iptables rules created by tunnel server can be cleaned completely
		if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
			deleted, err := r.isTunnelServerDeleted(ctx)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to check if tunnel server is deleted before do revert")
			}
			if !deleted {
				klog.Info("wait tunnel server being deleted before do revert")
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
		}

		if !util.IsNodeReady(node) {
			klog.Infof("node %v is not ready now, will try best to revert", node.Name)
			return r.runRevertLocally(ctx, yurtCluster)
		}
		return r.runRevert(ctx, yurtCluster, node.Name)
	}

	// ensure edge node
	if controllersutil.IsNodeAlreadyConverted(yurtCluster, node.Name) {
		return ctrl.Result{}, nil
	}
	if !util.IsNodeReady(node) {
		return ctrl.Result{}, errors.Errorf("can not convert node %q because it is in NotReady status", node.Name)
	}
	return r.runConvert(ctx, yurtCluster, node.Name)
}

// runRevertLocally is an best-effort to revert unhealthy node
// It requires the yurt node agent to run in privileged mode, which can be a security hazard.
func (r *NodeReconciler) runRevertLocally(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster) (ctrl.Result, error) {
	result, err := util.RunCommandWithCombinedOutput("/local-revert.sh")
	if err != nil {
		return ctrl.Result{}, err
	}
	klog.Infof("run revert result: %v", string(result))

	if yurtCluster.Spec.YurtHub.AutoRestartNodePod != nil && *yurtCluster.Spec.YurtHub.AutoRestartNodePod {
		if err := restartLocalPodsLocally(ctx, false); err != nil {
			klog.Warningf("post restart containers with error %v", err)
		}
	}

	return ctrl.Result{}, nil
}

// assume that all materials have been prepared by yurt agent init container
func restartLocalPodsLocally(ctx context.Context, isConvert bool) error {
	svc := &corev1.Service{}
	err := kclient.CtlClient().Get(ctx, types.NamespacedName{Namespace: "default", Name: "kubernetes"}, svc)
	if err != nil {
		return err
	}
	if svc.Spec.ClusterIP == "" {
		klog.Warningf("found empty ClusterIP in kubernetes default service, skip restart containers")
		return nil
	}
	cmd := fmt.Sprintf("/local-restart-container.sh %v %s", isConvert, svc.Spec.ClusterIP)
	result, err := util.RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return err
	}
	klog.Infof("restart containers with command %q result: %v", cmd, string(result))
	return nil
}

func (r *NodeReconciler) runRevert(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) (_ ctrl.Result, reterr error) {
	// check if there is a pod with same name and running status
	// if true, just requeue this request
	running, err := r.isPreviousRevertPodRunning(ctx, nodeName)
	if err != nil {
		klog.Warningf("failed to check if previous revert Pod is running, %v, assume no execution is in progress", err)
	}
	if running {
		return ctrl.Result{RequeueAfter: defaultRequeueAfterTime}, nil
	}

	// apply RBAC artifacts before run revert Pod
	manifestsTemplate, err := util.LoadManifestsTemplate(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := applyRBACForYurtNode(ctx, manifestsTemplate, yurtCluster); err != nil {
		return ctrl.Result{}, err
	}

	podToSave, err := r.genNodeRevertPod(manifestsTemplate, yurtCluster, nodeName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// always try to delete pod if exists
	key := types.NamespacedName{Namespace: podToSave.Namespace, Name: podToSave.Name}
	pod := &corev1.Pod{}
	if err := r.Client.Get(ctx, key, pod); err == nil {
		if err := r.Client.Delete(ctx, pod); err != nil {
			klog.Errorf("failed to delete previous revert pod %v, %v", key, err)
		}
	}

	// create revert Pod with retry
	err = wait.ExponentialBackoff(DefaultBackoff, func() (bool, error) {
		if err := r.Client.Create(ctx, podToSave); err != nil {
			klog.Warningf("failed to create revert pod for %v, %v", nodeName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create revert pod for %v", nodeName)
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) isPreviousRevertPodRunning(ctx context.Context, nodeName string) (bool, error) {
	key := types.NamespacedName{Namespace: "kube-system", Name: fmt.Sprintf("yurt-revert-%s", nodeName)}
	pod := &corev1.Pod{}
	err := r.Client.Get(ctx, key, pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if pod.Status.Phase != corev1.PodRunning {
		// check if container is creating now
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Waiting != nil && status.State.Waiting.Reason == "ContainerCreating" {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

func (r *NodeReconciler) genNodeRevertPod(manifestsTemplate *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) (*corev1.Pod, error) {
	yurtNodeRevertPodTmpl, ok := manifestsTemplate.Data[constants.YurtNodeRevertPodKey]
	if !ok {
		return nil, errors.Errorf("cannot find key %v in operator manifests template", constants.YurtNodeRevertPodKey)
	}
	values := map[string]string{
		"nodeName":                nodeName,
		"yurtNodeImage":           r.Options.YurtNodeImage,
		"yurtNodeImagePullPolicy": r.Options.YurtNodeImagePullPolicy,
		"healthCheckTimeout":      r.Options.YurtTaskHealthCheckTimeout.String(),
	}
	pod, err := util.GetConvertOrRevertPodFromYamlTemplate(yurtNodeRevertPodTmpl, values)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate revert pod from yaml template")
	}
	return pod, nil
}

func (r *NodeReconciler) runConvert(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) (_ ctrl.Result, reterr error) {
	// check if there is a pod with same name and running status
	// if true, just requeue this request
	running, err := r.isPreviousConvertPodRunning(ctx, nodeName)
	if err != nil {
		klog.Warningf("failed to check if previous convert Pod is running, %v, assume no execution is in progress", err)
	}
	if running {
		return ctrl.Result{RequeueAfter: defaultRequeueAfterTime}, nil
	}

	// apply RBAC artifacts before run revert Pod
	manifestsTemplate, err := util.LoadManifestsTemplate(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := applyRBACForYurtNode(ctx, manifestsTemplate, yurtCluster); err != nil {
		return ctrl.Result{}, err
	}

	podToSave, err := r.genNodeConvertPod(manifestsTemplate, yurtCluster, nodeName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// always try to delete pod if exists
	key := types.NamespacedName{Namespace: podToSave.Namespace, Name: podToSave.Name}
	pod := &corev1.Pod{}
	if err := r.Client.Get(ctx, key, pod); err == nil {
		if err := r.Client.Delete(ctx, pod); err != nil {
			klog.Errorf("failed to delete previous convert pod %v", key)
		}
	}

	// create convert Pod with retry
	err = wait.ExponentialBackoff(DefaultBackoff, func() (bool, error) {
		if err := r.Client.Create(ctx, podToSave); err != nil {
			klog.Warningf("failed to create convert pod for %v, %v", nodeName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create convert pod for %v", nodeName)
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) isPreviousConvertPodRunning(ctx context.Context, nodeName string) (bool, error) {
	key := types.NamespacedName{Namespace: "kube-system", Name: fmt.Sprintf("yurt-convert-%s", nodeName)}
	pod := &corev1.Pod{}
	err := r.Client.Get(ctx, key, pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if pod.Status.Phase != corev1.PodRunning {
		// check if container is creating now
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Waiting != nil && status.State.Waiting.Reason == "ContainerCreating" {
				return true, nil
			}
		}
		return false, nil
	}

	return true, nil
}

func (r *NodeReconciler) isTunnelServerDeleted(ctx context.Context) (bool, error) {
	daemonSets := &appsv1.DaemonSetList{}
	if err := r.Client.List(ctx, daemonSets, []client.ListOption{
		client.MatchingLabels{
			"k8s-app": "yurt-tunnel-server",
		},
	}...); err != nil {
		return false, errors.Wrap(err, "failed to list yurt tunnel server DaemonSets")
	}
	if len(daemonSets.Items) > 0 {
		return false, nil
	}

	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: "kube-system", Name: "yurt-tunnel-server"}, deploy)
	if err == nil {
		return false, nil
	}
	if !apierrors.IsNotFound(err) {
		return false, errors.Wrap(err, "failed to get yurt tunnel server Deployment")
	}

	return true, nil
}

func (r *NodeReconciler) genNodeConvertPod(manifestsTemplate *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) (*corev1.Pod, error) {
	yurtNodeConvertPodTmpl, ok := manifestsTemplate.Data[constants.YurtNodeConvertPodKey]
	if !ok {
		return nil, errors.Errorf("cannot find key %v in operator manifests template", constants.YurtNodeConvertPodKey)
	}
	values := map[string]string{
		"nodeName":                nodeName,
		"yurtNodeImage":           r.Options.YurtNodeImage,
		"yurtNodeImagePullPolicy": r.Options.YurtNodeImagePullPolicy,
		"healthCheckTimeout":      r.Options.YurtTaskHealthCheckTimeout.String(),
	}
	pod, err := util.GetConvertOrRevertPodFromYamlTemplate(yurtNodeConvertPodTmpl, values)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate convert pod from yaml template")
	}
	return pod, nil
}

func applyRBACForYurtNode(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.YurtNodeServiceAccountKey,
		constants.YurtNodeClusterRoleKey,
		constants.YurtNodeClusterRoleBindingKey,
	}

	values := map[string]string{}

	// ensure manifests
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to apply yurt node rbac")
		}
	}
	return nil
}

func (r *NodeReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicates.ResourceLabelChanged(ctrl.LoggerFrom(ctx), projectinfo.GetEdgeWorkerLabelKey())).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

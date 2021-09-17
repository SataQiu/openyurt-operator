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
	"time"

	"github.com/go-logr/logr"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/agent/options"
	"github.com/openyurtio/openyurt-operator/pkg/constants"
	controllersutil "github.com/openyurtio/openyurt-operator/pkg/controllers"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

const (
	defaultRequeueAfterTime                  = time.Second * 5
	kubeControllerManagerDefaultManifestPath = "/etc/kubernetes/manifests/kube-controller-manager.yaml"
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

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// quick return
	if req.Name != r.Options.NodeName {
		return ctrl.Result{}, nil
	}

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

	log.V(2).Info("reconcile node", "NamespacedName", req.NamespacedName)

	// Initialize the patch helper.
	patchHelper, err := patcher.NewHelper(node, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to Patch the Cluster object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patcher.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patcher.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, node, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

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

		// ensure no edge taint
		util.RemoveEdgeTaintForNode(node)

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
			return r.runRevertForce(ctx, yurtCluster)
		}
		return r.runRevert(ctx, yurtCluster, node.Name)
	}

	// ensure edge node
	util.AddEdgeTaintForNode(node)
	if controllersutil.IsNodeAlreadyConverted(yurtCluster, node.Name) {
		return ctrl.Result{}, nil
	}
	if !util.IsNodeReady(node) {
		return ctrl.Result{}, errors.Errorf("can not convert node %q because it is in NotReady status", node.Name)
	}
	return r.runConvert(ctx, yurtCluster, node.Name)
}

// runRevertForce is an best-effort to revert unhealthy node locally
// It requires the yurt node agent to run in privileged mode, which can be a security hazard.
func (r *NodeReconciler) runRevertForce(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster) (ctrl.Result, error) {
	cmd := dedent.Dedent(fmt.Sprintf(`
          set -e
          cp -a /var/run/secrets/kubernetes.io/serviceaccount /tmp/serviceaccount
          nsenter -t 1 -m -u -n -i sh <<EOF
          if [ ! -d /var/run/secrets/kubernetes.io ]; then
            mkdir -p /var/run/secrets/kubernetes.io
          fi
          cp -a /var/tmp/serviceaccount /var/run/secrets/kubernetes.io/
          /var/tmp/edgectl revert --node-name %v --health-check-timeout %v --force
          rm -rf /var/run/secrets/kubernetes.io/serviceaccount
          exit
          EOF
          rm -rf /tmp/serviceaccount
	`, r.Options.NodeName, r.Options.YurtTaskHealthCheckTimeout))

	result, err := util.RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return ctrl.Result{}, err
	}
	klog.Infof("run revert with result: %v", string(result))

	return ctrl.Result{}, nil
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

func (r *NodeReconciler) YurtClusterToNodeMapFunc(o client.Object) []ctrl.Request {
	yurtCluster, ok := o.(*operatorv1alpha1.YurtCluster)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("expected a YurtCluster but got a %T", o))
		return nil
	}

	if yurtCluster.Name != constants.SingletonYurtClusterInstanceName {
		return nil
	}

	// enable nodelifecycle controller back
	isMaster, err := util.IsMasterNode(context.TODO(), r.Client, r.Options.NodeName)
	if err != nil {
		r.Log.Error(err, "failed to check whether node is Master", "NodeName", r.Options.NodeName)
	}
	if isMaster {
		if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
			if err := util.EnableNodeLifeCycleController(kubeControllerManagerDefaultManifestPath); err != nil {
				r.Log.Error(err, "failed to enable nodelifecycle controller for kube-controller-manager")
			}
		} else {
			if err := util.DisableNodeLifeCycleController(kubeControllerManagerDefaultManifestPath); err != nil {
				r.Log.Error(err, "failed to disable nodelifecycle controller for kube-controller-manager")
			}
		}
	}

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: r.Options.NodeName,
			},
		},
	}
}

func (r *NodeReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Watches(
			&source.Kind{Type: &operatorv1alpha1.YurtCluster{}},
			handler.EnqueueRequestsFromMapFunc(r.YurtClusterToNodeMapFunc),
		).
		WithOptions(options).
		Complete(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

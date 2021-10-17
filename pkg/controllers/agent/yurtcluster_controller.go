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
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/agent/options"
	controllersutil "github.com/openyurtio/openyurt-operator/pkg/controllers/util"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/projectinfo"
	"github.com/openyurtio/openyurt-operator/pkg/templates"
	"github.com/openyurtio/openyurt-operator/pkg/templates/trans"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

// YurtClusterReconciler reconciles a YurtCluster object
type YurtClusterReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Options *options.Options

	recorder record.EventRecorder
}

func (r *YurtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// quick return
	if req.Name != operatorv1alpha1.SingletonYurtClusterInstanceName {
		return ctrl.Result{}, nil
	}

	log := r.Log.WithValues("YurtCluster", req.NamespacedName)

	yurtCluster := &operatorv1alpha1.YurtCluster{}
	err := r.Get(ctx, req.NamespacedName, yurtCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get YurtCluster", "Name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// return when the YurtCluster is not approved by operator
	if !controllerutil.ContainsFinalizer(yurtCluster, operatorv1alpha1.YurtClusterFinalizer) {
		return ctrl.Result{}, nil
	}

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.Options.NodeName}, node); err != nil {
		return ctrl.Result{}, err
	}

	// initialize the patch helper.
	patchHelper, err := patcher.NewHelper(node, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to Patch the node object and status after each reconciliation.
		patchOpts := []patcher.Option{}
		if err := patchHelper.Patch(ctx, node, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// handle deletion reconciliation loop
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, yurtCluster, node)
	}

	// handle normal reconciliation loop
	return r.reconcile(ctx, yurtCluster, node)
}

func (r *YurtClusterReconciler) reconcileDelete(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, node *corev1.Node) (ctrl.Result, error) {
	if util.IsControlPlaneNode(node) {
		path := filepath.Join(yurtCluster.Spec.YurtHub.PodManifestsPath, "kube-controller-manager.yaml")
		if err := util.EnableNodeLifeCycleController(path); err != nil {
			return ctrl.Result{}, err
		}
	}
	return r.reconcileNormalNode(ctx, yurtCluster, node)
}

func (r *YurtClusterReconciler) reconcile(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, node *corev1.Node) (ctrl.Result, error) {
	if util.IsControlPlaneNode(node) {
		path := filepath.Join(yurtCluster.Spec.YurtHub.PodManifestsPath, "kube-controller-manager.yaml")
		if err := util.DisableNodeLifeCycleController(path); err != nil {
			return ctrl.Result{}, err
		}
	}

	switch controllersutil.GetNodeType(node, yurtCluster) {
	case operatorv1alpha1.CloudNode:
		return r.reconcileCloudNode(ctx, yurtCluster, node)
	case operatorv1alpha1.EdgeNode:
		return r.reconcileEdgeNode(ctx, yurtCluster, node)
	default:
		return r.reconcileNormalNode(ctx, yurtCluster, node)
	}
}

func (r *YurtClusterReconciler) reconcileCloudNode(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, node *corev1.Node) (ctrl.Result, error) {
	if controllersutil.IsNodeConvertOrRevertCompleted(node, yurtCluster) {
		return ctrl.Result{}, nil
	}

	if !util.IsNodeReady(node) {
		return ctrl.Result{}, errors.Errorf("can not convert node %q because it is in NotReady status", node.Name)
	}

	// add edge label for node
	node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "false"

	return r.runNodeConvert(ctx, operatorv1alpha1.CloudNode, yurtCluster)
}

func (r *YurtClusterReconciler) reconcileEdgeNode(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, node *corev1.Node) (ctrl.Result, error) {
	if controllersutil.IsNodeConvertOrRevertCompleted(node, yurtCluster) {
		return ctrl.Result{}, nil
	}

	if !util.IsNodeReady(node) {
		return ctrl.Result{}, errors.Errorf("can not convert node %q because it is in NotReady status", node.Name)
	}

	// add edge taint
	util.EnsureEdgeTaintForNode(node)

	// add edge label for node
	node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "true"

	return r.runNodeConvert(ctx, operatorv1alpha1.EdgeNode, yurtCluster)
}

func (r *YurtClusterReconciler) reconcileNormalNode(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster, node *corev1.Node) (ctrl.Result, error) {
	if controllersutil.IsNodeConvertOrRevertCompleted(node, yurtCluster) {
		return ctrl.Result{}, nil
	}

	// remove edge taint if exists
	util.RemoveEdgeTaintForNode(node)

	// remove edge label from node labels
	delete(node.Labels, projectinfo.GetEdgeWorkerLabelKey())

	return r.runNodeRevert(ctx, yurtCluster)
}

func (r *YurtClusterReconciler) runNodeConvert(ctx context.Context, nodeType operatorv1alpha1.NodeType, yurtCluster *operatorv1alpha1.YurtCluster) (ctrl.Result, error) {
	tpl, err := templates.LoadTemplate(ctx, trans.NamespacedName)
	if err != nil {
		// waiting template created by operator
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil //nolint
	}

	scripts, ok := tpl.Data[trans.NodeConvertCommand]
	if !ok {
		return ctrl.Result{}, errors.Errorf("can not find key %v from template %v", trans.NodeConvertCommand, klog.KObj(tpl))
	}

	cmd := fmt.Sprintf(scripts, yurtCluster.Spec.YurtHub.PodManifestsPath, r.Options.NodeName, nodeType, r.Options.TransHealthCheckTimeout)
	klog.Infof("run command %q, waiting for the result...", cmd)
	result, err := util.RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return ctrl.Result{}, err
	}

	klog.Infof("run convert with result: \n%v", string(result))

	return ctrl.Result{}, nil
}

func (r *YurtClusterReconciler) runNodeRevert(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster) (ctrl.Result, error) {
	tpl, err := templates.LoadTemplate(ctx, trans.NamespacedName)
	if err != nil {
		// waiting template created by operator
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil //nolint
	}

	scripts, ok := tpl.Data[trans.NodeRevertCommand]
	if !ok {
		return ctrl.Result{}, errors.Errorf("can not find key %v from template %v", trans.NodeRevertCommand, klog.KObj(tpl))
	}

	cmd := fmt.Sprintf(scripts, yurtCluster.Spec.YurtHub.PodManifestsPath, r.Options.NodeName, r.Options.TransHealthCheckTimeout)
	klog.Infof("run command %q, waiting for the result...", cmd)
	result, err := util.RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return ctrl.Result{}, err
	}

	klog.Infof("run revert with result: \n%v", string(result))

	return ctrl.Result{}, nil
}

func (r *YurtClusterReconciler) NodeToYurtClusterMapFunc(o client.Object) []ctrl.Request {
	node, ok := o.(*corev1.Node)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("expected a Node but got a %T", o))
		return nil
	}

	if node.Name != r.Options.NodeName {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: operatorv1alpha1.SingletonYurtClusterInstanceName},
		},
	}
}

func (r *YurtClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.YurtCluster{}).
		Watches(
			&source.Kind{Type: &corev1.Node{}},
			handler.EnqueueRequestsFromMapFunc(r.NodeToYurtClusterMapFunc),
		).
		WithOptions(options).
		Complete(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("yurt-cluster-controller")
	return nil
}

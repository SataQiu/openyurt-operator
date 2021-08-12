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

package operator

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/openyurtio/openyurt-operator/pkg/constants"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/predicates"
	"github.com/openyurtio/openyurt-operator/pkg/projectinfo"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	YurtClusterReconciler *YurtClusterReconciler
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := r.Log.WithValues("Node", req.NamespacedName)

	yurtCluster, err := util.LoadYurtCluster(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// sleep one minute, then re-check
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, err
	}

	patchHelperYurtCluster, err := patcher.NewHelper(yurtCluster, r.Client)
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
		if err := patchHelperYurtCluster.Patch(ctx, yurtCluster, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	log.Info("reconcile node", "NamespacedName", req.NamespacedName)

	node := &corev1.Node{}
	if err := r.Client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	patchHelperNode, err := patcher.NewHelper(node, r.Client)
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
		if err := patchHelperNode.Patch(ctx, node, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	ensureEdgeLabel(node)

	// re-reconcile YurtCluster status to mark as it as Converting if need
	if _, err := r.YurtClusterReconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: constants.SingletonYurtClusterInstanceName},
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func ensureEdgeLabel(node *corev1.Node) {
	labelKey := projectinfo.GetEdgeWorkerLabelKey()
	found := false
	for key := range node.Labels {
		if key == labelKey {
			found = true
			break
		}
	}
	if !found {
		node.Labels[labelKey] = "false"
	}
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

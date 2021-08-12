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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/constants"
	"github.com/openyurtio/openyurt-operator/pkg/predicates"
)

// YurtClusterReconciler reconciles a YurtCluster object
type YurtClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	NodeName       string
	NodeReconciler *NodeReconciler
}

// +kubebuilder:rbac:groups=operator.openyurt.io,resources=yurtclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.openyurt.io,resources=yurtclusters/status,verbs=get;update;patch

func (r *YurtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := r.Log.WithValues("yurtcluster", req.NamespacedName)

	// quick return
	if req.Name != constants.SingletonYurtClusterInstanceName {
		return ctrl.Result{}, nil
	}

	log.Info("reconcile YurtCluster", "NamespacedName", req.NamespacedName)

	// just reconcile node to check if need do convert or revert
	return r.NodeReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: r.NodeName}})
}

func (r *YurtClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.YurtCluster{}).
		WithEventFilter(predicates.ResourceHasName(ctrl.LoggerFrom(ctx), constants.SingletonYurtClusterInstanceName)).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

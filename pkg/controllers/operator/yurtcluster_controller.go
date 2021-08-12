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
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/operator/options"
	"github.com/openyurtio/openyurt-operator/pkg/constants"
	controllersutil "github.com/openyurtio/openyurt-operator/pkg/controllers"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/patcher"
	"github.com/openyurtio/openyurt-operator/pkg/predicates"
	"github.com/openyurtio/openyurt-operator/pkg/projectinfo"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

const (
	YurtClusterFinalizer = "openyurt.io/openyurt"
	DefaultClusterDomain = "cluster.local"
)

// YurtClusterReconciler reconciles a YurtCluster object
type YurtClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Options *options.Options
}

var yurtClusterReconcilerLock sync.Mutex

// +kubebuilder:rbac:groups=operator.openyurt.io,resources=yurtclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.openyurt.io,resources=yurtclusters/status,verbs=get;update;patch

func (r *YurtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// quick return
	if req.Name != constants.SingletonYurtClusterInstanceName {
		return ctrl.Result{}, nil
	}

	yurtClusterReconcilerLock.Lock()
	defer yurtClusterReconcilerLock.Unlock()

	log := r.Log.WithValues("yurtcluster", req.NamespacedName)
	log.Info("reconcile YurtCluster", "NamespacedName", req.NamespacedName)

	yurtCluster, err := util.LoadYurtCluster(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get YurtCluster", "Name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Initialize the patch helper.
	patchHelper, err := patcher.NewHelper(yurtCluster, r.Client)
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
		if err := patchHelper.Patch(ctx, yurtCluster, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Add finalizer if not exist and complete default fields first
	if !controllerutil.ContainsFinalizer(yurtCluster, YurtClusterFinalizer) {
		controllerutil.AddFinalizer(yurtCluster, YurtClusterFinalizer)
		yurtCluster.Status.Phase = operatorv1alpha1.PhaseConverting
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, yurtCluster)
}

func (r *YurtClusterReconciler) reconcile(ctx context.Context, yurtCluster *operatorv1alpha1.YurtCluster) (ctrl.Result, error) {
	// check if is deleting
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if yurtCluster.Status.LastUpdateTime.Time.UnixNano() != yurtCluster.ObjectMeta.DeletionTimestamp.UnixNano() {
			yurtCluster.Status.LastUpdateSpec = yurtCluster.Spec
			yurtCluster.Status.LastUpdateTime = *yurtCluster.ObjectMeta.DeletionTimestamp
		}
		// mark all nodes as normal node
		if err := r.markNodesAsNormal(ctx); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// check if should propagate again
		if !reflect.DeepEqual(yurtCluster.Spec, yurtCluster.Status.LastUpdateSpec) {
			yurtCluster.Status.LastUpdateSpec = yurtCluster.Spec
			yurtCluster.Status.LastUpdateTime = metav1.Time{
				Time: time.Now(),
			}
		}
	}

	// prepare operator manifests template
	if err := r.reconcileOperatorManifestsTemplate(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// load manifests from cluster
	manifestsTemplate, err := util.LoadManifestsTemplate(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// install yurt node agent
	if err := r.reconcileYurtNodeAgent(ctx, manifestsTemplate, yurtCluster); err != nil {
		return ctrl.Result{}, err
	}

	// ensure yurt components
	if err := r.reconcileYurtComponents(ctx, manifestsTemplate, yurtCluster); err != nil {
		return ctrl.Result{}, err
	}

	// update status
	if err := r.reconcileStatus(ctx, manifestsTemplate, yurtCluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *YurtClusterReconciler) reconcileOperatorManifestsTemplate(ctx context.Context) error {
	if err := util.Apply(ctx, util.OperatorManifestsTemplate); err != nil {
		return errors.Errorf("failed to ensure operator manifests template, %v", err)
	}
	return nil
}

func (r *YurtClusterReconciler) reconcileYurtNodeAgent(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.YurtNodeAgentServiceAccountKey,
		constants.YurtNodeAgentClusterRoleKey,
		constants.YurtNodeAgentClusterRoleBindingKey,
		constants.YurtNodeAgentDaemonSetKey,
	}

	values := map[string]string{
		"yurtNodeAgentImage":           r.Options.YurtAgentImage,
		"yurtNodeAgentImagePullPolicy": r.Options.YurtAgentImagePullPolicy,
		"yurtNodeImage":                r.Options.YurtNodeImage,
		"yurtNodeImagePullPolicy":      r.Options.YurtNodeImagePullPolicy,
		"yurtTaskHealthCheckTimeout":   r.Options.YurtTaskHealthCheckTimeout.String(),
	}

	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt node agent")
		}
	}
	return nil
}

func (r *YurtClusterReconciler) reconcileYurtComponents(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	var errs []error

	// yurt controller manager
	if err := r.reconcileYurtControllerManager(ctx, template, yurtCluster); err != nil {
		errs = append(errs, err)
	}

	// yurt app manager
	if err := r.reconcileYurtAppManager(ctx, template, yurtCluster); err != nil {
		errs = append(errs, err)
	}

	// node local dns cache
	if yurtCluster.Spec.NodeLocalDNSCache.Enable != nil && *yurtCluster.Spec.NodeLocalDNSCache.Enable {
		if err := r.reconcileNodeLocalDNSCache(ctx, template, yurtCluster); err != nil {
			errs = append(errs, err)
		}
	}

	// yurt tunnel
	if yurtCluster.Spec.YurtTunnel.Enable != nil && *yurtCluster.Spec.YurtTunnel.Enable {
		if err := r.reconcileYurtTunnelServer(ctx, template, yurtCluster); err != nil {
			errs = append(errs, err)
		}

		if err := r.reconcileYurtTunnelAgent(ctx, template, yurtCluster); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (r *YurtClusterReconciler) reconcileYurtControllerManager(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.YurtControllerManagerDeploymentKey,
		constants.YurtControllerManagerServiceAccountKey,
		constants.YurtControllerManagerClusterRoleKey,
		constants.YurtControllerManagerClusterRoleBindingKey,
	}

	values := map[string]string{
		"edgeNodeLabel":       projectinfo.GetEdgeWorkerLabelKey(),
		"yurtControllerImage": util.GetYurtComponentImageByName(yurtCluster, "yurt-controller-manager"),
	}

	// reconcile delete
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, key := range keys {
			if err := util.DeleteTemplateWithRender(ctx, template, key, values); err != nil {
				return errors.Wrap(err, "failed to reconcile yurt controller manager")
			}
		}
		// restore node-lifecycle controller
		var nodeControllerClusterRoleBinding rbacv1.ClusterRoleBinding
		key := types.NamespacedName{Name: constants.NodeControllerClusterRoleBindingName}
		err := kclient.CtlClient().Get(ctx, key, &nodeControllerClusterRoleBinding)
		if err == nil {
			// already created, so skip
			return nil
		}
		if apierrors.IsNotFound(err) {
			nodeControllerClusterRoleBinding = rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.NodeControllerClusterRoleBindingName,
					Annotations: map[string]string{
						"rbac.authorization.kubernetes.io/autoupdate": "true",
					},
					Labels: map[string]string{
						"kubernetes.io/bootstrapping": "rbac-defaults",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     "system:controller:node-controller",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      "node-controller",
						Namespace: metav1.NamespaceSystem,
					},
				},
			}
			if err := kclient.CtlClient().Create(ctx, &nodeControllerClusterRoleBinding); err != nil {
				return errors.Wrapf(err, "failed to restore %q ClusterRoleBinding", constants.NodeControllerClusterRoleBindingName)
			}
		}
		return err
	}

	// normal reconcile
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt controller manager")
		}
	}

	// ensure the node-lifecycle controller of the k8s controller manager is off
	// we implement this by delete the system:controller:node-controller ClusterRoleBinding
	nodeControllerClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	key := types.NamespacedName{Name: constants.NodeControllerClusterRoleBindingName}
	if err := r.Client.Get(ctx, key, nodeControllerClusterRoleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Client.Delete(ctx, nodeControllerClusterRoleBinding)
}

func (r *YurtClusterReconciler) reconcileYurtAppManager(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	var keys []string

	if yurtCluster.Spec.YurtAppManager.EnableController != nil && *yurtCluster.Spec.YurtAppManager.EnableController {
		keys = append(keys, []string{
			constants.YurtAppManagerElectionRoleKey,
			constants.YurtAppManagerClusterRoleKey,
			constants.YurtAppManagerServiceAccountKey,
			constants.YurtAppManagerRoleBindingKey,
			constants.YurtAppManagerClusterRoleBindingKey,
			constants.YurtAppManagerSecretKey,
			constants.YurtAppManagerServiceKey,
			constants.YurtAppManagerDeploymentKey,
			constants.YurtAppManagerMutatingWebhookConfigurationKey,
			constants.YurtAppManagerValidatingWebhookConfigurationKey,
		}...)
	}

	if yurtCluster.Spec.YurtAppManager.InstallCRD != nil && *yurtCluster.Spec.YurtAppManager.InstallCRD {
		keys = append(keys, []string{
			constants.YurtAppManagerNodepoolsCRDKey,
			constants.YurtAppManagerUniteddeploymentsCRDKey,
		}...)
	}

	values := map[string]string{
		"yurtAppManagerImage": util.GetYurtAppManagerImageByName(yurtCluster, "yurt-app-manager"),
	}

	// reconcile delete
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, key := range keys {
			if err := util.DeleteTemplateWithRender(ctx, template, key, values); err != nil {
				return errors.Wrap(err, "failed to reconcile yurt app manager")
			}
		}
		return nil
	}

	// normal reconcile
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt app manager")
		}
	}
	return nil
}

func (r *YurtClusterReconciler) reconcileNodeLocalDNSCache(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.NodeLocalDNSCacheDaemonSetKey,
		constants.NodeLocalDNSCacheServiceAccountKey,
		constants.NodeLocalDNSCacheUpstreamServiceKey,
		constants.NodeLocalDNSCacheConfigKey,
		constants.NodeLocalDNSCacheServiceKey,
	}

	kubeDNSClusterIP, err := util.GetKubeDNSClusterIP(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile node local dns cache")
	}

	clusterDomain, err := util.GetClusterDomain(ctx)
	if err != nil {
		klog.Warningf("failed to find cluster domain from CoreDNS config, fallback to %v, %v", DefaultClusterDomain, err)
		clusterDomain = DefaultClusterDomain
	}

	values := map[string]string{
		"nodeLocalAddress":  yurtCluster.Spec.NodeLocalDNSCache.NodeLocalAddress,
		"kubeDNSClusterIP":  kubeDNSClusterIP,
		"nodeLocalDNSImage": util.GetNodeLocalDNSImageByName(yurtCluster, "k8s-dns-node-cache"),
		"clusterDomain":     clusterDomain,
	}

	// reconcile delete
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, key := range keys {
			if err := util.DeleteTemplateWithRender(ctx, template, key, values); err != nil {
				return errors.Wrap(err, "failed to reconcile node local dns cache")
			}
		}
		return nil
	}

	// normal reconcile
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile node local dns cache")
		}
	}
	return nil
}

func (r *YurtClusterReconciler) reconcileYurtTunnelServer(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.YurtTunnelServerClusterRoleKey,
		constants.YurtTunnelServerClusterRoleBindingKey,
		constants.YurtTunnelServerServiceAccountKey,
		constants.YurtTunnelServerInternalServiceKey,
		constants.YurtTunnelServerConfigKey,
	}

	values := map[string]string{}

	if len(yurtCluster.Spec.YurtTunnel.PublicIP) != 0 {
		keys = append(keys, constants.YurtTunnelServerServiceWithPublicIPKey)
		keys = append(keys, constants.YurtTunnelServerServiceWithPublicIPNodePortKey)
		values["publicIP"] = yurtCluster.Spec.YurtTunnel.PublicIP
		values["publicPort"] = strconv.Itoa(yurtCluster.Spec.YurtTunnel.PublicPort)
	} else {
		keys = append(keys, constants.YurtTunnelServerServiceKey)
	}

	// reconcile delete
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		// delete all yurt tunnel server deployment/daemonset
		daemonSets := &appsv1.DaemonSetList{}
		if err := r.Client.List(ctx, daemonSets, []client.ListOption{
			client.MatchingLabels{
				"k8s-app": "yurt-tunnel-server",
			},
		}...); err != nil {
			return errors.Wrap(err, "failed to list yurt tunnel server DaemonSets")
		}
		for i := range daemonSets.Items {
			ds := &daemonSets.Items[i]
			if err := r.Client.Delete(ctx, ds); err != nil {
				return errors.Wrapf(err, "failed to delete yurt tunnel server DaemonSet %v", klog.KObj(ds))
			}
		}
		deploy := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: "kube-system", Name: "yurt-tunnel-server"}, deploy); err == nil {
			if err := r.Client.Delete(ctx, deploy); err != nil {
				return errors.Wrapf(err, "failed to delete yurt tunnel server Deployment %v", klog.KObj(deploy))
			}
		}

		// delete rbac etc.
		for _, key := range keys {
			if err := util.DeleteTemplateWithRender(ctx, template, key, values); err != nil {
				return errors.Wrap(err, "failed to reconcile yurt tunnel server")
			}
		}
		return nil
	}

	// normal reconcile
	// apply rbac etc.
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt tunnel server")
		}
	}

	// ensure yurt-tunnel-server for control-plane nodes
	nodes, err := util.GetMasterNodes(ctx, r.Client)
	if err != nil {
		return errors.Wrap(err, "failed to list master nodes")
	}

	for i := range nodes.Items {
		node := nodes.Items[i]
		values := map[string]string{
			"nodeName":              node.Name,
			"edgeNodeLabel":         projectinfo.GetEdgeWorkerLabelKey(),
			"tunnelServerReplicas":  strconv.Itoa(max(yurtCluster.Spec.YurtTunnel.ServerCount, len(nodes.Items))),
			"yurtTunnelServerImage": util.GetYurtComponentImageByName(yurtCluster, "yurt-tunnel-server"),
		}
		if err := util.ApplyTemplateWithRender(ctx, template, constants.YurtTunnelServerDeploymentTemplateKey, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt tunnel server")
		}
	}

	// install addon tunnel servers
	remainCnt := yurtCluster.Spec.YurtTunnel.ServerCount - len(nodes.Items)
	if remainCnt > 0 {
		values := map[string]string{
			"replicas":              strconv.Itoa(remainCnt),
			"edgeNodeLabel":         projectinfo.GetEdgeWorkerLabelKey(),
			"tunnelServerReplicas":  strconv.Itoa(max(yurtCluster.Spec.YurtTunnel.ServerCount, len(nodes.Items))),
			"yurtTunnelServerImage": util.GetYurtComponentImageByName(yurtCluster, "yurt-tunnel-server"),
		}
		if err := util.ApplyTemplateWithRender(ctx, template, constants.YurtTunnelServerDeploymentKey, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt tunnel server")
		}
	}

	return nil
}

func (r *YurtClusterReconciler) reconcileYurtTunnelAgent(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	keys := []string{
		constants.YurtTunnelAgentDaemonSetKey,
	}

	values := map[string]string{
		"edgeNodeLabel":        projectinfo.GetEdgeWorkerLabelKey(),
		"yurtTunnelAgentImage": util.GetYurtComponentImageByName(yurtCluster, "yurt-tunnel-agent"),
	}

	// reconcile delete
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, key := range keys {
			if err := util.DeleteTemplateWithRender(ctx, template, key, values); err != nil {
				return errors.Wrap(err, "failed to reconcile yurt tunnel agent")
			}
		}
		return nil
	}

	// normal reconcile
	for _, key := range keys {
		if err := util.ApplyTemplateWithRender(ctx, template, key, values); err != nil {
			return errors.Wrap(err, "failed to reconcile yurt tunnel agent")
		}
	}
	return nil
}

func (r *YurtClusterReconciler) reconcileStatus(ctx context.Context, template *corev1.ConfigMap, yurtCluster *operatorv1alpha1.YurtCluster) error {
	// reset status
	yurtCluster.Status.Phase = operatorv1alpha1.PhaseConverting
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		yurtCluster.Status.Phase = operatorv1alpha1.PhaseDeleting
	}

	nodeList := &corev1.NodeList{}
	if err := r.Client.List(ctx, nodeList); err != nil {
		return errors.Wrap(err, "failed to list nodes from cluster to check the node convert/revert status")
	}

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if !isNodeConvertOrRevertCompleted(node, yurtCluster) {
			// do not need to requeue, because node controller will trigger this reconcile
			return nil
		}
	}

	// all nodes convert/revert completed
	yurtCluster.Status.Phase = operatorv1alpha1.PhaseSucceed
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(yurtCluster, YurtClusterFinalizer)
	}
	return nil
}

func (r *YurtClusterReconciler) markNodesAsNormal(ctx context.Context) error {
	nodeList := corev1.NodeList{}
	if err := r.List(ctx, &nodeList); err != nil {
		return errors.Wrap(err, "failed to list Nodes from cluster")
	}
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		patchHelper, err := patcher.NewHelper(node, r.Client)
		if err != nil {
			return err
		}
		node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "false"
		if err := patchHelper.Patch(ctx, node); err != nil {
			return errors.Wrapf(err, "failed to patch node %q with %q label", klog.KObj(node), projectinfo.GetEdgeWorkerLabelKey())
		}
	}
	return nil
}

func isNodeConvertOrRevertCompleted(node *corev1.Node, yurtCluster *operatorv1alpha1.YurtCluster) bool {
	isEdge := controllersutil.IsEdgeNode(node)
	if isEdge {
		return controllersutil.IsNodeAlreadyConverted(yurtCluster, node.Name)
	}
	return controllersutil.IsNodeAlreadyReverted(yurtCluster, node.Name)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

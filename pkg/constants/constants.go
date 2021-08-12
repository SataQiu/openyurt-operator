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

package constants

const (
	// SingletonYurtClusterInstanceName defines the global singleton instance name of YurtCluster
	SingletonYurtClusterInstanceName = "openyurt"
	// SkipReconcileAnnotation defines the annotation which marks the object should not be reconciled
	SkipReconcileAnnotation = "operator.openyurt.io/skip-reconcile"
	// NodeControllerClusterRoleBindingName defines the name of the ClusterRoleBinding name for node controller
	NodeControllerClusterRoleBindingName = "system:controller:node-controller"
)

// manifests data keys
// These keys should exactly match with operator manifests template which defined at pkg/util/assets/operator-manifests-template.yaml
const (
	YurtControllerManagerServiceAccountKey          = "yurt-cm-sa"
	YurtControllerManagerClusterRoleKey             = "yurt-cm-cluster-role"
	YurtControllerManagerClusterRoleBindingKey      = "yurt-cm-cluster-role-binding"
	YurtControllerManagerDeploymentKey              = "yurt-cm-deploy"
	YurtTunnelServerClusterRoleKey                  = "yurt-tunnel-server-cluster-role"
	YurtTunnelServerClusterRoleBindingKey           = "yurt-tunnel-server-cluster-role-binding"
	YurtTunnelServerServiceAccountKey               = "yurt-tunnel-server-sa"
	YurtTunnelServerServiceKey                      = "yurt-tunnel-server-svc"
	YurtTunnelServerServiceWithPublicIPKey          = "yurt-tunnel-server-svc-with-public-ip"
	YurtTunnelServerServiceWithPublicIPNodePortKey  = "yurt-tunnel-server-svc-with-public-ip-nodeport"
	YurtTunnelServerInternalServiceKey              = "yurt-tunnel-server-internal-svc"
	YurtTunnelServerConfigKey                       = "yurt-tunnel-server-cfg"
	YurtTunnelServerDeploymentTemplateKey           = "yurt-tunnel-server-deploy-template"
	YurtTunnelServerDeploymentKey                   = "yurt-tunnel-server-deploy"
	YurtTunnelAgentDaemonSetKey                     = "yurt-tunnel-agent-ds"
	YurtHubPodKey                                   = "yurt-hub-pod"
	YurtKubeletConfigKey                            = "yurt-kubelet-config"
	YurtNodeServiceAccountKey                       = "yurt-node-sa"
	YurtNodeClusterRoleKey                          = "yurt-node-cluster-role"
	YurtNodeClusterRoleBindingKey                   = "yurt-node-cluster-role-binding"
	YurtNodeConvertPodKey                           = "yurt-node-convert-pod"
	YurtNodeRevertPodKey                            = "yurt-node-revert-pod"
	YurtNodeAgentServiceAccountKey                  = "yurt-node-agent-sa"
	YurtNodeAgentClusterRoleKey                     = "yurt-node-agent-cluster-role"
	YurtNodeAgentClusterRoleBindingKey              = "yurt-node-agent-cluster-role-binding"
	YurtNodeAgentDaemonSetKey                       = "yurt-node-agent-ds"
	YurtAppManagerNodepoolsCRDKey                   = "yurt-app-manager-nodepools-crd"
	YurtAppManagerUniteddeploymentsCRDKey           = "yurt-app-manager-uniteddeployments-crd"
	YurtAppManagerElectionRoleKey                   = "yurt-app-manager-election-role"
	YurtAppManagerClusterRoleKey                    = "yurt-app-manager-cluster-role"
	YurtAppManagerServiceAccountKey                 = "yurt-app-manager-sa"
	YurtAppManagerRoleBindingKey                    = "yurt-app-manager-role-binding"
	YurtAppManagerClusterRoleBindingKey             = "yurt-app-manager-cluster-role-binding"
	YurtAppManagerSecretKey                         = "yurt-app-manager-secret"
	YurtAppManagerServiceKey                        = "yurt-app-manager-service"
	YurtAppManagerDeploymentKey                     = "yurt-app-manager-deployment"
	YurtAppManagerMutatingWebhookConfigurationKey   = "yurt-app-manager-mwc"
	YurtAppManagerValidatingWebhookConfigurationKey = "yurt-app-manager-vmc"
	NodeLocalDNSCacheServiceAccountKey              = "node-local-dns-cache-sa"
	NodeLocalDNSCacheUpstreamServiceKey             = "node-local-dns-cache-upstream-service"
	NodeLocalDNSCacheConfigKey                      = "node-local-dns-cache-config"
	NodeLocalDNSCacheDaemonSetKey                   = "node-local-dns-cache-ds"
	NodeLocalDNSCacheServiceKey                     = "node-local-dns-cache-service"
)

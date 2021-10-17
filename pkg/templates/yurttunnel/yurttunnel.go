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

package yurttunnel

import (
	_ "embed"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// data keys
	ServerServiceAccount     = "yurt-tunnel-server-service-account"
	ServerClusterRole        = "yurt-tunnel-server-cluster-role"
	ServerClusterRoleBinding = "yurt-tunnel-server-cluster-role-binding"
	ServerService            = "yurt-tunnel-server-svc"
	ServerNodePortService    = "yurt-tunnel-server-nodeport-svc"
	ServerInternalService    = "yurt-tunnel-server-internal-svc"
	ServerConfigMap          = "yurt-tunnel-server-cfg"
	ServerDaemonSet          = "yurt-tunnel-server-daemonset"
	ServerDeployment         = "yurt-tunnel-server-deployment"
	AgentDaemonSet           = "yurt-tunnel-agent-daemonset"
)

var (
	//go:embed yurttunnel.yaml
	TemplateContent string

	NamespacedName = types.NamespacedName{
		Namespace: metav1.NamespaceSystem,
		Name:      "yurt-operator-yurt-tunnel-template",
	}
)

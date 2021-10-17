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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// YurtClusterFinalizer is the finalizer used by the YurtCluster controller to
	// cleanup the cluster resources when a YurtCluster is being deleted.
	YurtClusterFinalizer = "cluster.operator.openyurt.io"

	// SingletonYurtClusterInstanceName defines the global singleton instance name of YurtCluster
	SingletonYurtClusterInstanceName = "cluster"
)

// ImageMeta allows to customize the image used for components that are not
// originated from the OpenYurt release process
type ImageMeta struct {
	// Repository sets the container registry to pull images from.
	// If not set, the ImageRepository defined in YurtClusterSpec will be used instead.
	// +optional
	Repository string `json:"repository,omitempty"`
	// Tag allows to specify a tag for the image.
	// If not set, the tag related to the YurtVersion defined in YurtClusterSpec will be used instead.
	// +optional
	Tag string `json:"tag,omitempty"`
}

// ComponentConfig defines the common config for the yurt components
type ComponentConfig struct {
	// ImageMeta allows to customize the image used for the yurt component
	// +optional
	ImageMeta `json:",inline"`

	// Enabled indicates whether the yurt component has been enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the OpenYurt component.
	// A key in this map is the flag name as it appears on the
	// command line except without leading dash(es).
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`
}

// YurtHubSpec defines the configuration for yurt-hub
type YurtHubSpec struct {
	// Cloud defines the yurt-hub configuration about cloud nodes
	// +optional
	Cloud YurtHubSpecTemplate `json:"cloud,omitempty"`

	// Edge defines the yurt-hub configuration about edge nodes
	// +optional
	Edge YurtHubSpecTemplate `json:"edge,omitempty"`

	// PodManifestsPath defines the path to the directory on edge node containing static pod files
	// +optional
	PodManifestsPath string `json:"podManifestsPath,omitempty"`

	// KubeadmConfPath defines the path to kubelet service conf that is used by kubelet component
	// to join the cluster on the edge node
	// +optional
	KubeadmConfPath string `json:"kubeadmConfPath,omitempty"`
}

// YurtHubSpecTemplate defines the configuration template for yurt-hub
type YurtHubSpecTemplate struct {
	// ComponentConfig defines the common config for the yurt components
	// +optional
	ComponentConfig `json:",inline"`
}

// YurtTunnelSpec defines the configuration for yurt tunnel
type YurtTunnelSpec struct {
	// Server defines the configuration for tunnel server
	// +optional
	Server YurtTunnelServerSpec `json:"server,omitempty"`

	// Agent defines the configuration for tunnel agent
	// +optional
	Agent YurtTunnelAgentSpec `json:"agent,omitempty"`
}

// YurtTunnelServerSpec defines the configuration for tunnel server
type YurtTunnelServerSpec struct {
	// ComponentConfig defines the common config for the yurt components
	// +optional
	ComponentConfig `json:",inline"`

	// ServerCount defines the replicas for the tunnel server Pod.
	// Its value should be greater than or equal to the number of API Server.
	// Operator will automatically override this value if it is less than the number of API Server.
	// +optional
	ServerCount int `json:"serverCount,omitempty"`

	// PublicIP defines the public IP for tunnel server listen on.
	// If this field is empty, the tunnel agent will use NodePort Service to connect to the tunnel server.
	// +optional
	PublicIP string `json:"publicIP,omitempty"`

	// PublicPort defines the public port for tunnel server listen on.
	// +optional
	PublicPort int `json:"publicPort,omitempty"`
}

// YurtTunnelAgentSpec defines the configuration for tunnel agent
type YurtTunnelAgentSpec struct {
	// ComponentConfig defines the common config for the yurt components
	// +optional
	ComponentConfig `json:",inline"`
}

// NodeSet defines a set of Kubernetes nodes.
// It will merge the nodes that selected by Names, NamePattern, and Selector,
// and then remove the nodes that match ExcludedNames and ExcludedNamePattern as the final set of nodes.
type NodeSet struct {
	// Names defines the node names to be selected
	// +optional
	Names []string `json:"names,omitempty"`

	// NamePattern defines the regular expression to select nodes based on node name
	// +optional
	NamePattern string `json:"namePattern,omitempty"`

	// Selector defines the label selector to select nodes
	// +optional
	Selector *corev1.NodeSelector `json:"selector,omitempty"`

	// ExcludedNames defines the node names to be excluded
	// +optional
	ExcludedNames []string `json:"excludedNames,omitempty"`

	// ExcludedNamePattern defines the regular expression to exclude nodes based on node name
	// +optional
	ExcludedNamePattern string `json:"excludedNamePattern,omitempty"`
}

// YurtClusterSpec defines the desired state of YurtCluster
type YurtClusterSpec struct {
	// ImageRepository sets the container registry to pull images from.
	// If empty, `docker.io/openyurt` will be used by default
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// YurtVersion is the target version of OpenYurt
	// +optional
	YurtVersion string `json:"yurtVersion,omitempty"`

	// CloudNodes defines the node set with cloud role.
	// +optional
	CloudNodes NodeSet `json:"cloudNodes,omitempty"`

	// EdgeNodes defines the node set with edge role.
	// +optional
	EdgeNodes NodeSet `json:"edgeNodes,omitempty"`

	// YurtHub defines the configuration for yurt-hub
	// +optional
	YurtHub YurtHubSpec `json:"yurtHub,omitempty"`

	// YurtTunnel defines the configuration for yurt tunnel
	// +optional
	YurtTunnel YurtTunnelSpec `json:"yurtTunnel,omitempty"`
}

// Phase is a string representation of a YurtCluster Phase.
type Phase string

const (
	// PhaseInvalid is the state when the YurtCluster is invalid
	PhaseInvalid = Phase("Invalid")

	// PhaseConverting is the state when the YurtCluster is converting
	PhaseConverting = Phase("Converting")

	// PhaseDeleting is the state when the YurtCluster is deleting
	PhaseDeleting = Phase("Deleting")

	// PhaseSucceed is the state when the YurtCluster is ready
	PhaseSucceed = Phase("Succeed")
)

// YurtClusterStatusFailure defines errors states for YurtCluster objects.
type YurtClusterStatusFailure string

const (
	// InvalidConfigurationYurtClusterError indicates a state that must be fixed before progress can be made.
	InvalidConfigurationYurtClusterError YurtClusterStatusFailure = "InvalidConfiguration"
)

// NodeType defines the type of the node in the edge cluster
type NodeType string

const (
	// CloudNode represents the cloud node
	CloudNode NodeType = "CloudNode"
	// EdgeNode represents the edge node
	EdgeNode NodeType = "EdgeNode"
	// NormalNode represents the normal node (not cloud or edge)
	NormalNode NodeType = "NormalNode"
)

const (
	// NodeConditionReasonCloudNodeConvert represents the reason for cloud node convert
	NodeConditionReasonCloudNodeConvert = "CloudNodeConvert"
	// NodeConditionReasonEdgeNodeConvert represents the reason for edge node convert
	NodeConditionReasonEdgeNodeConvert = "EdgeNodeConvert"
	// NodeConditionReasonNodeRevert represents the reason for normal node revert
	NodeConditionReasonNodeRevert = "NodeRevert"
)

// NodeCondition describes the state of a node at a certain point
type NodeCondition struct {
	// The status for the condition's last transition.
	// +optional
	Status string `json:"status,omitempty"`

	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`

	// The last time this condition was updated.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The generation observed by the node agent controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// YurtClusterStatus defines the observed state of YurtCluster
type YurtClusterStatus struct {
	// Phase represents the current phase of the yurt cluster
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// FailureReason indicates that there is a problem reconciling the state, and
	// will be set to a token value suitable for programmatic interpretation.
	// +optional
	FailureReason *YurtClusterStatusFailure `json:"failureReason,omitempty"`

	// FailureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// NodeConditions holds the info about node conditions
	// +optional
	NodeConditions map[string]NodeCondition `json:"nodeConditions,omitempty"`

	// The generation observed by the operator controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName="yc",scope="Cluster"
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`

// YurtCluster is the Schema for the yurtclusters API
type YurtCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtClusterSpec   `json:"spec,omitempty"`
	Status YurtClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// YurtClusterList contains a list of YurtCluster
type YurtClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtCluster{}, &YurtClusterList{})
}

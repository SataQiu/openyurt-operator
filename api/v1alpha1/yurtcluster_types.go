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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// YurtClusterSpec defines the desired state of YurtCluster
type YurtClusterSpec struct {
	// ImageRepository defines the image repository address
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// Version defines the version to deploy
	// +optional
	Version string `json:"version,omitempty"`

	// YurtHub defines the configuration for yurt hub
	// +optional
	YurtHub YurtHub `json:"yurtHub,omitempty"`

	// YurtTunnel defines the configuration for yurt tunnel
	// +optional
	YurtTunnel YurtTunnel `json:"yurtTunnel,omitempty"`

	// YurtAppManager defines the configuration for yurt app manager
	// +optional
	YurtAppManager YurtAppManager `json:"yurtAppManager,omitempty"`

	// NodeLocalDNSCache defines the configuration for node local dns cache
	// +optional
	NodeLocalDNSCache NodeLocalDNSCache `json:"nodeLocalDNSCache,omitempty"`
}

// YurtTunnel defines the configuration for yurt tunnel
type YurtTunnel struct {
	// Enable defines whether to enable the yurt tunnel component
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// ServerCount defines the replicas for the tunnel server deployment
	// Operator will automatically override this value if it is less than the number of master node
	// +optional
	ServerCount int `json:"serverCount,omitempty"`

	// PublicIP defines the public IP for tunnel server listen on
	// +optional
	PublicIP string `json:"publicIP,omitempty"`

	// PublicPort defines the public port for tunnel server listen on
	// +optional
	PublicPort int `json:"publicPort,omitempty"`
}

// YurtAppManager defines the configuration for yurt app manager
type YurtAppManager struct {
	// InstallCRD defines whether to install the yurt app manager crd
	// +optional
	InstallCRD *bool `json:"installCRD,omitempty"`

	// EnableController defines whether to enable the yurt app manager controller
	// +optional
	EnableController *bool `json:"enableController,omitempty"`

	// Version defines the version of yurt app manager
	// +optional
	Version string `json:"version,omitempty"`
}

// YurtHub defines the configuration for yurt-hub
type YurtHub struct {
	// EnableResourceFilter enables to filter response that comes back from reverse proxy
	// +optional
	EnableResourceFilter *bool `json:"enableResourceFilter,omitempty"`

	// AccessServerThroughHub enables pods access kube-apiserver through yurthub or not
	// +optional
	AccessServerThroughHub *bool `json:"accessServerThroughHub,omitempty"`

	// AutoRestartNodePod represents whether to automatically restart the pod after yurt-hub added or removed
	// +optional
	AutoRestartNodePod *bool `json:"autoRestartNodePod,omitempty"`
}

// NodeLocalDNSCache defines the configuration for node local dns cache
type NodeLocalDNSCache struct {
	// Enable defines whether to enable the node local dns cache
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// UpstreamPublicIP defines the public IP for cloud coredns listen on
	// +optional
	UpstreamPublicIP string `json:"upstreamPublicIP,omitempty"`

	// UpstreamPublicPort defines the public port for cloud coredns listen on
	// +optional
	UpstreamPublicPort int `json:"upstreamPublicPort,omitempty"`

	// NodeLocalAddress defines the IP that the node local dns will bind to
	// +optional
	NodeLocalAddress string `json:"nodeLocalAddress,omitempty"`

	// NodeLocalDNSImage defines the image full path for node local dns
	// +optional
	NodeLocalDNSImage string `json:"image,omitempty"`
}

// Phase is a string representation of a YurtCluster Phase.
type Phase string

const (
	// PhaseConverting is the state when the YurtCluster is converting
	PhaseConverting = Phase("Converting")

	// PhaseDeleting is the state when the YurtCluster is deleting
	PhaseDeleting = Phase("Deleting")

	// PhaseSucceed is the state when the YurtCluster is ready
	PhaseSucceed = Phase("Succeed")
)

// YurtClusterStatus defines the observed state of YurtCluster
type YurtClusterStatus struct {
	// Phase represents the current phase of the openyurt cluster
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// NodeConvertConditions holds the info about node convert conditions
	// +optional
	NodeConvertConditions map[string]NodeConvertCondition `json:"nodeConvertConditions,omitempty"`

	// LastUpdateTime is the latest update time by the operator
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastUpdateTime is the latest update spec by the operator
	// +optional
	LastUpdateSpec YurtClusterSpec `json:"lastUpdateSpec,omitempty"`
}

// NodeConvertCondition describes the state of a node at a certain point.
type NodeConvertCondition struct {
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
	// AckLastUpdateTime is the latest ack for the latest update time set by the operator
	// +optional
	AckLastUpdateTime metav1.Time `json:"ackLastUpdateTime,omitempty"`
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

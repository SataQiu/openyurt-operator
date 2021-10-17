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
	"k8s.io/utils/pointer"
)

const (
	// DefaultYurtVersion defines the default yurt version (image tag)
	DefaultYurtVersion = "v0.5.0"

	// DefaultYurtImageRepository defines the default repository for the yurt component images
	DefaultYurtImageRepository = "docker.io/openyurt"

	// DefaultPodManifestsPath defines the default path of Pod manifests
	DefaultPodManifestsPath = "/etc/kubernetes/manifests"

	// DefaultKubeadmConfPath defines the default path to kubelet service conf
	DefaultKubeadmConfPath = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"

	// DefaultYurtTunnelPublicPort defines the default public port for tunnel server
	DefaultYurtTunnelPublicPort = 32502
)

var (
	// TrueBoolPrt is a bool pointer to true
	TrueBoolPrt = pointer.BoolPtr(true)

	// FalseBoolPrt is a bool pointer to false
	FalseBoolPrt = pointer.BoolPtr(false)
)

// Complete completes all the required fields with the default value
func (in *YurtCluster) Complete() {
	if len(in.Spec.ImageRepository) == 0 {
		in.Spec.ImageRepository = DefaultYurtImageRepository
	}

	if len(in.Spec.YurtVersion) == 0 {
		in.Spec.YurtVersion = DefaultYurtVersion
	}

	if in.Spec.YurtHub.Cloud.Enabled == nil {
		in.Spec.YurtHub.Cloud.Enabled = TrueBoolPrt
	}

	if in.Spec.YurtHub.Edge.Enabled == nil {
		in.Spec.YurtHub.Edge.Enabled = TrueBoolPrt
	}

	if len(in.Spec.YurtHub.PodManifestsPath) == 0 {
		in.Spec.YurtHub.PodManifestsPath = DefaultPodManifestsPath
	}

	if len(in.Spec.YurtHub.KubeadmConfPath) == 0 {
		in.Spec.YurtHub.KubeadmConfPath = DefaultKubeadmConfPath
	}

	if in.Spec.YurtTunnel.Server.Enabled == nil {
		in.Spec.YurtTunnel.Server.Enabled = TrueBoolPrt
	}

	if in.Spec.YurtTunnel.Server.PublicPort == 0 {
		in.Spec.YurtTunnel.Server.PublicPort = DefaultYurtTunnelPublicPort
	}

	if in.Spec.YurtTunnel.Agent.Enabled == nil {
		in.Spec.YurtTunnel.Agent.Enabled = TrueBoolPrt
	}
}

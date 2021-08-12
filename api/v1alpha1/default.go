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

const (
	// DefaultYurtVersion defines the default yurt version (image tag) to install
	DefaultYurtVersion = "v0.4.1"
	// DefaultYurtImageRepository defines the default repository for the yurt images
	DefaultYurtImageRepository = "docker.io/openyurt"
	// DefaultNodeLocalDNSCacheNodeLocalAddress defines the default dummy IP for node local dns
	DefaultNodeLocalDNSCacheNodeLocalAddress = "169.254.20.10"
	// DefaultYurtTunnelPublicPort defines the default public port for tunnel server
	DefaultYurtTunnelPublicPort = 32502
)

var (
	DefaultYurtHubEnableResourceFilter    = false
	DefaultYurtHubAccessServerThroughHub  = false
	DefaultYurtHubAutoRestartNodePod      = true
	DefaultYurtTunnelEnable               = true
	DefaultYurtAppManagerInstallCRD       = true
	DefaultYurtAppManagerEnableController = true
	DefaultNodeLocalDNSCacheEnable        = true
)

// ApplyDefault fills all the required field with default value.
func (o *YurtCluster) ApplyDefault() {
	if len(o.Spec.Version) == 0 {
		o.Spec.Version = DefaultYurtVersion
	}
	if len(o.Spec.ImageRepository) == 0 {
		o.Spec.ImageRepository = DefaultYurtImageRepository
	}
	if o.Spec.YurtHub.EnableResourceFilter == nil {
		o.Spec.YurtHub.EnableResourceFilter = &DefaultYurtHubEnableResourceFilter
	}
	if o.Spec.YurtHub.AccessServerThroughHub == nil {
		o.Spec.YurtHub.AccessServerThroughHub = &DefaultYurtHubAccessServerThroughHub
	}
	if o.Spec.YurtHub.AutoRestartNodePod == nil {
		o.Spec.YurtHub.AutoRestartNodePod = &DefaultYurtHubAutoRestartNodePod
	}
	if o.Spec.YurtTunnel.Enable == nil {
		o.Spec.YurtTunnel.Enable = &DefaultYurtTunnelEnable
	}
	if o.Spec.YurtTunnel.PublicPort == 0 {
		o.Spec.YurtTunnel.PublicPort = DefaultYurtTunnelPublicPort
	}
	if o.Spec.YurtAppManager.InstallCRD == nil {
		o.Spec.YurtAppManager.InstallCRD = &DefaultYurtAppManagerInstallCRD
	}
	if o.Spec.YurtAppManager.EnableController == nil {
		o.Spec.YurtAppManager.EnableController = &DefaultYurtAppManagerEnableController
	}
	if len(o.Spec.YurtAppManager.Version) == 0 {
		o.Spec.YurtAppManager.Version = o.Spec.Version
	}
	if o.Spec.NodeLocalDNSCache.Enable == nil {
		o.Spec.NodeLocalDNSCache.Enable = &DefaultNodeLocalDNSCacheEnable
	}
	if len(o.Spec.NodeLocalDNSCache.NodeLocalAddress) == 0 {
		o.Spec.NodeLocalDNSCache.NodeLocalAddress = DefaultNodeLocalDNSCacheNodeLocalAddress
	}
}

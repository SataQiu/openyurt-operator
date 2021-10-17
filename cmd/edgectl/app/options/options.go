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

package options

import (
	"os"
	"time"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
)

const (
	defaultHealthCheckTimeout = time.Minute * 5
)

// BaseOptions defines the common options for convert/revert operation
type BaseOptions struct {
	NodeName              string
	YurtWorkDir           string
	PodManifestsPath      string
	KubeletConfigPath     string
	KubeletKubeConfigPath string
	APIServerAddress      string
	KubeletHealthPort     int
	Force                 bool
	HealthCheckTimeout    time.Duration
}

// AddBaseOptionsFlags adds the flags for base options
func AddBaseOptionsFlags(flagSet *flag.FlagSet, opt *BaseOptions) {
	flagSet.StringVar(&opt.NodeName, "node-name", opt.NodeName, "The node name of the current kubernetes node.")
	flagSet.StringVar(&opt.YurtWorkDir, "yurt-work-dir", opt.YurtWorkDir, "The directory to store yurt config files.")
	flagSet.StringVar(&opt.PodManifestsPath, "pod-manifests-path", opt.PodManifestsPath, "The directory on node containing kubernetes manifests files.")
	flagSet.StringVar(&opt.KubeletConfigPath, "kubelet-config-path", opt.KubeletConfigPath, "The path to kubelet config file.")
	flagSet.StringVar(&opt.KubeletKubeConfigPath, "kubelet-kube-config-path", opt.KubeletKubeConfigPath, "The path to kubelet kubeconfig file.")
	flagSet.StringVar(&opt.APIServerAddress, "kube-apiserver-address", opt.APIServerAddress, "The kubernetes api server to connect.")
	flagSet.IntVar(&opt.KubeletHealthPort, "kubelet-health-port", opt.KubeletHealthPort, "The port for kubelet health check.")
	flagSet.BoolVar(&opt.Force, "force", opt.Force, "Force to run convert/revert even node is not ready")
	flagSet.DurationVar(&opt.HealthCheckTimeout, "trans-health-check-timeout", opt.HealthCheckTimeout, "The timeout to check kubelet/edge-hub health status.")
}

// ConvertOptions has the information required by sub command convert node to edge
type ConvertOptions struct {
	BaseOptions

	NodeType string
}

// NewConvertOptions returns a new ConvertOptions
func NewConvertOptions() *ConvertOptions {
	hostname, _ := os.Hostname()
	return &ConvertOptions{
		BaseOptions: BaseOptions{
			NodeName:              hostname,
			YurtWorkDir:           "/var/lib/openyurt",
			PodManifestsPath:      "/etc/kubernetes/manifests",
			KubeletConfigPath:     "/var/lib/kubelet/config.yaml",
			KubeletKubeConfigPath: "/etc/kubernetes/kubelet.conf",
			KubeletHealthPort:     10248,
			HealthCheckTimeout:    defaultHealthCheckTimeout,
		},
	}
}

// AddAllFlags adds all flags for convert options
func (opt *ConvertOptions) AddAllFlags(flagSet *flag.FlagSet) {
	// add base flags
	AddBaseOptionsFlags(flagSet, &opt.BaseOptions)
	// add custom flags
	flagSet.StringVar(&opt.NodeType, "node-type", opt.NodeType, "The node type of the current kubernetes node.")
}

// Validate validates the convert options
func (opt *ConvertOptions) Validate() error {
	if opt.NodeType != string(operatorv1alpha1.CloudNode) && opt.NodeType != string(operatorv1alpha1.EdgeNode) {
		return errors.Errorf("--node-type is required and should be one of [%s %s]",
			operatorv1alpha1.CloudNode, operatorv1alpha1.EdgeNode)
	}
	return nil
}

// RevertOptions has the information required by sub command revert node to edge
type RevertOptions struct {
	BaseOptions
}

// NewRevertOptions returns a new RevertOptions
func NewRevertOptions() *RevertOptions {
	hostname, _ := os.Hostname()
	return &RevertOptions{
		BaseOptions: BaseOptions{
			NodeName:              hostname,
			YurtWorkDir:           "/var/lib/openyurt",
			PodManifestsPath:      "/etc/kubernetes/manifests",
			KubeletConfigPath:     "/var/lib/kubelet/config.yaml",
			KubeletKubeConfigPath: "/etc/kubernetes/kubelet.conf",
			KubeletHealthPort:     10248,
			HealthCheckTimeout:    defaultHealthCheckTimeout,
		},
	}
}

// AddAllFlags adds all flags for revert options
func (opt *RevertOptions) AddAllFlags(flagSet *flag.FlagSet) {
	// add base flags
	AddBaseOptionsFlags(flagSet, &opt.BaseOptions)
}

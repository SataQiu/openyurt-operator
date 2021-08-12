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
	"flag"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Options has all the params needed to run the OpenYurt operator
type Options struct {
	MetricsBindAddr            string
	YurtAgentImage             string
	YurtNodeImage              string
	YurtAgentImagePullPolicy   string
	YurtNodeImagePullPolicy    string
	YurtTaskHealthCheckTimeout time.Duration
}

// NewDefaultOptions builds a default options for dns manager
func NewDefaultOptions() *Options {
	return &Options{
		MetricsBindAddr:            ":8080",
		YurtAgentImage:             "registry.cn-hangzhou.aliyuncs.com/ecp_builder/openyurt-agent:v1.0.0",
		YurtNodeImage:              "registry.cn-hangzhou.aliyuncs.com/ecp_builder/openyurt-node:v1.0.0",
		YurtAgentImagePullPolicy:   string(corev1.PullAlways),
		YurtNodeImagePullPolicy:    string(corev1.PullIfNotPresent),
		YurtTaskHealthCheckTimeout: time.Minute * 5,
	}
}

// ParseFlags parses the command-line flags and return the final options
func (o *Options) ParseFlags() *Options {
	flag.StringVar(&o.MetricsBindAddr, "metrics-addr", o.MetricsBindAddr, "The address the metric endpoint binds to.")
	flag.StringVar(&o.YurtAgentImage, "yurt-agent-image", o.YurtAgentImage, "The image full path for yurt agent.")
	flag.StringVar(&o.YurtNodeImage, "yurt-node-image", o.YurtNodeImage, "The image full path for yurt node.")
	flag.StringVar(&o.YurtAgentImagePullPolicy, "yurt-agent-image-pull-policy", o.YurtAgentImagePullPolicy, "The image pull policy for yurt agent.")
	flag.StringVar(&o.YurtNodeImagePullPolicy, "yurt-node-image-pull-policy", o.YurtNodeImagePullPolicy, "The image pull policy for yurt node.")
	flag.DurationVar(&o.YurtTaskHealthCheckTimeout, "yurt-task-health-check-timeout", o.YurtTaskHealthCheckTimeout, "The timeout for health check in convert or revert tasks.")
	flag.Parse()
	return o
}

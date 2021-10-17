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
	"os"
	"time"
)

// Options has all the params needed to run the OpenYurt operator
type Options struct {
	NodeName                string
	MetricsBindAddr         string
	APIServerAddress        string
	TransHealthCheckTimeout time.Duration
}

// NewDefaultOptions builds a default options for dns manager
func NewDefaultOptions() *Options {
	nodeName, _ := os.Hostname()
	return &Options{
		NodeName:                nodeName,
		MetricsBindAddr:         "0", // set 0 to disable metrics
		TransHealthCheckTimeout: time.Minute * 5,
	}
}

// ParseFlags parses the command-line flags and return the final options
func (o *Options) ParseFlags() *Options {
	flag.StringVar(&o.NodeName, "node-name", o.NodeName, "The node name used by the node to do convert/revert.")
	flag.StringVar(&o.MetricsBindAddr, "metrics-addr", o.MetricsBindAddr, "The address the metric endpoint binds to.")
	flag.StringVar(&o.APIServerAddress, "apiserver-address", o.APIServerAddress, "The kubernetes api server to connect.")
	flag.DurationVar(&o.TransHealthCheckTimeout, "trans-health-check-timeout", o.TransHealthCheckTimeout, "The timeout for health check in convert or revert tasks.")
	flag.Parse()
	return o
}

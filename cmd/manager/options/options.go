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
)

// Options has all the params needed to run the OpenYurt operator
type Options struct {
	MetricsBindAddr string
}

// NewDefaultOptions builds a default options for dns manager
func NewDefaultOptions() *Options {
	return &Options{
		MetricsBindAddr: ":8080",
	}
}

// ParseFlags parses the command-line flags and return the final options
func (o *Options) ParseFlags() *Options {
	flag.StringVar(&o.MetricsBindAddr, "metrics-addr", o.MetricsBindAddr, "The address the metric endpoint binds to.")
	flag.Parse()
	return o
}

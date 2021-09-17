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

package main

import (
	"context"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openyurtio/openyurt-operator/cmd/agent/options"
	controllers "github.com/openyurtio/openyurt-operator/pkg/controllers/agent"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/util"
	// +kubebuilder:scaffold:imports
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	klog.InitFlags(nil)
}

func main() {
	opt := options.NewDefaultOptions().ParseFlags()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// init client
	restConfig, err := kclient.GetConfig(util.GetAPIServerAddress(opt.APIServerAddress))
	if err != nil {
		setupLog.Error(err, "failed to load in-cluster config")
		os.Exit(1)
	}
	kclient.InitializeKubeClient(restConfig)

	mgr, err := ctrl.NewManager(kclient.Config(), ctrl.Options{
		Scheme:             kclient.Scheme,
		MetricsBindAddress: opt.MetricsBindAddr,
		Port:               9443,
		LeaderElection:     false,
		LeaderElectionID:   "8f94aa2y.openyurt.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start agent")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupReconcilers(ctx, mgr, opt)

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting agent")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running agent")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager, opt *options.Options) {
	if err := (&controllers.NodeReconciler{
		Client:  mgr.GetClient(),
		Log:     ctrl.Log.WithName("controllers").WithName("Node"),
		Scheme:  mgr.GetScheme(),
		Options: opt,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Node")
		os.Exit(1)
	}
}

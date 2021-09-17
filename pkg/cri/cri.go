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

package cri

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt-operator/pkg/cri/util"
)

var (
	Timeout          = 2 * time.Second
	RuntimeEndpoints = []string{"unix:///var/run/dockershim.sock", "unix:///run/containerd/containerd.sock", "unix:///run/crio/crio.sock"}
)

var (
	runtimeServiceClientLock sync.Mutex
	runtimeServiceClient     pb.RuntimeServiceClient
	runtimeServiceClientConn *grpc.ClientConn
)

// GetRuntimeClient returns a runtime client for cri
func GetRuntimeClient() (pb.RuntimeServiceClient, *grpc.ClientConn, error) {
	if runtimeServiceClient != nil && runtimeServiceClientConn != nil {
		return runtimeServiceClient, runtimeServiceClientConn, nil
	}

	runtimeServiceClientLock.Lock()
	defer runtimeServiceClientLock.Unlock()

	// Set up a connection to the server.
	conn, err := getConnection(RuntimeEndpoints)
	if err != nil {
		return nil, nil, errors.Wrap(err, "connect")
	}
	runtimeClient := pb.NewRuntimeServiceClient(conn)
	runtimeServiceClient, runtimeServiceClientConn = runtimeClient, conn
	return runtimeServiceClient, runtimeServiceClientConn, nil
}

func getConnection(endPoints []string) (*grpc.ClientConn, error) {
	if endPoints == nil || len(endPoints) == 0 {
		return nil, fmt.Errorf("endpoint is not set")
	}
	endPointsLen := len(endPoints)
	var conn *grpc.ClientConn
	for indx, endPoint := range endPoints {
		klog.V(4).Infof("connect using endpoint '%s' with '%s' timeout", endPoint, Timeout)
		addr, dialer, err := util.GetAddressAndDialer(endPoint)
		if err != nil {
			if indx == endPointsLen-1 {
				return nil, err
			}
			klog.Error(err)
			continue
		}
		conn, err = grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(Timeout), grpc.WithContextDialer(dialer))
		if err != nil {
			errMsg := errors.Wrapf(err, "connect endpoint '%s', make sure you are running as root and the endpoint has been started", endPoint)
			if indx == endPointsLen-1 {
				return nil, errMsg
			}
			klog.V(4).Info(errMsg)
		} else {
			klog.Infof("connected successfully using endpoint: %s", endPoint)
			break
		}
	}
	return conn, nil
}

func closeConnection(conn *grpc.ClientConn) error {
	if conn == nil {
		return nil
	}
	return conn.Close()
}

// StopAllReadyPodsExceptYurt tries to stop all the ready Pods except yurt Pods.
// These Pods will be re-launched by kubelet later.
func StopAllReadyPodsExceptYurt(ctx context.Context) error {
	runtimeClient, runtimeConn, err := GetRuntimeClient()
	if err != nil {
		return errors.Wrap(err, "failed to init runtime client")
	}
	defer closeConnection(runtimeConn)

	pods, err := runtimeClient.ListPodSandbox(ctx, &pb.ListPodSandboxRequest{
		Filter: &pb.PodSandboxFilter{
			State: &pb.PodSandboxStateValue{
				State: pb.PodSandboxState_SANDBOX_READY,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to list Pods")
	}

	for _, pod := range pods.GetItems() {
		if strings.HasPrefix(pod.Metadata.Name, "yurt") || strings.HasPrefix(pod.Metadata.Name, "openyurt") {
			klog.Infof("skip yurt system Pod %v/%v", pod.Metadata.Namespace, pod.Metadata.Name)
			continue
		}

		if _, err := runtimeClient.StopPodSandbox(ctx, &pb.StopPodSandboxRequest{
			PodSandboxId: pod.Id,
		}); err != nil {
			klog.Errorf("failed to stop Pod %v/%v", pod.Metadata.Namespace, pod.Metadata.Name)
			continue
		}
		klog.Infof("stop Pod %v/%v successfully", pod.Metadata.Namespace, pod.Metadata.Name)
	}

	return nil
}

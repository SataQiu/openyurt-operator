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

package controllers

import (
	corev1 "k8s.io/api/core/v1"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/projectinfo"
)

// IsNodeAlreadyConverted returns true if the Node is already converted
func IsNodeAlreadyConverted(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) bool {
	condition, ok := yurtCluster.Status.NodeConvertConditions[nodeName]
	if !ok {
		return false
	}
	if condition.Status != "True" {
		return false
	}
	if condition.Reason != "NodeConvert" {
		return false
	}
	if condition.AckLastUpdateTime.UnixNano() < yurtCluster.Status.LastUpdateTime.UnixNano() {
		return false
	}
	return true
}

// IsNodeAlreadyReverted returns true if the Node is already reverted
func IsNodeAlreadyReverted(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) bool {
	condition, ok := yurtCluster.Status.NodeConvertConditions[nodeName]
	if !ok {
		// no conditions, treat it as a normal node
		return true
	}
	if condition.Status != "True" {
		return false
	}
	if condition.Reason != "NodeRevert" {
		return false
	}
	if condition.AckLastUpdateTime.UnixNano() < yurtCluster.Status.LastUpdateTime.UnixNano() {
		return false
	}
	return true
}

// IsEdgeNode returns true if the Node has edge label
func IsEdgeNode(node *corev1.Node) bool {
	isEdgeNode, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	return ok && isEdgeNode == "true"
}

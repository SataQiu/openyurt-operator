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

package util

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"
	schedulingcorev1 "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
)

// GetNodeType returns the type of the node in edge cluster
func GetNodeType(node *corev1.Node, yurtCluster *operatorv1alpha1.YurtCluster) operatorv1alpha1.NodeType {
	if isCloudNode(node, yurtCluster) {
		return operatorv1alpha1.CloudNode
	}
	if isEdgeNode(node, yurtCluster) {
		return operatorv1alpha1.EdgeNode
	}
	return operatorv1alpha1.NormalNode
}

// IsNodeConvertOrRevertCompleted returns true when the node is converted or reverted completely
func IsNodeConvertOrRevertCompleted(node *corev1.Node, yurtCluster *operatorv1alpha1.YurtCluster) bool {
	if !yurtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return isNodeAlreadyReverted(yurtCluster, node.Name)
	}
	switch GetNodeType(node, yurtCluster) {
	case operatorv1alpha1.CloudNode:
		return isCloudNodeAlreadyConverted(yurtCluster, node.Name)
	case operatorv1alpha1.EdgeNode:
		return isEdgeNodeAlreadyConverted(yurtCluster, node.Name)
	default:
		return isNodeAlreadyReverted(yurtCluster, node.Name)
	}
}

// isCloudNode returns true when the node is a cloud node
func isCloudNode(node *corev1.Node, yurtCluster *operatorv1alpha1.YurtCluster) bool {
	return inNodeSet(node, yurtCluster.Spec.CloudNodes)
}

// isCloudNodeAlreadyConverted returns true if the cloud node is already converted
func isCloudNodeAlreadyConverted(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) bool {
	condition, ok := yurtCluster.Status.NodeConditions[nodeName]
	if !ok {
		return false
	}
	if condition.Status != "True" {
		return false
	}
	if condition.Reason != operatorv1alpha1.NodeConditionReasonCloudNodeConvert {
		return false
	}
	if condition.ObservedGeneration != yurtCluster.Generation {
		return false
	}
	return true
}

// isEdgeNode returns true when the node is an edge node
func isEdgeNode(node *corev1.Node, yurtCluster *operatorv1alpha1.YurtCluster) bool {
	return inNodeSet(node, yurtCluster.Spec.EdgeNodes)
}

// isEdgeNodeAlreadyConverted returns true if the edge node is already converted
func isEdgeNodeAlreadyConverted(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) bool {
	condition, ok := yurtCluster.Status.NodeConditions[nodeName]
	if !ok {
		return false
	}
	if condition.Status != "True" {
		return false
	}
	if condition.Reason != operatorv1alpha1.NodeConditionReasonEdgeNodeConvert {
		return false
	}
	if condition.ObservedGeneration != yurtCluster.Generation {
		return false
	}
	return true
}

// isNodeAlreadyReverted returns true if the node is already reverted
func isNodeAlreadyReverted(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string) bool {
	condition, ok := yurtCluster.Status.NodeConditions[nodeName]
	if !ok {
		// no conditions, treat it as a normal node
		return true
	}
	if condition.Status != "True" {
		return false
	}
	if condition.Reason != operatorv1alpha1.NodeConditionReasonNodeRevert {
		return false
	}
	if condition.ObservedGeneration != yurtCluster.Generation {
		return false
	}
	return true
}

// inNodeSet returns true when the node is selected by the node set
func inNodeSet(node *corev1.Node, nodeSet operatorv1alpha1.NodeSet) bool {
	selected := false

	for _, name := range nodeSet.Names {
		if node.Name == name {
			selected = true
		}
	}

	if !selected && len(nodeSet.NamePattern) > 0 {
		match, err := regexp.MatchString(nodeSet.NamePattern, node.Name)
		if err != nil {
			klog.Errorf("failed to do regex match %v, pattern: %q, string: %q", err, nodeSet.NamePattern, node.Name)
		}
		if match {
			selected = true
		}
	}

	if !selected {
		match, err := schedulingcorev1.MatchNodeSelectorTerms(node, nodeSet.Selector)
		if err != nil {
			klog.Errorf("failed to check nodeSelector %v against node %v, %v", nodeSet.Selector, klog.KObj(node), err)
		}
		if match {
			selected = true
		}
	}

	if !selected {
		return false
	}

	for _, name := range nodeSet.ExcludedNames {
		if node.Name == name {
			selected = false
		}
	}

	if selected && len(nodeSet.ExcludedNamePattern) > 0 {
		match, err := regexp.MatchString(nodeSet.ExcludedNamePattern, node.Name)
		if err != nil {
			klog.Errorf("failed to do regex match %v, pattern: %q, string: %q", err, nodeSet.ExcludedNamePattern, node.Name)
		}
		if match {
			selected = false
		}
	}

	return selected
}

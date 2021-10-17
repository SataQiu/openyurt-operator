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
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
)

const (
	// SkipReconcileAnnotation defines the annotation which marks the object should not be reconciled
	SkipReconcileAnnotation = "operator.openyurt.io/skip-reconcile"

	// EdgeNodeTaintKey defines the taint key for edge node
	EdgeNodeTaintKey = "node-role.openyurt.io/edge"

	// EdgeNodeTaintEffect defines the taint effect for edge node
	EdgeNodeTaintEffect = "NoSchedule"

	// ControlPlaneLabel defines the label for control-plane node
	ControlPlaneLabel = "node-role.kubernetes.io/master"
)

var (
	decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
)

// ApplyTemplateWithRender renders and applies the manifests into cluster.
func ApplyTemplateWithRender(ctx context.Context, template *corev1.ConfigMap, dataKey string, values map[string]string) error {
	yamlContent, ok := template.Data[dataKey]
	if !ok {
		return errors.Errorf("cannot find key %q in manifests template %q", dataKey, klog.KObj(template))
	}
	yamlContentCompleted, err := RenderTemplate(yamlContent, values)
	if err != nil {
		return errors.Wrapf(err, "failed to render yaml content of %q in manifests template %q", dataKey, klog.KObj(template))
	}
	if err := Apply(ctx, yamlContentCompleted); err != nil {
		return errors.Wrapf(err, "failed to apply yaml content of %q in manifests template %q", dataKey, klog.KObj(template))
	}
	return nil
}

// Apply applies the manifests into cluster.
func Apply(ctx context.Context, yamlContent string) error {
	obj := &unstructured.Unstructured{}
	_, gvk, err := decUnstructured.Decode([]byte(yamlContent), nil, obj)
	if err != nil {
		return err
	}

	if ShouldSkip(ctx, yamlContent) {
		klog.Warningf("skip apply %q %q as it contains %q annotation with 'true' value",
			obj.GetObjectKind().GroupVersionKind().Kind,
			klog.KObj(obj),
			SkipReconcileAnnotation)
		return nil
	}

	klog.Infof("apply %q %q", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(kclient.DiscoveryClient()))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var dr dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		dr = kclient.DynamicClient().Resource(mapping.Resource).Namespace(obj.GetNamespace())
	} else {
		dr = kclient.DynamicClient().Resource(mapping.Resource)
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	force := true
	_, err = dr.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
		Force:        &force,
		FieldManager: "last-applied",
	})

	return err
}

// DeleteTemplateWithRender renders and deletes the manifests from cluster.
func DeleteTemplateWithRender(ctx context.Context, template *corev1.ConfigMap, dataKey string, values map[string]string) error {
	yamlContent, ok := template.Data[dataKey]
	if !ok {
		return errors.Errorf("cannot find key %q in manifests template %q", dataKey, klog.KObj(template))
	}
	yamlContentCompleted, err := RenderTemplate(yamlContent, values)
	if err != nil {
		return errors.Wrapf(err, "failed to render yaml content of %q in manifests template %q", dataKey, klog.KObj(template))
	}
	if err := Delete(ctx, yamlContentCompleted); err != nil {
		return errors.Wrapf(err, "failed to delete yaml content of %q in manifests template %q", dataKey, klog.KObj(template))
	}
	return nil
}

// Delete deletes the manifests from cluster.
func Delete(ctx context.Context, yamlContent string) error {
	obj := &unstructured.Unstructured{}
	_, gvk, err := decUnstructured.Decode([]byte(yamlContent), nil, obj)
	if err != nil {
		return err
	}

	if ShouldSkip(ctx, yamlContent) {
		klog.Warningf("skip delete %q %q as it contains %q annotation with 'true' value",
			obj.GetObjectKind().GroupVersionKind().Kind,
			klog.KObj(obj),
			SkipReconcileAnnotation)
		return nil
	}

	klog.Infof("delete %q %q", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(kclient.DiscoveryClient()))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var dr dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		dr = kclient.DynamicClient().Resource(mapping.Resource).Namespace(obj.GetNamespace())
	} else {
		dr = kclient.DynamicClient().Resource(mapping.Resource)
	}

	if _, err := Get(ctx, yamlContent); err != nil {
		// maybe has been deleted already
		return nil //nolint
	}

	return dr.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
}

// ShouldSkip returns true if the object contains SkipReconcileAnnotation annotation with true value.
func ShouldSkip(ctx context.Context, yamlContent string) bool {
	obj, err := Get(ctx, yamlContent)
	if err != nil {
		klog.V(4).Infof("unable to check whether the resource should be skipped to reconcile, %v", err)
		return false
	}

	annotations := obj.GetAnnotations()
	if v, ok := annotations[SkipReconcileAnnotation]; ok && v == "true" {
		return true
	}

	return false
}

// Get gets object based on the yaml manifests from cluster.
func Get(ctx context.Context, yamlContent string) (*unstructured.Unstructured, error) {
	obj, err := YamlToObject([]byte(yamlContent))
	if err != nil {
		return nil, err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(obj.GetObjectKind().GroupVersionKind())
	request := kclient.DynamicClient().Resource(gvr)
	if len(obj.GetNamespace()) != 0 {
		return request.Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	}

	return request.Get(ctx, obj.GetName(), metav1.GetOptions{})
}

// YamlToObject deserializes object in yaml format to a client.Object
func YamlToObject(yamlContent []byte) (client.Object, error) {
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	obj, _, err := decode(yamlContent, nil, nil)
	if err != nil {
		return nil, err
	}
	o, ok := obj.(client.Object)
	if !ok {
		return nil, errors.Errorf("cannot convert runtime.Object %v into client.Object", obj)
	}
	return o, nil
}

// RenderTemplate fills out the manifest template with the context.
func RenderTemplate(tmpl string, context map[string]string) (string, error) {
	t, tmplPrsErr := template.New("manifests").Option("missingkey=error").Parse(tmpl)
	if tmplPrsErr != nil {
		return "", tmplPrsErr
	}
	writer := bytes.NewBuffer([]byte{})
	if err := t.Execute(writer, context); err != nil {
		return "", err
	}

	return writer.String(), nil
}

// GetControlPlaneSize returns the size of control-plane
func GetControlPlaneSize(ctx context.Context, cli client.Client) (int, error) {
	nodes := &corev1.NodeList{}
	if err := cli.List(ctx, nodes, []client.ListOption{
		client.HasLabels{ControlPlaneLabel},
	}...); err != nil {
		return 0, err
	}
	return len(nodes.Items), nil
}

// IsControlPlaneNode returns true when the node is control-plane node
func IsControlPlaneNode(node *corev1.Node) bool {
	if _, ok := node.Labels[ControlPlaneLabel]; ok {
		return true
	}
	return false
}

// GetNodeCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetNodeCondition(status *corev1.NodeStatus, conditionType corev1.NodeConditionType) (int, *corev1.NodeCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// IsNodeReady returns true if the node is ready
func IsNodeReady(node *corev1.Node) bool {
	_, condition := GetNodeCondition(&node.Status, corev1.NodeReady)
	if condition == nil || condition.Status != corev1.ConditionTrue {
		return false
	}
	return true
}

// GetYurtComponentImageByName returns the full image name of OpenYurt component
func GetYurtComponentImageByName(yurtCluster *operatorv1alpha1.YurtCluster, name string) string {
	return fmt.Sprintf("%s/%s:%s", yurtCluster.Spec.ImageRepository, name, yurtCluster.Spec.YurtVersion)
}

// EnsureEdgeTaintForNode adds edge taint for node
func EnsureEdgeTaintForNode(node *corev1.Node) {
	found := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == EdgeNodeTaintKey && taint.Effect == EdgeNodeTaintEffect {
			found = true
			break
		}
	}

	if !found {
		taint := corev1.Taint{
			Key:    EdgeNodeTaintKey,
			Effect: EdgeNodeTaintEffect,
		}
		node.Spec.Taints = append(node.Spec.Taints, taint)
	}
}

// RemoveEdgeTaintForNode removes edge taint for node
func RemoveEdgeTaintForNode(node *corev1.Node) {
	var newTaints []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Key == EdgeNodeTaintKey && taint.Effect == EdgeNodeTaintEffect {
			continue
		}
		newTaints = append(newTaints, taint)
	}
	node.Spec.Taints = newTaints
}

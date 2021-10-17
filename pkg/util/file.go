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
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
)

var notFoundErr = errors.Errorf("FileNotFound")

// GetSingleContentPreferLastMatchFromFile determines whether there is a unique string that matches the
// regular expression regularExpression and returns it.
// If multiple values exist, return the last one.
func GetSingleContentPreferLastMatchFromFile(filename string, regularExpression string) (string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s, %v", filename, err)
	}
	return GetSingleContentPreferLastMatchFromString(string(content), regularExpression)
}

// GetSingleContentPreferLastMatchFromString determines whether there is a unique string that matches the
// regular expression regularExpression and returns it.
// If multiple values exist, return the last one.
func GetSingleContentPreferLastMatchFromString(content string, regularExpression string) (string, error) {
	contents := GetContentFormString(content, regularExpression)
	if contents == nil {
		return "", fmt.Errorf("no matching string %s in content %s", regularExpression, content)
	}
	if len(contents) > 1 {
		return contents[len(contents)-1], nil
	}
	return contents[0], nil
}

// GetContentFormString returns all strings that match the regular expression regularExpression
func GetContentFormString(content string, regularExpression string) []string {
	reg := regexp.MustCompile(regularExpression)
	res := reg.FindAllString(content, -1)
	return res
}

// FileExists determines whether the file exists
func FileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsExist(err) {
		return true, err
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// CopyFile copies sourceFile to destinationFile
func CopyFile(sourceFile string, destinationFile string) error {
	content, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file %s: %v", sourceFile, err)
	}
	err = ioutil.WriteFile(destinationFile, content, 0666)
	if err != nil {
		return fmt.Errorf("failed to write destination file %s: %v", destinationFile, err)
	}
	return nil
}

// FileSameContent determines whether the source file and destination file is same
func FileSameContent(sourceFile string, destinationFile string) (bool, error) {
	contentSrc, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		return false, err
	}
	contentDst, err := ioutil.ReadFile(destinationFile)
	if err != nil {
		return false, err
	}
	return string(contentSrc) == string(contentDst), nil
}

// EnableNodeLifeCycleController enables nodelifecycle controller for kube-controller-manager
func EnableNodeLifeCycleController(filePath string) error {
	pod, err := getPodFromKubeControllerManagerManifest(filePath)
	if err != nil {
		if err == notFoundErr {
			klog.Info("skip enable node-life-cycle controller, kube-controller-manager.yaml not exists")
			return nil
		}
		return errors.Wrap(err, "failed to enable nodelifecycle controller for kube-controller-manager")
	}

	old := pod.DeepCopy()

	for i, container := range pod.Spec.Containers {
		for j, command := range container.Command {
			if strings.Contains(command, ",-nodelifecycle") {
				command = strings.ReplaceAll(command, ",-nodelifecycle", "")
				pod.Spec.Containers[i].Command[j] = command
			}
		}
	}

	if reflect.DeepEqual(pod, old) {
		return nil
	}

	return writePodManifestIntoFile(pod, filePath)
}

// DisableNodeLifeCycleController disables nodelifecycle controller for kube-controller-manager
func DisableNodeLifeCycleController(filePath string) error {
	pod, err := getPodFromKubeControllerManagerManifest(filePath)
	if err != nil {
		if err == notFoundErr {
			klog.Info("skip disable node-life-cycle controller, kube-controller-manager.yaml not exists")
			return nil
		}
		return errors.Wrap(err, "failed to disable nodelifecycle controller for kube-controller-manager")
	}

	old := pod.DeepCopy()

	for i, container := range pod.Spec.Containers {
		if len(container.Command) == 0 || !strings.HasSuffix(container.Command[0], "kube-controller-manager") {
			continue
		}
		foundControllersFlag := false
		for j, command := range container.Command {
			if strings.HasPrefix(command, "--controllers=") {
				foundControllersFlag = true
				if strings.Contains(command, "-nodelifecycle") {
					break
				} else {
					command = fmt.Sprintf("%s,-nodelifecycle", command)
					pod.Spec.Containers[i].Command[j] = command
				}
				break
			}
		}

		if !foundControllersFlag {
			pod.Spec.Containers[i].Command = append(pod.Spec.Containers[i].Command, "--controllers=*,bootstrapsigner,tokencleaner,-nodelifecycle")
		}
	}

	if reflect.DeepEqual(pod, old) {
		return nil
	}

	return writePodManifestIntoFile(pod, filePath)
}

func getPodFromKubeControllerManagerManifest(filePath string) (*corev1.Pod, error) {
	exists, err := FileExists(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check file %v exists", filePath)
	}
	if !exists {
		return nil, notFoundErr
	}
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %v", filePath)
	}

	obj, err := YamlToObject(content)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert content of %v into client.Object", filePath)
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, errors.Errorf("the content of %v is not Pod", filePath)
	}
	return pod, nil
}

func writePodManifestIntoFile(pod *corev1.Pod, filePath string) error {
	serialized, err := MarshalToYaml(pod, corev1.SchemeGroupVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal manifest for pod %v", klog.KObj(pod))
	}
	if err := ioutil.WriteFile(filePath, serialized, 0600); err != nil {
		return errors.Wrapf(err, "failed to write static pod manifest file for %v", klog.KObj(pod))
	}
	return nil
}

// MarshalToYaml marshals an object into yaml.
func MarshalToYaml(obj runtime.Object, gv schema.GroupVersion) ([]byte, error) {
	return MarshalToYamlForCodecs(obj, gv, clientsetscheme.Codecs)
}

// MarshalToYamlForCodecs marshals an object into yaml using the specified codec
func MarshalToYamlForCodecs(obj runtime.Object, gv schema.GroupVersion, codecs serializer.CodecFactory) ([]byte, error) {
	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return []byte{}, errors.Errorf("unsupported media type %q", mediaType)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, gv)
	return runtime.Encode(encoder, obj)
}

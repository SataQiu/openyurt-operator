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
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

const (
	apiServerAddrRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"
)

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

// ParseAPIServerAddressFromKubeConfigFile tries to parse API Server address from kubeconfig file
func ParseAPIServerAddressFromKubeConfigFile(kubeletConfPath string) (string, error) {
	exists, err := FileExists(kubeletConfPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.Errorf("file %q not exists", kubeletConfPath)
	}
	apiServerAddr, err := GetSingleContentPreferLastMatchFromFile(kubeletConfPath, apiServerAddrRegularExpression)
	if err != nil {
		return "", err
	}
	args := strings.Split(apiServerAddr, " ")
	if len(args) != 2 {
		return "", errors.Errorf("cannot parse apiserver address from arg %v", apiServerAddr)
	}
	return args[1], nil
}

// GetAPIServerAddress returns the API Server address
func GetAPIServerAddress(apiServerAddress string) string {
	if len(apiServerAddress) != 0 {
		return apiServerAddress
	}
	// try to fetch apiServerAddress from kubelet kubeconfig file
	addr, err := ParseAPIServerAddressFromKubeConfigFile("/etc/kubernetes/kubelet.conf")
	if err != nil {
		return apiServerAddress
	}
	return addr
}

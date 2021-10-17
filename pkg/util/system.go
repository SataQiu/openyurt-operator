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
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// RunCommand runs a command by sh shell.
func RunCommand(cmd string) ([]byte, error) {
	return exec.Command("sh", "-c", cmd).Output()
}

// RunCommandWithCombinedOutput runs a command by sh shell with combined output.
func RunCommandWithCombinedOutput(cmd string) ([]byte, error) {
	return exec.Command("sh", "-c", cmd).CombinedOutput()
}

// GetServiceCmdLine returns the cmd line of given service
func GetServiceCmdLine(name string) (string, error) {
	return getSystemdServiceCmdLine(name)
}

func getSystemdServiceCmdLine(name string) (string, error) {
	pid, err := getSystemdServicePid(fmt.Sprintf(`%v.service`, name))
	if err != nil {
		return "", errors.Wrapf(err, "failed to get pid of %q service", name)
	}
	cmdLine, err := GetCmdLineByPid(pid)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get cmd line of %q", name)
	}
	return cmdLine, nil
}

func getSystemdServicePid(service string) (int, error) {
	cmd := fmt.Sprintf("systemctl show --property MainPID  %s", service)
	out, err := RunCommand(cmd)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to execute cmd %q", cmd)
	}

	outStr := string(out)
	arr := strings.Split(strings.TrimSpace(outStr), "=")
	if len(arr) != 2 {
		return -1, errors.Wrapf(err, "cannot parse PID from the output %q", outStr)
	}

	pid, err := strconv.Atoi(arr[1])
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert PID string %q into integer ", arr[1])
	}

	return pid, nil
}

// RestartService tries to restart a service.
// Currently, only systemd service is supported.
func RestartService(service string) error {
	return restartSystemdService(service)
}

func restartSystemdService(service string) error {
	cmd := "systemctl daemon-reload"
	_, err := RunCommand(cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to execute cmd %q", cmd)
	}
	cmd = fmt.Sprintf("systemctl restart %s", service)
	_, err = RunCommand(cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to execute cmd %q", cmd)
	}
	return nil
}

// GetCmdLineByPid returns the service CMDLine by PID
func GetCmdLineByPid(pid int) (string, error) {
	return getProcCmdLineByPid(pid)
}

func getProcCmdLineByPid(pid int) (string, error) {
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// CleanIPTablesRules cleans the iptables rules matched given string
func CleanIPTablesRules(match string) (string, error) {
	cmd := fmt.Sprintf(`iptables-save | grep -v "%v" | iptables-restore`, match)
	out, err := RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

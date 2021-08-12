package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	openyurtv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/pkg/kclient"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

// getKubeletCmdLine returns the cmd line of kubelet service
func getKubeletCmdLine() (string, error) {
	pid, err := util.GetServicePid("kubelet.service")
	if err != nil {
		return "", errors.Wrap(err, "failed to get pid of kubelet service")
	}
	cmdLine, err := util.GetCmdLineByPid(pid)
	if err != nil {
		return "", errors.Wrap(err, "failed to get cmd line of kubelet")
	}
	return cmdLine, nil
}

// setNodeConvertCondition sets NodeConvertCondition for YurtCluster
func setNodeConvertCondition(yurtCluster *openyurtv1alpha1.YurtCluster, nodeName string, condition openyurtv1alpha1.NodeConvertCondition) {
	if yurtCluster.Status.NodeConvertConditions == nil {
		yurtCluster.Status.NodeConvertConditions = make(map[string]openyurtv1alpha1.NodeConvertCondition)
	}
	yurtCluster.Status.NodeConvertConditions[nodeName] = condition
}

// parseKubeletHealthzPort tries to parse kubelet healthz port from cmd line
func parseKubeletHealthzPort(cmdLine string) (int, error) {
	kubeletHealthzPortFlag, err := util.GetSingleContentPreferLastMatchFromString(cmdLine, kubeletHealthzPortRegularExpression)
	if err != nil {
		return 0, errors.Errorf("failed to find --healthz-port arg from cmdLine %v", cmdLine)
	}
	args := strings.Split(kubeletHealthzPortFlag, "=")
	if len(args) != 2 {
		return 0, errors.Errorf("failed to split --healthz-port arg from cmdLine %v", cmdLine)
	}
	port, err := strconv.Atoi(args[1])
	if err != nil {
		return 0, errors.Errorf("failed to convert string %q into int", args[1])
	}
	return port, nil
}

// parsePkiDir returns the pki dir of the kubelet
func parsePkiDir(cmdLine string) (string, error) {
	clientCAPath, err := util.GetSingleContentPreferLastMatchFromString(cmdLine, clientCARegularExpression)
	if err != nil {
		return "", errors.Errorf("failed to find --client-ca-file arg from cmdLine %v", cmdLine)
	}
	args := strings.Split(clientCAPath, "=")
	if len(args) != 2 {
		return "", errors.Errorf("failed to split --client-ca-file arg from cmdLine %v", cmdLine)
	}
	return filepath.Dir(args[1]), nil
}

// assume that all materials have been prepared by yurt agent init container
func restartLocalPods(ctx context.Context, isConvert bool) error {
	svc := &corev1.Service{}
	err := kclient.CtlClient().Get(ctx, types.NamespacedName{Namespace: "default", Name: "kubernetes"}, svc)
	if err != nil {
		return err
	}
	if svc.Spec.ClusterIP == "" {
		klog.Warningf("found empty ClusterIP in kubernetes default service, skip restart containers")
		return nil
	}

	cmd := fmt.Sprintf("/var/tmp/restart-container.sh %v %s", isConvert, svc.Spec.ClusterIP)
	result, err := util.RunCommandWithCombinedOutput(cmd)
	if err != nil {
		return err
	}
	klog.Infof("restart containers with command %q result: %v", cmd, string(result))
	return nil
}

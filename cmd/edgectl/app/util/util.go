package util

import (
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
	kubeletconfig "k8s.io/kubelet/config/v1beta1"

	operatorv1alpha1 "github.com/openyurtio/openyurt-operator/api/v1alpha1"
	"github.com/openyurtio/openyurt-operator/cmd/edgectl/app/options"
	"github.com/openyurtio/openyurt-operator/pkg/util"
)

var (
	Scheme = runtime.NewScheme()
)

func init() {
	_ = kubeletconfig.AddToScheme(Scheme)
}

const (
	kubeletConfigRegularExpression           = "\\-\\-config=.*\\.yaml"
	kubeletKubeConfigRegularExpression       = "\\-\\-kubeconfig=.*kubelet\\.conf"
	kubeletPodManifestsPathRegularExpression = "\\-\\-pod\\-manifest\\-path=[A-Za-z0-9\\/]+'"
	kubeletAPIServerAddressRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"
	kubeletHealthPortRegularExpression       = "\\-\\-healthz\\-port=[0-9]+"
)

// KubeletConfiguration defines the configurations for kubelet
type KubeletConfiguration struct {
	CMD              string
	ConfigPath       string
	KubeConfigPath   string
	PodManifestsPath string
	APIServerAddress string
	HealthPort       int
}

// LoadKubeletConfiguration returns the configuration of kubelet
func LoadKubeletConfiguration(opt *options.BaseOptions) (*KubeletConfiguration, error) {
	cmdLine, err := util.GetServiceCmdLine("kubelet")
	if err != nil {
		return nil, err
	}

	configPath, err := parseStringValueFromCommand(cmdLine, kubeletConfigRegularExpression)
	if err != nil {
		configPath = opt.KubeletConfigPath
		klog.V(2).Infof("can not parse kubelet config path from command line, %v, using %q", err, configPath)
	}

	kubeletConfiguration, err := loadKubeletConfigurationFromFile(configPath)
	if err != nil {
		klog.V(2).Infof("can not load *kubeletconfig.KubeletConfiguration from %q, %v", configPath, err)
	}

	kubeConfigPath, err := parseKubeConfigPath(cmdLine, kubeletConfiguration)
	if err != nil {
		kubeConfigPath = opt.KubeletKubeConfigPath
		klog.V(2).Infof("can not parse kubelet kubeconfig path from command line, %v, using %q", err, kubeConfigPath)
	}

	podManifestsPath, err := parsePodManifestsPath(cmdLine, kubeletConfiguration)
	if err != nil {
		podManifestsPath = opt.PodManifestsPath
		klog.V(2).Infof("can not parse pod manifests path from command line, %v, using %q", err, podManifestsPath)
	}

	apiServerAddress, err := parseAPIServerAddressFromKubeConfig(kubeConfigPath)
	if err != nil {
		apiServerAddress = opt.APIServerAddress
		klog.V(2).Infof("can not parse kube-api-server address from kubeconfig %q, %v using %q", kubeConfigPath, err, apiServerAddress)
	}

	healthPort, err := parseKubeletHealthPort(cmdLine, kubeletConfiguration)
	if err != nil {
		healthPort = opt.KubeletHealthPort
		klog.V(2).Infof("can not parse kubelet health check port from command line, %v, using %q", err, healthPort)
	}

	return &KubeletConfiguration{
		CMD:              cmdLine,
		ConfigPath:       configPath,
		KubeConfigPath:   kubeConfigPath,
		PodManifestsPath: podManifestsPath,
		APIServerAddress: apiServerAddress,
		HealthPort:       healthPort,
	}, nil
}

func parseStringValueFromCommand(cmdLine string, regexp string) (string, error) {
	flagPair, err := util.GetSingleContentPreferLastMatchFromString(cmdLine, regexp)
	if err != nil {
		return "", errors.Wrapf(err, "failed to match %q from %q", regexp, cmdLine)
	}
	args := strings.Split(flagPair, "=")
	if len(args) != 2 {
		return "", errors.Errorf("failed to split flag pair %q by '='", flagPair)
	}
	return args[1], nil
}

func parseIntValueFromCommand(cmdLine string, regexp string) (int, error) {
	value, err := parseStringValueFromCommand(cmdLine, regexp)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

func loadKubeletConfigurationFromFile(path string) (*kubeletconfig.KubeletConfiguration, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %v", path)
	}
	decode := serializer.NewCodecFactory(Scheme).UniversalDeserializer().Decode
	obj, _, err := decode(content, nil, nil)
	if err != nil {
		return nil, err
	}
	o, ok := obj.(*kubeletconfig.KubeletConfiguration)
	if !ok {
		return nil, errors.Errorf("cannot convert runtime.Object %v into *kubeletconfig.KubeletConfiguration", obj)
	}
	return o, nil
}

// parseKubeConfigPath tries to parse kubelet kube-config path from the cmd line
func parseKubeConfigPath(cmdLine string, kubeletConfiguration *kubeletconfig.KubeletConfiguration) (string, error) {
	path, err := parseStringValueFromCommand(cmdLine, kubeletKubeConfigRegularExpression)
	if err == nil {
		return path, nil
	}

	return "", errors.Wrap(err, "can not parse kubelet kubeconfig path")
}

// parsePodManifestsPath tries to parse Pod manifests path from the cmd line
func parsePodManifestsPath(cmdLine string, kubeletConfiguration *kubeletconfig.KubeletConfiguration) (string, error) {
	path, err := parseStringValueFromCommand(cmdLine, kubeletPodManifestsPathRegularExpression)
	if err == nil {
		return path, nil
	}

	if kubeletConfiguration != nil {
		if len(kubeletConfiguration.StaticPodPath) > 0 {
			return kubeletConfiguration.StaticPodPath, nil
		}
	}

	return "", errors.Wrap(err, "can not parse pod manifests path")
}

// parseAPIServerAddressFromKubeConfig returns the API server address defined in kubelet kube-config
func parseAPIServerAddressFromKubeConfig(kubeConfigPath string) (string, error) {
	apiServerAddr, err := util.GetSingleContentPreferLastMatchFromFile(kubeConfigPath, kubeletAPIServerAddressRegularExpression)
	if err != nil {
		return "", err
	}
	args := strings.Split(apiServerAddr, " ")
	if len(args) != 2 {
		return "", errors.Errorf("cannot parse apiserver address from arg %v", apiServerAddr)
	}
	return args[1], nil
}

// parseKubeletHealthPort tries to parse kubelet health check port from the cmd line
func parseKubeletHealthPort(cmdLine string, kubeletConfiguration *kubeletconfig.KubeletConfiguration) (int, error) {
	port, err := parseIntValueFromCommand(cmdLine, kubeletHealthPortRegularExpression)
	if err == nil {
		return port, nil
	}

	if kubeletConfiguration != nil {
		if kubeletConfiguration.HealthzPort != nil && *kubeletConfiguration.HealthzPort != 0 {
			return int(*kubeletConfiguration.HealthzPort), nil
		}
	}

	return 0, errors.Wrap(err, "can not parse kubelet health port")
}

// UpdateNodeCondition updates NodeCondition for YurtCluster
func UpdateNodeCondition(yurtCluster *operatorv1alpha1.YurtCluster, nodeName string, condition operatorv1alpha1.NodeCondition) {
	if yurtCluster.Status.NodeConditions == nil {
		yurtCluster.Status.NodeConditions = make(map[string]operatorv1alpha1.NodeCondition)
	}
	yurtCluster.Status.NodeConditions[nodeName] = condition
}

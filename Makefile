# Image URL to use all building/pushing image targets
TAG ?= v1.0.0
REPO ?= registry.cn-hangzhou.aliyuncs.com/ecp_builder
BUILD_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7
BUILD_GO_PROXY_ARG ?= GO_PROXY=https://goproxy.cn,direct
OPERATOR_IMG ?= ${REPO}/openyurt-operator:${TAG}
AGENT_IMG ?= ${REPO}/openyurt-agent:${TAG}
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager agent edgectl

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager cmd/operator/operator.go

# Build agent binary
agent: generate fmt vet
	go build -o bin/agent cmd/agent/agent.go

# Build edgectl binary
edgectl: generate fmt vet
	go build -o bin/edgectl cmd/edgectl/edgectl.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./cmd/operator/operator.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image openyurt-operator=${OPERATOR_IMG}
	kustomize build config/default | kubectl apply -f -

# Release manifests into docs/manifests and push docker image to dockerhub
release-artifacts: manifests docker-push
	cd config/manager && kustomize edit set image openyurt-operator=${OPERATOR_IMG}
	cp config/crd/bases/* charts/crds/
	kustomize build config/default > docs/manifests/${TAG}/deploy.yaml

# Release manifests into docs/manifests and save docker images locally
release-local: manifests docker-build
	cd config/manager && kustomize edit set image openyurt-operator=${OPERATOR_IMG}
	cp config/crd/bases/* charts/crds/
	kustomize build config/default > docs/manifests/${TAG}/deploy.yaml

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image in linux/amd64 arch
docker-build: docker-build-operator docker-build-agent

docker-build-operator:
	docker buildx build --load --platform linux/amd64 -f Dockerfile . -t ${OPERATOR_IMG} --build-arg ${BUILD_GO_PROXY_ARG}
docker-build-agent:
	docker buildx build --load --platform linux/amd64 -f Dockerfile.agent . -t ${AGENT_IMG} --build-arg ${BUILD_GO_PROXY_ARG}

# Push the docker images with multi-arch
docker-push: docker-push-operator docker-push-agent

docker-push-operator:
	docker buildx build --push --platform ${BUILD_PLATFORMS} -f Dockerfile . -t ${OPERATOR_IMG} --build-arg ${BUILD_GO_PROXY_ARG}
docker-push-agent:
	docker buildx build --push --platform ${BUILD_PLATFORMS} -f Dockerfile.agent . -t ${AGENT_IMG} --build-arg ${BUILD_GO_PROXY_ARG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

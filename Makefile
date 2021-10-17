# Image URL to use all building/pushing image targets
TAG ?= v1.0.2
REPO ?= registry.cn-hangzhou.aliyuncs.com/ecp_builder
MANAGER_IMG ?= ${REPO}/openyurt-operator-manager:${TAG}
AGENT_IMG ?= ${REPO}/openyurt-operator-agent:${TAG}

# Build linux/amd64 arch with `make release-artifacts BUILD_PLATFORMS=linux/amd64`
BUILD_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7
BUILD_GO_PROXY_ARG ?= GO_PROXY=https://goproxy.cn,direct

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
	go build -o bin/manager cmd/manager/manager.go

# Build agent binary
agent: generate fmt vet
	go build -o bin/agent cmd/agent/agent.go

# Build edgectl binary
edgectl: generate fmt vet
	go build -o bin/edgectl cmd/edgectl/edgectl.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./cmd/manager/manager.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image openyurt-operator-manager=${MANAGER_IMG}
	cd config/agent && kustomize edit set image openyurt-operator-agent=${AGENT_IMG}
	kustomize build config/default | kubectl apply -f -

# Release manifests into docs/manifests and push docker image to dockerhub
release-artifacts: docker-push release-manifests

# Release manifests into docs/manifests
release-manifests: manifests
	cd config/manager && kustomize edit set image openyurt-operator-manager=${MANAGER_IMG}
	cd config/agent && kustomize edit set image openyurt-operator-agent=${AGENT_IMG}
	kustomize build config/default > docs/manifests/${TAG}/deploy.yaml

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." \
		output:crd:artifacts:config=config/crd/bases
	cp config/crd/bases/* charts/crds/

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Lint codebase
lint: golangci-lint
	$(GOLANGCI_LINT) run

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: docker-build-manager docker-build-agent

docker-build-manager:
	docker buildx build --load --platform ${BUILD_PLATFORMS} -f Dockerfile . -t ${MANAGER_IMG} \
		--build-arg ${BUILD_GO_PROXY_ARG}
docker-build-agent:
	docker buildx build --load --platform ${BUILD_PLATFORMS} -f Dockerfile.agent . -t ${AGENT_IMG} \
		--build-arg ${BUILD_GO_PROXY_ARG}

# Push the docker images with multi-arch
docker-push: docker-push-manager docker-push-agent

docker-push-manager:
	docker buildx build --push --platform ${BUILD_PLATFORMS} -f Dockerfile . -t ${MANAGER_IMG} \
		--build-arg ${BUILD_GO_PROXY_ARG}
docker-push-agent:
	docker buildx build --push --platform ${BUILD_PLATFORMS} -f Dockerfile.agent . -t ${AGENT_IMG} \
		--build-arg ${BUILD_GO_PROXY_ARG}

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

# find or download golangci-lint
golangci-lint:
ifeq (, $(shell which golangci-lint))
	@{ \
	set -e ;\
	GOLANGCI_LINT_TMP_DIR=$$(mktemp -d) ;\
	cd $$GOLANGCI_LINT_TMP_DIR ;\
	go mod init tmp ;\
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.42.1 ;\
	rm -rf $$GOLANGCI_LINT_TMP_DIR ;\
	}
GOLANGCI_LINT=$(GOBIN)/golangci-lint
else
GOLANGCI_LINT=$(shell which golangci-lint)
endif

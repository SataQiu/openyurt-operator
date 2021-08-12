# Image URL to use all building/pushing image targets
TAG ?= v1.0.0
REPO ?= registry.cn-hangzhou.aliyuncs.com/ecp_builder
OPERATOR_IMG ?= ${REPO}/openyurt-operator:${TAG}
AGENT_IMG ?= ${REPO}/openyurt-agent:${TAG}
NODE_IMG ?= ${REPO}/openyurt-node:${TAG}
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

# Build the docker image
docker-build: docker-build-operator docker-build-agent docker-build-node

docker-build-operator:
	docker build -f Dockerfile . -t ${OPERATOR_IMG} --build-arg GO_PROXY=https://goproxy.cn,direct

docker-build-agent:
	docker build -f Dockerfile.agent . -t ${AGENT_IMG} --build-arg GO_PROXY=https://goproxy.cn,direct

docker-build-node:
	docker build -f Dockerfile.node . -t ${NODE_IMG} --build-arg GO_PROXY=https://goproxy.cn,direct

# Push the docker image
docker-push: docker-build
	docker push ${OPERATOR_IMG}
	docker push ${AGENT_IMG}
	docker push ${NODE_IMG}

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

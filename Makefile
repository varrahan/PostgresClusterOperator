# Project parameters
PROJECT_NAME := postgres-operator
MODULE := $(shell go list -m)
CONTROLLER_GEN := $(shell pwd)/bin/controller-gen
KUSTOMIZE := $(shell pwd)/bin/kustomize

# Image parameters
IMAGE_REGISTRY ?= localhost:5000
IMAGE_TAG ?= latest
IMG ?= $(IMAGE_REGISTRY)/$(PROJECT_NAME):$(IMAGE_TAG)

# Kubernetes parameters
NAMESPACE ?= postgres-operator-system

# Setup Go related variables
GO ?= go
GOBIN ?= $(shell go env GOPATH)/bin

# Directories
API_V1_DIR := ./api/v1
CONFIG_DIR := ./config
CRD_DIR := $(CONFIG_DIR)/crd
CRD_BASE_DIR := $(CRD_DIR)/bases

.PHONY: all
all: build

## Build the operator binary
.PHONY: build
build:
	$(GO) build -o bin/manager main.go

## Run tests
.PHONY: test
test:
	$(GO) test ./... -coverprofile cover.out

## Generate deepcopy and other code in api/v1
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) \
		object \
		paths="$(API_V1_DIR)/..." \
		output:object:dir=$(API_V1_DIR)
	@echo "Code generation complete in $(API_V1_DIR)"

## Generate CRD manifests and RBAC
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) \
		crd \
		rbac:roleName=manager-role \
		webhook \
		paths="$(API_V1_DIR)/..." \
		output:crd:artifacts:config=$(CRD_BASE_DIR)
	@echo "Manifests generated at $(CRD_BASE_DIR)"

## Install CRDs into the cluster
.PHONY: install
install: manifests kustomize
	$(KUSTOMIZE) build $(CRD_DIR) | kubectl apply -f -

## Uninstall CRDs from the cluster
.PHONY: uninstall
uninstall: manifests kustomize
	$(KUSTOMIZE) build $(CRD_BASE_DIR) | kubectl delete -f -

## Deploy controller to the cluster
.PHONY: deploy
deploy: install manifests kustomize
	cd $(CONFIG_DIR)/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build $(CONFIG_DIR)/default > deploy.yaml
	@trap "rm -f deploy.yaml" EXIT; \
	if ! kubectl apply -f deploy.yaml; then \
		echo "Error: kubectl apply failed"; exit 1; \
	fi; \
	echo "Waiting for deployment (timeout 60s)..."; \
	if ! kubectl rollout status deployment $(PROJECT_NAME)-controller-manager -n $(NAMESPACE) --timeout=60s; then \
		echo "Error: Rollout timed out. Check logs with: make logs"; \
		exit 1; \
	fi; \
	echo "Deployment successful"

## Undeploy controller from the cluster
.PHONY: undeploy
undeploy: kustomize
	$(KUSTOMIZE) build $(CONFIG_DIR)/default | kubectl delete -f -

## Run the operator locally
.PHONY: run
run: manifests generate
	$(GO) run ./main.go

## Build Docker image
.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

## Push Docker image
.PHONY: docker-push
docker-push:
	docker push $(IMG)

## Build and push Docker image
.PHONY: docker-build-push
docker-build-push: docker-build docker-push

## Create sample PostgreSQL resources
.PHONY: samples
samples:
	kubectl apply -f $(CONFIG_DIR)/samples/

## Remove sample PostgreSQL resources
.PHONY: samples-clean
samples-clean:
	kubectl delete -f $(CONFIG_DIR)/samples/ --ignore-not-found=true

## Setup development environment
.PHONY: dev-setup
dev-setup: controller-gen kustomize
	$(MAKE) generate manifests

## Full deployment (build, push, deploy)
.PHONY: deploy-full
deploy-full: docker-build-push deploy

## Install controller-gen locally if not present
.PHONY: controller-gen
controller-gen:
	@{ \
	if [ ! -f $(CONTROLLER_GEN) ]; then \
		set -e ; \
		CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ; \
		cd $$CONTROLLER_GEN_TMP_DIR ; \
		go mod init tmp ; \
		GOBIN=$(shell pwd)/bin $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@latest ; \
		rm -rf $$CONTROLLER_GEN_TMP_DIR ; \
	fi ; \
	}

## Install kustomize locally if not present
.PHONY: kustomize
kustomize:
	@{ \
	if [ ! -f $(KUSTOMIZE) ]; then \
		set -e ; \
		KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ; \
		cd $$KUSTOMIZE_GEN_TMP_DIR ; \
		curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- 3.8.7 $(shell pwd)/bin ; \
		rm -rf $$KUSTOMIZE_GEN_TMP_DIR ; \
	fi ; \
	}

## Check cluster connectivity and resources
.PHONY: status
status:
	@echo "Checking cluster connectivity..."
	kubectl cluster-info
	@echo "\nChecking CRDs..."
	kubectl get crd | grep database.example.com || echo "No CRDs found"
	@echo "\nChecking operator deployment..."
	kubectl get deployment -n $(NAMESPACE) | grep $(PROJECT_NAME) || echo "No operator deployment found"
	@echo "\nChecking PostgreSQL resources..."
	kubectl get postgresclusters --all-namespaces || echo "No PostgreSQL clusters found"

## View operator logs
.PHONY: logs
logs:
	kubectl logs -n $(NAMESPACE) deployment/$(PROJECT_NAME)-controller-manager -f

## Clean up generated files and binaries
.PHONY: clean
clean:
	rm -rf bin
	find $(API_DIR) -name zz_generated.deepcopy.go -delete
	rm -f cover.out

## Full cleanup (clean + undeploy + uninstall)
.PHONY: clean-all
clean-all: clean undeploy uninstall samples-clean

## Lint the code
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## Format the code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

## Vet the code
.PHONY: vet
vet:
	$(GO) vet ./...

## Run pre-commit checks
.PHONY: pre-commit
pre-commit: fmt vet lint test

## Generate all code and manifests
.PHONY: generate-all
generate-all: generate manifests

## Verify code generation is up-to-date
.PHONY: verify-generate
verify-generate: generate-all
	@if ! git diff --quiet; then \
		echo "Generated files are out of date. Please run 'make generate-all' and commit the changes."; \
		git diff; \
		exit 1; \
	fi

## Help: show commands
.PHONY: help
help:
	@echo "PostgreSQL Operator Makefile"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build              Build the operator binary"
	@echo "  make docker-build       Build Docker image"
	@echo "  make docker-push        Push Docker image"
	@echo "  make docker-build-push  Build and push Docker image"
	@echo ""
	@echo "Development Commands:"
	@echo "  make generate           Generate deepcopy and other code"
	@echo "  make generate-all       Generate all code and manifests"
	@echo "  make verify-generate    Verify generated code is up-to-date"
	@echo "  make manifests          Generate CRD manifests and RBAC"
	@echo "  make dev-setup          Setup development environment"
	@echo "  make run                Run operator locally"
	@echo "  make test               Run unit tests"
	@echo "  make pre-commit         Run pre-commit checks (fmt, vet, lint, test)"
	@echo ""
	@echo "Cluster Commands:"
	@echo "  make install            Install CRDs to cluster"
	@echo "  make uninstall          Remove CRDs from cluster"
	@echo "  make deploy             Deploy operator to cluster"
	@echo "  make undeploy           Remove operator from cluster"
	@echo "  make deploy-full        Build, push, and deploy (full workflow)"
	@echo ""
	@echo "Sample Commands:"
	@echo "  make samples            Create sample PostgreSQL resources"
	@echo "  make samples-clean      Remove sample PostgreSQL resources"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make status             Check cluster and operator status"
	@echo "  make logs               View operator logs"
	@echo "  make clean              Clean up generated files and binaries"
	@echo "  make clean-all          Full cleanup (clean + undeploy + uninstall)"
	@echo ""
	@echo "Configuration:"
	@echo "  IMG=$(IMG)"
	@echo "  NAMESPACE=$(NAMESPACE)"
	@echo ""
	@echo "Examples:"
	@echo "  make IMG=my-registry/postgres-operator:v1.0.0 deploy-full"
	@echo "  make NAMESPACE=my-namespace deploy"
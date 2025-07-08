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

# Directories
API_DIR := ./api/v1
CONFIG_DIR := ./config

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

## Generate deepcopy and other code
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object paths="$(API_DIR)/..."

## Generate CRD manifests and RBAC
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./api/v1/..." output:crd:artifacts:config=config/crd/bases

## Install CRDs into the cluster
.PHONY: install
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

## Uninstall CRDs from the cluster
.PHONY: uninstall
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

## Deploy controller to the cluster
.PHONY: deploy
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

## Undeploy controller from the cluster
.PHONY: undeploy
undeploy: kustomize
	$(KUSTOMIZE) build config/default | kubectl delete -f -

## Run the operator locally (requires cluster connection)
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
	kubectl apply -f config/samples/

## Remove sample PostgreSQL resources
.PHONY: samples-clean
samples-clean:
	kubectl delete -f config/samples/ --ignore-not-found=true

## Setup development environment
.PHONY: dev-setup
dev-setup: controller-gen kustomize manifests generate

## Full deployment (build, push, deploy)
.PHONY: deploy-full
deploy-full: docker-build-push deploy

## Install controller-gen locally if not present
.PHONY: controller-gen
controller-gen:
	@{ \
		if [ ! -x "$(CONTROLLER_GEN)" ]; then \
			echo "Downloading controller-gen..."; \
			GOBIN=$(shell pwd)/bin $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@latest; \
		fi \
	}

## Install kustomize locally if not present
.PHONY: kustomize
kustomize:
	@{ \
		if [ ! -x "$(KUSTOMIZE)" ]; then \
			echo "Downloading kustomize..."; \
			curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash; \
			mv kustomize $(shell pwd)/bin/; \
		fi \
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
	rm -rf bin zz_generated.deepcopy.go cover.out

## Full cleanup (clean + undeploy + uninstall)
.PHONY: clean-all
clean-all: clean undeploy uninstall samples-clean

## Lint the code
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, skipping lint"; \
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
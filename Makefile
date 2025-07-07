# Makefile for Kubernetes operator

# Project parameters
PROJECT_NAME := postgres-operator
MODULE := $(shell go list -m)
CONTROLLER_GEN := $(shell pwd)/bin/controller-gen

# Setup Go related variables
GO ?= go

# Directories
API_DIR := ./api/v1

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

## Generate code (deepcopy, etc.)
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object paths="$(API_DIR)/..."

## Install controller-gen locally if not present
.PHONY: controller-gen
controller-gen:
	@{ \
		if [ ! -x "$(CONTROLLER_GEN)" ]; then \
			echo "Downloading controller-gen..."; \
			GOBIN=$(shell pwd)/bin $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@latest; \
		fi \
	}

## Clean up generated files and binaries
.PHONY: clean
clean:
	rm -rf bin zz_generated.deepcopy.go cover.out

## Help: show commands
.PHONY: help
help:
	@echo "Usage:"
	@echo "  make build           Build the operator binary"
	@echo "  make generate        Generate deepcopy and other code"
	@echo "  make test            Run unit tests"
	@echo "  make clean

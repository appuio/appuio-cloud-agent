# Set Shell to bash, otherwise some targets fail with dash/zsh etc.
SHELL := /bin/bash

# Disable built-in rules
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables
.SUFFIXES:
.SECONDARY:
.DEFAULT_GOAL := help

# General variables
include Makefile.vars.mk

# Optional kind module
-include kind/kind.mk

.PHONY: help
help: ## Show this help
	@grep -E -h '\s##\s' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: build-bin build-docker ## All-in-one build

.PHONY: build-bin
build-bin: export CGO_ENABLED = 0
build-bin: fmt vet ## Build binary
	@go build -o $(BIN_FILENAME)

.PHONY: build-docker
build-docker: build-bin ## Build docker image
	$(DOCKER_CMD) build -t $(CONTAINER_IMG) .

.PHONY: run
run:
	go run . -webhook-cert-dir webhook-certs -zap-devel -zap-log-level debug

.PHONY: test
test: test-go ## All-in-one test

.PHONY: fuzz
fuzz:
	go test ./ratio -fuzztime 1m -fuzz .

.PHONY: test-go
test-go: ## Run unit tests against code
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -race -coverprofile cover.out -covermode atomic ./...

.PHONY: fmt
fmt: ## Run 'go fmt' against code
	go fmt ./...

.PHONY: vet
vet: ## Run 'go vet' against code
	go vet ./...

.PHONY: lint
lint: fmt vet generate manifests ## All-in-one linting
	@echo 'Check for uncommitted changes ...'
	git diff --exit-code

.PHONY: generate
generate: ## Generate additional code and artifacts
	@go generate ./...
	@# Kubebuilder misses the scope field for the webhook generator
	@yq eval -i '.webhooks[] |= with(select(.name == "validate-request-ratio.appuio.io"); .rules[] |= .scope = "Namespaced")' config/webhook/manifests.yaml
	@## Kubebuilder misses the namespaceSelector field for the webhook generator
	@# @yq eval -i '.webhooks[] |= with(select(.name == "validate-namespace-quota.appuio.io");                 .namespaceSelector = {"matchExpressions": [{"key": "appuio.io/organization", "operator": "Exists" }]})' config/webhook/manifests.yaml
	@# @yq eval -i '.webhooks[] |= with(select(.name == "validate-namespace-quota-projectrequests.appuio.io"); .namespaceSelector = {"matchExpressions": [{"key": "appuio.io/organization", "operator": "Exists" }]})' config/webhook/manifests.yaml

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	go run sigs.k8s.io/controller-tools/cmd/controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: clean
clean: ## Cleans local build artifacts
	rm -rf docs/node_modules $(docs_out_dir) dist .cache

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

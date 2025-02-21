# default target is build
.DEFAULT_GOAL := help

OUT := $(shell pwd)/_out
TEST_DNS_SERVER ?= ns.rackspace.com:53
TEST_ZONE_NAME ?= cert-manager.undercloud.rax.io.

.PHONY: help
help: ## Displays this help message
	@echo "$$(grep -hE '^\S+:.*##' $(MAKEFILE_LIST) | sed -e 's/:.*##\s*/|/' -e 's/^\(.\+\):\(.*\)/\\x1b[36m\1\\x1b[m:\2/' | column -c2 -t -s'|' | sort)"

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Ensure consistent code style
	@go mod tidy
	@go fmt ./...
	@golangci-lint run --fix

.PHONY: build
build: ## Local build
	@goreleaser build --snapshot --clean

.PHONY: install-tools
install-tools:
	mkdir -p $(OUT)
	./scripts/fetch-test-binaries.sh

.PHONY: test
test: install-tools
	./scripts/setup-tests.sh
	TEST_ASSET_ETCD=$(OUT)/controller-tools/envtest/etcd \
	TEST_ASSET_KUBECTL=$(OUT)/controller-tools/envtest/kubectl \
	TEST_ASSET_KUBE_APISERVER=$(OUT)/controller-tools/envtest/kube-apiserver \
	TEST_ZONE_NAME=$(TEST_ZONE_NAME) \
	TEST_DNS_SERVER=$(TEST_DNS_SERVER) go test -v ./cmd/webhook

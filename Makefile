# default target is build
.DEFAULT_GOAL := help

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

.PHONY: all build dev deps generate help print-version test vet

CHECK_FILES ?= $$(go list ./... | grep -v /vendor/)

help: ## Show this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {sub("\\\\n",sprintf("\n%22c"," "), $$2);printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: deps generate build test ## Run all steps

build: ## Build all
	go build ./...

deps: ## Download dependencies.
	go mod tidy

generate: ## Run code generation
	go generate ./...

test: ## Run tests
	go test -v $(CHECK_FILES)

vet: ## Run vet
	go vet ./...

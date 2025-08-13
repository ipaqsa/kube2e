.PHONY: test lint lint-fix fmt vet build build-all install \
        build-image push push-latest builder clean help

BINARY    := kube2e
BIN_DIR   := bin
REGISTRY  ?= ghcr.io/ipaqsa
IMAGE     ?= kube2e:latest
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "unknown")
PLATFORMS ?= linux/amd64,linux/arm64

LDFLAGS   := -X github.com/ipaqsa/kube2e/internal/version.Version=$(VERSION) -s -w
GO_BUILD  := CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)"

DOCKER_PLATFORM_ARGS := \
	--platform $(PLATFORMS) \
	--build-arg VERSION=$(VERSION)

DOCKER_TAGS := \
	--tag $(REGISTRY)/$(IMAGE) \
	--tag $(REGISTRY)/$(BINARY):$(VERSION)

## test:       Run tests
test:
	go test -v ./...

## lint:       Lint the codebase
lint:
	golangci-lint run

## lint-fix:   Fix autofixable lint issues
lint-fix:
	golangci-lint run --fix

## fmt:        Format code
fmt:
	go fmt ./...

## vet:        Run static analysis
vet:
	go vet ./...

## build:      Build the CLI binary to bin/
build: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/$(BINARY) ./cmd/kube2e

## build-all:  Build for all supported platforms to bin/
build-all: | $(BIN_DIR)
	GOOS=linux  GOARCH=amd64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-linux-amd64  ./cmd/kube2e
	GOOS=linux  GOARCH=arm64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-linux-arm64  ./cmd/kube2e
	GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-darwin-arm64 ./cmd/kube2e

## install:    Build and install binary to /usr/local/bin
install: build
	sudo install -m 0755 $(BIN_DIR)/$(BINARY) /usr/local/bin/$(BINARY)

## build-image: Build multi-platform container image (no push)
build-image: builder
	docker buildx build $(DOCKER_PLATFORM_ARGS) $(DOCKER_TAGS) .

## push:       Build and push multi-platform image to registry
push: builder
	docker buildx build $(DOCKER_PLATFORM_ARGS) $(DOCKER_TAGS) --push .

## push-latest: Build and push latest tag only (for development)
push-latest: builder
	docker buildx build $(DOCKER_PLATFORM_ARGS) --tag $(REGISTRY)/$(IMAGE) --push .

## builder:    Create or reuse multiplatform buildx builder
builder:
	@if ! docker buildx ls | grep -q multiplatform-builder; then \
		echo "Creating new multiplatform builder..."; \
		docker buildx create --name multiplatform-builder --driver docker-container --use; \
	else \
		echo "Using existing multiplatform builder..."; \
		docker buildx use multiplatform-builder; \
	fi

## clean:      Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## help:       Show available targets
help:
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

$(BIN_DIR):
	@mkdir -p $@

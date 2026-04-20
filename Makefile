.DEFAULT_GOAL := help

GOCACHE ?= $(CURDIR)/.cache/go-build

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
CGO_ENABLED ?= 1

GO_LDFLAGS := -s -w \
	-X swaves/internal/platform/buildinfo.Version=$(VERSION) \
	-X swaves/internal/platform/buildinfo.Commit=$(COMMIT) \
	-X swaves/internal/platform/buildinfo.BuildTime=$(BUILD_TIME)

RELEASE_BASENAME := swaves_$(VERSION)_$(GOOS)_$(GOARCH)
RELEASE_BIN_PATH := $(RELEASE_BASENAME)
RELEASE_ARCHIVE := $(RELEASE_BASENAME).tar.gz
RELEASE_SHA256 := $(RELEASE_ARCHIVE).sha256

.PHONY: help fe ceditor seditor test build release clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

ceditor: ## Build the CodeMirror-based source editor bundle
	cd web/ceditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=CEditor --outfile=../static/ceditor/dist/ceditor.js --target=es2018
	cd web/ceditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=CEditor --outfile=../static/ceditor/dist/ceditor.min.js --minify --target=es2018

seditor: ## Build the ProseMirror-based markdown editor bundle
	cd web/seditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=SEditor --outfile=../static/seditor/dist/seditor.js --target=es2018
	cd web/seditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=SEditor --outfile=../static/seditor/dist/seditor.min.js --minify --target=es2018

fe: ceditor seditor ## Build both frontend editor bundles

test: ## Run the full Go test suite
	@mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) go test ./...

build: fe ## Build frontend bundles and the local executable
	@mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" \
		-o swaves ./cmd/swaves

release: ## Build and package the release executable with version metadata
	@mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" \
		-o $(RELEASE_BIN_PATH) ./cmd/swaves
	@rm -f $(RELEASE_ARCHIVE) $(RELEASE_SHA256)
	tar -czf "$(RELEASE_ARCHIVE)" "$(RELEASE_BIN_PATH)"
	@if command -v shasum >/dev/null 2>&1; then \
		shasum -a 256 "$(RELEASE_ARCHIVE)" > "$(RELEASE_SHA256)"; \
	elif command -v sha256sum >/dev/null 2>&1; then \
		sha256sum "$(RELEASE_ARCHIVE)" > "$(RELEASE_SHA256)"; \
	else \
		echo "missing shasum/sha256sum" >&2; \
		exit 1; \
	fi

clean: ## Remove build artifacts produced by this Makefile
	rm -rf .cache/go-build swaves swaves_*.tar.gz swaves_*.tar.gz.sha256 swaves_*_*_*

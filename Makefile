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

TAG_BUMP := $(word 2,$(MAKECMDGOALS))
ifeq ($(strip $(TAG_BUMP)),)
TAG_BUMP := patch
endif

.PHONY: help fe ceditor seditor test binary build release tag major minor patch clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

ceditor: ## Build the CodeMirror-based source editor bundle
	cd web/ceditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=CEditor --outfile=../static/ceditor/dist/ceditor.js --target=es2018
	cd web/ceditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=CEditor --outfile=../static/ceditor/dist/ceditor.min.js --minify --target=es2018

seditor: ## Build the ProseMirror-based markdown editor bundle
	cd web/seditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=SEditor --outfile=../static/seditor/dist/seditor.js --target=es2018
	cd web/seditor && npm exec -- esbuild src/index.js --bundle --format=iife --global-name=SEditor --outfile=../static/seditor/dist/seditor.min.js --minify --target=es2018

fe: ceditor seditor ## Build both frontend editor bundles

test: ## Run the full Go test suitethe local executable
	@mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" \
		-o swaves ./cmd/swaves

build: fe binary ## Build frontend bundles and the local executable

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

tag: ## Pull latest changes, bump remote semver tag, and push branch with the new tag
	@set -eu; \
	mode="$(TAG_BUMP)"; \
	echo "==> tag bump mode: $$mode"; \
	case "$$mode" in \
		major|minor|patch) ;; \
		*) echo "invalid bump type: $$mode (use major, minor, or patch)" >&2; exit 1 ;; \
	esac; \
	echo "==> pulling latest branch"; \
	git pull --ff-only; \
	echo "==> reading latest remote tag from origin"; \
	latest_tag=$$(git ls-remote --tags --refs origin 'v*' | awk '{ \
		sub("refs/tags/", "", $$2); \
		tag = $$2; \
		if (tag ~ /^v[0-9]+\.[0-9]+\.[0-9]+$$/) { \
			version = substr(tag, 2); \
			split(version, parts, "."); \
			printf("%09d %09d %09d %s\n", parts[1], parts[2], parts[3], tag); \
		} \
	}' | sort | tail -n 1 | awk '{print $$4}'); \
	if [ -z "$$latest_tag" ]; then \
		latest_tag="v0.0.0"; \
	fi; \
	version=$${latest_tag#v}; \
	old_ifs=$$IFS; \
	IFS=.; set -- $$version; \
	IFS=$$old_ifs; \
	major=$${1:-0}; \
	minor=$${2:-0}; \
	patch=$${3:-0}; \
	case "$$mode" in \
		major) major=$$((major + 1)); minor=0; patch=0 ;; \
		minor) minor=$$((minor + 1)); patch=0 ;; \
		patch) patch=$$((patch + 1)) ;; \
	esac; \
	new_tag="v$${major}.$$minor.$$patch"; \
	echo "==> latest remote tag: $$latest_tag"; \
	echo "==> creating new tag: $$new_tag"; \
	git tag -a "$$new_tag" -m "$$new_tag"; \
	echo "==> pushing current branch and tag $$new_tag"; \
	git push origin HEAD --follow-tags

major minor patch:
	@:

clean: ## Remove build artifacts produced by this Makefile
	rm -rf .cache/go-build swaves swaves_*.tar.gz swaves_*.tar.gz.sha256 swaves_*_*_*

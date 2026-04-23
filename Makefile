SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

DOCKERFILES_DIR  := dockerfiles

# All Dockerfiles in the dockerfiles directory (auto-discovered).
ALL_IMAGES := $(patsubst $(DOCKERFILES_DIR)/Dockerfile.%,%,$(wildcard $(DOCKERFILES_DIR)/Dockerfile.*))

# Build a subset of images or all if none specified.
# Usage:
#   make build                          # builds all images
#   make build IMAGES="go1.25"          # builds only go1.25
#   make build IMAGES="go1.25 go1.26"   # builds go1.25 and go1.26
IMAGES           ?= $(ALL_IMAGES)
ORG              ?= rancher
REPO             ?= $(ORG)/ci-image
IMAGE_REPO 		 ?= ghcr.io/$(REPO)
TARGET_PLATFORMS ?= linux/amd64,linux/arm64
# VERSION is set by CI to YYYYMMDD-<run_number> for unique, Renovate-sortable tags.
# Falls back to a local dev value so `make push` works outside CI.
VERSION          ?= $(shell date -u +%Y%m%d-%H%M)-dev
NO_CACHE         ?=

_GIT_COMMIT  := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
_GIT_REMOTE  := $(shell git remote get-url origin 2>/dev/null | sed 's|git@github.com:|https://github.com/|;s|\.git$$||' || true)
_BUILD_DATE  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
_SOURCE_URL   = $(if $(ORG),https://github.com/$(REPO),$(_GIT_REMOTE))

.PHONY: all help test generate verify build push clean

all: test generate build ## Run tests, generate Dockerfiles, and build all images

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

test: ## Run unit tests
	go test -v -count=1 ./...

generate: ## Generate Dockerfiles from templates and deps.yaml
	go run main.go

verify: ## Verify no uncommitted changes exist
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: uncommitted changes detected:"; \
		git status --porcelain; \
		git diff; \
		exit 1; \
	fi

define buildx
	@if [ -z "$(IMAGES)" ]; then \
		echo "Error: no images found. Run 'make generate' first."; \
		exit 1; \
	fi
	@for img in $(IMAGES); do \
		echo "==> $(1) $${img}:$(VERSION)"; \
		docker buildx build \
			--file "$(DOCKERFILES_DIR)/Dockerfile.$${img}" \
			--platform "$(TARGET_PLATFORMS)" \
			--provenance mode=max \
			--sbom=true \
			$(if $(NO_CACHE),--no-cache) \
			--label "org.opencontainers.image.source=$(_SOURCE_URL)" \
			--label "org.opencontainers.image.url=$(_SOURCE_URL)" \
			--label "org.opencontainers.image.revision=$(_GIT_COMMIT)" \
			--label "org.opencontainers.image.created=$(_BUILD_DATE)" \
			--label "org.opencontainers.image.version=$(VERSION)" \
			--tag "$(IMAGE_REPO)/$${img}:$(VERSION)" \
			--tag "$(IMAGE_REPO)/$${img}:latest" \
			$(2) \
			. || exit 1; \
	done
endef

build: ## Build container images for all target platforms
	$(call buildx,Building,)

push: ## Build and push container images to the registry
	$(call buildx,Pushing,--push)

clean: ## Remove generated Dockerfiles
	rm -rf $(DOCKERFILES_DIR)

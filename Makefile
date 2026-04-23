SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

DOCKERFILES_DIR  := dockerfiles

# Image names sourced from the images field of images-lock.yaml.
ALL_IMAGES := $(shell awk '/^images:/{f=1;next} /^[a-zA-Z]/{f=0} f && /- /{print $$2}' images-lock.yaml)

# IMAGE must be set explicitly for build/push. Use build-all/push-all to target every image.
# Usage:
#   make build IMAGE=go1.25   # builds only go1.25
#   make build-all             # builds every image
IMAGE            ?=
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

.PHONY: all help test generate verify build push build-all push-all clean setup validate

# Stamp file so setup only runs once per clone, not on every make invocation.
.git/hooks/.setup-done: .githooks/pre-push
	git config core.hooksPath .githooks
	@touch $@

# Pull setup into every real target via this phony prerequisite.
.PHONY: _setup
_setup: .git/hooks/.setup-done

all: _setup test generate build-all ## Run tests, generate Dockerfiles, and build all images

help: _setup ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

test: _setup ## Run unit tests
	go test -v -count=1 ./...

generate: _setup ## Generate Dockerfiles from templates and deps.yaml
	go run main.go

verify: _setup ## Verify no uncommitted changes exist
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: uncommitted changes detected:"; \
		git status --porcelain; \
		git diff; \
		exit 1; \
	fi

validate: _setup generate verify

define buildx
	@if [ -z "$(value IMAGE)" ]; then \
		echo "Error: IMAGE is not set. Specify IMAGE=<name> or use build-all/push-all."; \
		exit 1; \
	fi
	@echo "==> $(1) $(value IMAGE):$(VERSION)"
	@docker buildx build \
		--file "$(DOCKERFILES_DIR)/Dockerfile.$(value IMAGE)" \
		--platform "$(TARGET_PLATFORMS)" \
		--provenance mode=max \
		--sbom=true \
		$(if $(NO_CACHE),--no-cache) \
		--label "org.opencontainers.image.source=$(_SOURCE_URL)" \
		--label "org.opencontainers.image.url=$(_SOURCE_URL)" \
		--label "org.opencontainers.image.revision=$(_GIT_COMMIT)" \
		--label "org.opencontainers.image.created=$(_BUILD_DATE)" \
		--label "org.opencontainers.image.version=$(VERSION)" \
		--tag "$(IMAGE_REPO)/$(value IMAGE):$(VERSION)" \
		--tag "$(IMAGE_REPO)/$(value IMAGE):latest" \
		$(2) \
		.
endef

build: _setup ## Build a single image — requires IMAGE=<name>
	$(call buildx,Building,)

push: _setup ## Build and push a single image — requires IMAGE=<name>
	$(call buildx,Pushing,--push)

build-all: _setup ## Build all container images
	@for img in $(ALL_IMAGES); do \
		$(MAKE) build IMAGE="$${img}" || exit 1; \
	done

push-all: _setup ## Build and push all container images
	@for img in $(ALL_IMAGES); do \
		$(MAKE) push IMAGE="$${img}" || exit 1; \
	done

clean: _setup ## Remove generated Dockerfiles
	rm -rf $(DOCKERFILES_DIR)

setup: .git/hooks/.setup-done ## Configure git to use the repo's hooks (.githooks/pre-push runs make validate)

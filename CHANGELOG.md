# Changelog

All notable changes to ci-image are documented here.
Versions follow the `YYYYMMDD-<run_number>` format used by CI builds.

<!-- BEGIN ENTRIES -->
## Revision: 20260424-8 (2026-04-24)

### Image: go1.25:20260424-8

- Removed: `helm_v3`
- Added alias: `helm_v3` → `helm`

### Image: go1.26:20260424-8

- Removed: `helm_v3`
- Added alias: `helm_v3` → `helm`

## Revision: 20260424-7 (2026-04-24)

### Universal Packages Added

- `ca-certificates`
- `gzip`
- `tar`
- `zstd`

### Image: go1.25:20260424-7

- Added package: `nodejs24`
- Added: `helm_v3` `v3.20.2`
- Universal package changes

### Image: go1.26:20260424-7

- Added package: `nodejs24`
- Added: `helm_v3` `v3.20.2`
- Universal package changes

### Image: python3.11:20260424-7

- Universal package changes

### Image: python3.13:20260424-7

- Universal package changes

### Image: node22:20260424-7

- Universal package changes

### Image: node24:20260424-7

- Universal package changes

### Image: charts:20260424-7

- Universal package changes

## Revision: 20260424-6 (2026-04-24)

### Image: charts:20260424-6

- Dockerfile template changes

### Image: go1.25:20260424-6

- Dockerfile template changes

### Image: go1.26:20260424-6

- Dockerfile template changes

### Image: node22:20260424-6

- Dockerfile template changes

### Image: node24:20260424-6

- Dockerfile template changes

### Image: python3.11:20260424-6

- Dockerfile template changes

### Image: python3.13:20260424-6

- Dockerfile template changes

## Revision: 20260424-5 (2026-04-24)

### Image: go1.25:20260424-5

- Added: `oras` `v1.3.1`

### Image: go1.26:20260424-5

- Added: `oras` `v1.3.1`

### Image: charts:20260424-5

- Added: `oras` `v1.3.1`

## Initial state (2026-04-23)

_Changelog tracking begins here. Earlier changes can be found in git history._

### Universal packages (all images)

- `docker`, `gawk`, `git-core`, `jq`, `make`, `unzip`, `wget`

### go1.25

- Base: `registry.suse.com/bci/golang:1.25.9`
- Platforms: `linux/amd64`, `linux/arm64`
- Packages: `skopeo`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `golangci-lint` v2.11.4, `goreleaser` v2.15.2, `govulncheck` v1.2.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### go1.26

- Base: `registry.suse.com/bci/golang:1.26.2`
- Platforms: `linux/amd64`, `linux/arm64`
- Packages: `skopeo`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `golangci-lint` v2.11.4, `goreleaser` v2.15.2, `govulncheck` v1.2.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### python3.11

- Base: `registry.suse.com/bci/python:3.11.15`
- Platforms: `linux/amd64`, `linux/arm64`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### python3.13

- Base: `registry.suse.com/bci/python:3.13.13`
- Platforms: `linux/amd64`, `linux/arm64`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### node22

- Base: `registry.suse.com/bci/nodejs:22.22.2`
- Platforms: `linux/amd64`, `linux/arm64`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### node24

- Base: `registry.suse.com/bci/nodejs:24.14.1`
- Platforms: `linux/amd64`, `linux/arm64`
- Tools: `cosign` v3.0.6, `gh` v2.89.0, `helm` v3.20.2, `helmv4` v4.1.4, `slsactl` v0.1.30

### charts

- Base: `registry.suse.com/bci/bci-base:15.7`
- Platforms: `linux/amd64`, `linux/arm64`
- Packages: `git`, `patch`
- Tools: `charts-build-scripts` v1.9.18, `cosign` v3.0.6, `gh` v2.89.0, `golangci-lint` v2.11.4, `goreleaser` v2.15.2, `helm` v3.20.2, `helmv4` v4.1.4, `ob-charts-tool` v0.4.0, `slsactl` v0.1.30

<!-- END ENTRIES -->

# Changelog

All notable changes to ci-image are documented here.
Versions follow the `YYYYMMDD-<run_number>` format used by CI builds.

<!-- BEGIN ENTRIES -->
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

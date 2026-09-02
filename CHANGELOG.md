# Changelog

All notable changes to ci-image are documented here.
Versions follow the `YYYYMMDD-<run_number>` format used by CI builds.

<!-- BEGIN ENTRIES -->
## Revision: 20260902-27 (2026-09-02)

### Image: charts:20260902-27

- `charts-build-scripts`: `v1.9.30` → `v1.9.32`
- Added: `release` `v0.77.1`
- Removed: `ecm-distro-tools-release`

## Revision: 20260826-26 (2026-08-26)

### Image: charts:20260826-26

- `charts-build-scripts`: `v1.9.27` → `v1.9.30`
- Added: `ecm-distro-tools-release` `v0.77.0`

## Revision: 20260826-25 (2026-08-26)

### Image: go1.25:20260826-25

- Base image: `registry.suse.com/bci/golang:1.25.12` → `registry.suse.com/bci/golang:1.25.14`

### Image: go1.26:20260826-25

- Base image: `registry.suse.com/bci/golang:1.26.5` → `registry.suse.com/bci/golang:1.26.7`

### Image: python3.11:20260826-25

- Base image: `registry.suse.com/bci/python:3.11.15` → `registry.suse.com/bci/python:3.11.15`

### Image: python3.13:20260826-25

- Base image: `registry.suse.com/bci/python:3.13.13` → `registry.suse.com/bci/python:3.13.14`

### Image: node22:20260826-25

- Base image: `registry.suse.com/bci/nodejs:22.23.1` → `registry.suse.com/bci/nodejs:22.23.2`

### Image: node24:20260826-25

- Base image: `registry.suse.com/bci/nodejs:24` → `registry.suse.com/bci/nodejs:24`

### Image: charts:20260826-25

- Base image: `registry.suse.com/bci/bci-base:15.7` → `registry.suse.com/bci/bci-base:15.7`

### Image: nix:20260826-25

- Base image: `registry.suse.com/bci/bci-base:15.7` → `registry.suse.com/bci/bci-base:15.7`

## Revision: 20260730-24 (2026-07-30)

### Image: go1.25:20260730-24

- Base image: `registry.suse.com/bci/golang:1.25.9` → `registry.suse.com/bci/golang:1.25.12`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `golangci-lint`: `v2.11.4` → `v2.12.2`
- `goreleaser`: `v2.15.2` → `v2.17.1`
- `govulncheck`: `v1.2.0` → `v1.6.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `kubectl1.29`: `v1.29.10` → `v1.29.15`
- `kubectl1.30`: `v1.30.6` → `v1.30.14`
- `kubectl1.31`: `v1.31.2` → `v1.31.14`
- `oras`: `v1.3.1` → `v1.3.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`
- Added: `kubectl1.32` `v1.32.13`
- Added: `kubectl1.33` `v1.33.13`
- Added: `kubectl1.34` `v1.34.10`
- Added: `kubectl1.35` `v1.35.7`
- Added: `kubectl1.36` `v1.36.3`
- `kubectl` selector default: `kubectl1.31` → `kubectl1.36`

### Image: go1.26:20260730-24

- Base image: `registry.suse.com/bci/golang:1.26.2` → `registry.suse.com/bci/golang:1.26.5`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `golangci-lint`: `v2.11.4` → `v2.12.2`
- `goreleaser`: `v2.15.2` → `v2.17.1`
- `govulncheck`: `v1.2.0` → `v1.6.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `kubectl1.29`: `v1.29.10` → `v1.29.15`
- `kubectl1.30`: `v1.30.6` → `v1.30.14`
- `kubectl1.31`: `v1.31.2` → `v1.31.14`
- `oras`: `v1.3.1` → `v1.3.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`
- Added: `kubectl1.32` `v1.32.13`
- Added: `kubectl1.33` `v1.33.13`
- Added: `kubectl1.34` `v1.34.10`
- Added: `kubectl1.35` `v1.35.7`
- Added: `kubectl1.36` `v1.36.3`
- `kubectl` selector default: `kubectl1.31` → `kubectl1.36`

### Image: python3.11:20260730-24

- Base image: `registry.suse.com/bci/python:3.11.15` → `registry.suse.com/bci/python:3.11.15`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`

### Image: python3.13:20260730-24

- Base image: `registry.suse.com/bci/python:3.13.13` → `registry.suse.com/bci/python:3.13.13`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`

### Image: node22:20260730-24

- Base image: `registry.suse.com/bci/nodejs:22.22.2` → `registry.suse.com/bci/nodejs:22.23.1`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`

### Image: node24:20260730-24

- Base image: `registry.suse.com/bci/nodejs:24.14.1` → `registry.suse.com/bci/nodejs:24`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`

### Image: charts:20260730-24

- Base image: `registry.suse.com/bci/bci-base:15.7` → `registry.suse.com/bci/bci-base:15.7`
- `charts-build-scripts`: `v1.9.26` → `v1.9.27`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `golangci-lint`: `v2.11.4` → `v2.12.2`
- `goreleaser`: `v2.15.2` → `v2.17.1`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `kubectl1.29`: `v1.29.10` → `v1.29.15`
- `kubectl1.30`: `v1.30.6` → `v1.30.14`
- `kubectl1.31`: `v1.31.2` → `v1.31.14`
- `oras`: `v1.3.1` → `v1.3.3`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`
- Added: `kubectl1.32` `v1.32.13`
- Added: `kubectl1.33` `v1.33.13`
- Added: `kubectl1.34` `v1.34.10`
- Added: `kubectl1.35` `v1.35.7`
- Added: `kubectl1.36` `v1.36.3`
- `kubectl` selector default: `kubectl1.31` → `kubectl1.36`

### Image: nix:20260730-24

- Base image: `registry.suse.com/bci/bci-base:15.7` → `registry.suse.com/bci/bci-base:15.7`
- `cosign`: `v3.0.6` → `v3.1.2`
- `gh`: `v2.89.0` → `v2.96.0`
- `helmv3`: `v3.20.2` → `v3.21.3`
- `helmv4`: `v4.1.4` → `v4.2.3`
- `nix`: `2.34.7` → `2.35.1`
- `slsactl`: `v0.1.30` → `v0.1.35`
- `yq`: `v4.53.2` → `v4.53.3`

## Revision: 20260720-23 (2026-07-20)

_No notable changes._
## Revision: 20260715-22 (2026-07-15)

### Image: charts:20260715-22

- `charts-build-scripts`: `v1.9.25` → `v1.9.26`
- `ob-charts-tool`: `v0.6.0` → `v0.6.1`

## Revision: 20260707-21 (2026-07-07)

### Family Selectors Added

- `kubectl` (default: `kubectl1.31`) — use `ci-select kubectl <tool>` or `select-kubectl <tool>`

### Scripts Added

- `select-kubectl` (checksum: `bd442fc9`)

### Image: go1.25:20260707-21

- Added: `kubectl1.28` `v1.28.15`
- Added: `kubectl1.29` `v1.29.10`
- Added: `kubectl1.30` `v1.30.6`
- Added: `kubectl1.31` `v1.31.2`
- Added: `kuberlr` `v0.7.0`

### Image: go1.26:20260707-21

- Added: `kubectl1.28` `v1.28.15`
- Added: `kubectl1.29` `v1.29.10`
- Added: `kubectl1.30` `v1.30.6`
- Added: `kubectl1.31` `v1.31.2`
- Added: `kuberlr` `v0.7.0`

### Image: charts:20260707-21

- `charts-build-scripts`: `v1.9.21` → `v1.9.25`
- Added: `kubectl1.28` `v1.28.15`
- Added: `kubectl1.29` `v1.29.10`
- Added: `kubectl1.30` `v1.30.6`
- Added: `kubectl1.31` `v1.31.2`
- Added: `kuberlr` `v0.7.0`

## Revision: 20260618-20 (2026-06-18)

### Scripts Added

- `ci-env-init` (checksum: `b0df0621`)
- `ci-select` (checksum: `414dbb7a`)
- `select-helm` (checksum: `34decfcb`)

## Revision: 20260611-19 (2026-06-11)

### Image: charts:20260611-19

- `charts-build-scripts`: `v1.9.20` → `v1.9.21`
- `ob-charts-tool`: `v0.5.0` → `v0.6.0`

## Revision: 20260603-18 (2026-06-03)

### Images Added

- `nix`

## Revision: 20260529-17 (2026-05-29)

### Image: go1.25:20260529-17

- Added: `yq` `v4.53.2`

### Image: go1.26:20260529-17

- Added: `yq` `v4.53.2`

### Image: python3.11:20260529-17

- Added: `yq` `v4.53.2`

### Image: python3.13:20260529-17

- Added: `yq` `v4.53.2`

### Image: node22:20260529-17

- Added: `yq` `v4.53.2`

### Image: node24:20260529-17

- Added: `yq` `v4.53.2`

### Image: charts:20260529-17

- Added: `yq` `v4.53.2`

## Revision: 20260512-16 (2026-05-12)

_No notable changes._
## Revision: 20260512-15 (2026-05-12)

### Image: charts:20260512-15

- `charts-build-scripts`: `v1.9.18` → `v1.9.20`
- `ob-charts-tool`: `v0.4.1` → `v0.5.0`

## Revision: 20260430-14 (2026-04-30)

### Universal Packages Added

- `gettext-runtime`

### Image: charts:20260430-14

- Removed package: `gettext-runtime`
- Universal package changes

### Image: go1.25:20260430-14

- Universal package changes

### Image: go1.26:20260430-14

- Universal package changes

### Image: python3.11:20260430-14

- Universal package changes

### Image: python3.13:20260430-14

- Universal package changes

### Image: node22:20260430-14

- Universal package changes

### Image: node24:20260430-14

- Universal package changes

## Revision: 20260430-13 (2026-04-30)

### Family Selectors Added

- `helm` (default: `helmv4`) — use `ci-select helm <tool>` or `select-helm <tool>`

### Image: go1.25:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`
- Added alias: `helm_v3` → `helmv3`
- Removed alias: `helm_v3`

### Image: go1.26:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`
- Added alias: `helm_v3` → `helmv3`
- Removed alias: `helm_v3`

### Image: python3.11:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`

### Image: python3.13:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`

### Image: node22:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`

### Image: node24:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`

### Image: charts:20260430-13

- Added: `helmv3` `v3.20.2`
- Removed: `helm`

## Revision: 20260429-12 (2026-04-29)

### Image: charts:20260429-12

- Dockerfile template changes

### Image: go1.25:20260429-12

- Dockerfile template changes

### Image: go1.26:20260429-12

- Dockerfile template changes

### Image: node22:20260429-12

- Dockerfile template changes

### Image: node24:20260429-12

- Dockerfile template changes

### Image: python3.11:20260429-12

- Dockerfile template changes

### Image: python3.13:20260429-12

- Dockerfile template changes

## Revision: 20260427-11 (2026-04-27)

### Image: charts:20260427-11

- Added package: `gettext-runtime`

## Revision: 20260424-10 (2026-04-24)

### Image: charts:20260424-10

- Dockerfile template changes

### Image: go1.25:20260424-10

- Dockerfile template changes

### Image: go1.26:20260424-10

- Dockerfile template changes

### Image: node22:20260424-10

- Dockerfile template changes

### Image: node24:20260424-10

- Dockerfile template changes

### Image: python3.11:20260424-10

- Dockerfile template changes

### Image: python3.13:20260424-10

- Dockerfile template changes

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

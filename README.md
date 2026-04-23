# rancher-ci-image

A generator for Rancher's CI container images.

Each image bundles a curated set of tools at pinned, checksum-verified versions — defined in [`deps.yaml`](deps.yaml), rendered into Dockerfiles, and checked in so builds are reproducible and auditable.

## Available Images

Images are published to `ghcr.io/rancher/ci-image/<name>`, each tagged independently as `<YYYYMMDD>-<run_number>` (e.g. `ghcr.io/rancher/ci-image/go1.26:20240419-42`). The date and run number come from the CI job that built them.

<!-- BEGIN IMAGES TABLE -->
| Image | Go Version | Description |
|-------|------------|-------------|
| `go1.25` | 1.25.9 | CI image with Go 1.25 toolchain |
| `go1.26` | 1.26.2 | CI image with Go 1.26 toolchain |
| `python3.11` | none | CI image with Python 3.11 toolchain |
| `python3.13` | none | CI image with Python 3.13 toolchain |
| `node22` | none | CI image with Node 22 toolchain |
| `node24` | none | CI image with Node 24 toolchain |
| `charts` | none | Rancher charts build environment |
<!-- END IMAGES TABLE -->

## Usage

Use an image as the container for a job in your GitHub Actions workflow. The job's steps run inside the container, so all bundled tools are available without any install step.

**GitHub-hosted runners** (`ubuntu-latest`):

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/rancher/ci-image/go1.26:latest
    steps:
      - uses: actions/checkout@<sha256>

      - name: Git safe directory
        shell: bash
        run: git config --global --add safe.directory "$(pwd)"

      - run: go build ./...
```

**SUSE self-hosted runners**:

```yaml
jobs:
  build:
    runs-on: 
      labels:
        - runs-on
        - spot=<option>
        - runner=<option>
        - run-id=${{ github.run_id }}
    container:
      image: ghcr.io/rancher/ci-image/go1.26:latest
    steps:
      - uses: actions/checkout@<sha256>

      - name: Git safe directory
        shell: bash
        run: git config --global --add safe.directory "$(pwd)"

      - run: go build ./...
```

Pin to a specific date-stamped tag (e.g. `go1.26:20240419-42`) for fully reproducible workflows.

---

For adding tools, new Go versions, or modifying the build system, see [CONTRIBUTING.md](CONTRIBUTING.md).

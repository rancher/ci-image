# rancher-ci-image

A generator for Rancher's CI container images.

Each image bundles a curated set of tools at pinned, checksum-verified versions — defined in [`deps.yaml`](deps.yaml), rendered into Dockerfiles, and checked in so builds are reproducible and auditable.

## Available Images

Images are published to `ghcr.io/rancher/ci-image/<name>`, each tagged independently as `<YYYYMMDD>-<run_number>` (e.g. `ghcr.io/rancher/ci-image/go1.26:20240419-42`). The date and run number come from the CI job that built them.

<!-- BEGIN IMAGES TABLE -->
| Image | Go Version | Description |
|-------|------------|-------------|
| `go1.25` | 1.25.14 | CI image with Go 1.25 toolchain |
| `go1.26` | 1.26.7 | CI image with Go 1.26 toolchain |
| `python3.11` | none | CI image with Python 3.11 toolchain |
| `python3.13` | none | CI image with Python 3.13 toolchain |
| `node22` | none | CI image with Node 22 toolchain |
| `node24` | none | CI image with Node 24 toolchain |
| `charts` | none | Rancher charts build environment |
| `nix` | none | Nix environment |
<!-- END IMAGES TABLE -->

## Changelog

Per-build changes are tracked in [the `CHANGELOG.md` file](https://github.com/rancher/ci-image/blob/changelog/CHANGELOG.md) on the `changelog` branch.

Each entry is generated automatically after a successful push to `main` and records what changed in `images-lock.yaml` — tool version bumps, base image updates, packages added or removed, and images added or removed.

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

## Advanced Features

### Tool Family Selection

Some tools are available in multiple versions grouped into "families". For example, the `helm` family includes both `helmv3` and `helmv4`. You can select which version to use as the default command for that family.

**Environment Variable Configuration (Recommended):**

The easiest way to configure tool families is by setting `SELECT_{FAMILY}_VERSION` environment variables. The container entrypoint will automatically configure the selected version before your command runs.

**In GitHub Actions:**
```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/rancher/ci-image/charts:latest
    env:
      SELECT_HELM_VERSION: helmv4  # Use Helm v4 as the default 'helm' command
    steps:
      - run: helm version  # Runs helmv4
```

**Direct Docker CLI:**
```bash
# NOTE: -e flag must come BEFORE the image name
docker run -e SELECT_HELM_VERSION=helmv3 ghcr.io/rancher/ci-image/charts:latest helm version

# Interactive shell with specific version
docker run -e SELECT_HELM_VERSION=helmv3 -it ghcr.io/rancher/ci-image/charts:latest
```

**Manual Selection:**

You can also manually select a tool version within a job step using the `ci-select` command:

```yaml
steps:
  - name: Configure Helm v3
    run: ci-select helm helmv3
  
  - name: Deploy with Helm v3
    run: helm install myapp ./chart
```

Or use the family-specific wrapper:

```yaml
steps:
  - run: select-helm helmv4
  - run: helm version  # Now uses helmv4
```

**How it works:**

Tool families use symlinks in `/var/ci-tools/active/` (on PATH ahead of `/usr/local/bin`) to point to the selected version. Each family has a default version that is active when the container starts, unless overridden by `SELECT_{FAMILY}_VERSION` environment variables.

### Tool Installation Hooks

You can customize tool installation by adding pre-install or post-install hooks. Hooks are Go templates placed in the `hooks/` directory that inject additional Dockerfile commands before or after a tool is installed.

**Hook naming convention:**
- `hooks/{toolname}-pre.tmpl` - runs before tool installation
- `hooks/{toolname}-post.tmpl` - runs after tool installation

**Use cases:**
- Create directories or configuration files before installation
- Run custom install scripts that come with the tool
- Set up environment variables or permissions
- Clean up temporary files after installation

**Example pre-hook:**
```dockerfile
# hooks/mytool-pre.tmpl
RUN mkdir -p /opt/mytool-config
RUN echo "config_option=value" > /opt/mytool-config/settings.conf
```

**Example post-hook:**
```dockerfile
# hooks/mytool-post.tmpl
RUN /var/ci-tools/mytool/install.sh --daemon
RUN chmod +x /usr/local/bin/mytool
```

**Accessing tool files:** For tools that include install scripts or other files, set `install_to_path: false` in the `release:` block in `deps.yaml`. This extracts the tool to `/var/ci-tools/{toolname}/` instead of installing directly to `/usr/local/bin`, making all files available to hooks:

```yaml
tools:
  - name: mytool
    source: owner/repo
    version: v1.0.0
    checksums:
      linux/amd64: "checksum-here"
      linux/arm64: "checksum-here"
    release:
      download_template: "mytool-{version}-{os}-{arch}.tar.gz"
      extract: "mytool-{version}-{os}-{arch}"
      install_to_path: false  # Extract to /var/ci-tools/mytool/ for hooks
```

Hook changes are automatically tracked in the changelog and `images-lock.yaml` with MD5 checksums, ensuring any modification triggers a rebuild of affected images.

---

For adding tools, new Go versions, or modifying the build system, see [CONTRIBUTING.md](CONTRIBUTING.md).

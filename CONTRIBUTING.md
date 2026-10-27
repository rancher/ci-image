# Contributing

For a high-level overview of the project, see [README.md](README.md).

## How Images Get Built

1. Images and tool versions are defined in [`deps.yaml`](deps.yaml).
2. Dockerfile templates live in [`internal/dockerfile/tmpl/`](internal/dockerfile/tmpl/) and are embedded into the generator binary at compile time.
3. `make generate` renders the templates into `dockerfiles/Dockerfile.*` and updates `images-lock.yaml`.
4. `make verify` ensures no uncommitted changes exist (run in CI).
5. `make push` builds and pushes multi-arch images to GHCR.

## GitHub Access

Release lookups, checksum downloads and template rendering go through the vendored [`ghreleases`](https://github.com/mallardduck/ghreleases) package. Requests are restricted to GitHub hosts (`github.com` and `githubusercontent.com`, including redirects), and `GITHUB_TOKEN` is only ever sent to GitHub hosts. Template variables available in `deps.yaml` are `{name}`, `{source}`, `{version}`, `{version_commit}`, `{os}` and `{arch}`; unknown variables and modifiers are errors.

## Adding a New Tool

`deps.yaml` has two sections for tools:

- **`universal:`** — tools installed in *every* image (e.g. `cosign`, `gh`). Add here if the tool should be universally available.
- **`tools:`** — tools that are opt-in per image (e.g. `govulncheck`). Add here, then reference the tool name under the relevant image's `tools:` list.

Both require the same fields:

```yaml
- name: mytool
  source: org/repo          # GitHub org/repo, or a full URL for static sources
  version: v1.2.3
  checksums:
    linux/amd64: "<sha256>"
    linux/arm64: "<sha256>"
  release:
    download_template: "mytool-{version}-{os}-{arch}.tar.gz"
    extract: "mytool"       # path of the binary inside the archive; omit for plain binaries
```

Optional `release:` fields tell `manual-updates` where upstream publishes checksums: `checksum_template` for a single `<sha256>  <filename>` file, or a per-platform `<download>.sha256sum` is assumed if omitted. Some projects (e.g. `yq`) publish one file with many hash algorithms per line; set `checksum_hashes_order_template` alongside `checksum_template` to name the file that lists the column order. The SHA-256 column is then selected by name, never by position or shape (SHA3-256, GOST and BLAKE2s are also 64 hex characters), and the file is rejected if anything is ambiguous.

After editing, run `make generate` and commit the updated Dockerfiles.

## Updating Pinned Tools and Base Images

`make update` only refreshes `deps.lock` for `release-checksums` tools. GitHub-sourced tools with `version:` and `checksums:` in `deps.yaml`, and (with `--base-images`) the `base:` image digests, are updated with:

```sh
make manual-updates
```

This is an interactive review (there is deliberately no `--yes`/`--all` flag, and it refuses to run without a terminal). It looks up newer releases, then for each one shows the version change and the old/new checksums, and where the checksums were read from (the tool's own `release:` config). You stage or skip each update; major bumps require typing `yes`. Nothing is written until you confirm at the end, and `deps.yaml` is edited in place so comments and formatting survive. If upstream publishes no checksum for a tool (e.g. `cosign`), the review offers to download each artifact and hash it locally. This is opt-in per tool and clearly labelled, because it only proves the bytes match what was downloaded rather than what upstream published. Set `GITHUB_TOKEN` to avoid GitHub API rate limits.

To review just some tools, name them: `go run main.go manual-updates goreleaser oras` (or `make manual-updates ARGS="goreleaser oras"`). A family name selects all its members, and an unknown name is an error. Tool names cannot be combined with `--base-images`.

Base images are only checked when you pass `--base-images` (`go run main.go manual-updates --base-images`); by default only tools are reviewed. They follow `registry.suse.com` tags: a full `x.y.z` tag moves to the newest patch of the same `x.y`, while floating tags (`24`, `15.7`) keep their tag and only refresh the digest. Newer lines (such as a new Go minor) are listed for your attention but never proposed.

Only tools whose `source:` is a GitHub repository are checked. Tools with other sources (for example the kubectl and helm tools and nix) are listed as "not GitHub-sourced" and left alone; update them by hand.

## Adding a Tool Family

Tool families allow multiple versions of a tool to coexist in the same image, with a runtime selector to choose which one is active. This is useful for tools like Helm where you might need to support both v3 and v4.

To add a tool family:

1. Add multiple tools with the same `family` field in `deps.yaml`:

```yaml
tools:
  - name: helmv3
    family: helm
    source: "https://get.helm.sh"
    version: v3.20.2
    checksums:
      linux/amd64: "<sha256>"
      linux/arm64: "<sha256>"
    release:
      download_template: "{source}/helm-{version}-{os}-{arch}.tar.gz"
      extract: "{os}-{arch}/helm"

  - name: helmv4
    family: helm
    family_default: true  # This version is active by default
    source: "https://get.helm.sh"
    version: v4.1.4
    checksums:
      linux/amd64: "<sha256>"
      linux/arm64: "<sha256>"
    release:
      download_template: "{source}/helm-{version}-{os}-{arch}.tar.gz"
      extract: "{os}-{arch}/helm"
```

2. Run `make generate`. This will:
   - Create a `select-{family}.sh` script (e.g. `select-helm.sh`)
   - Add the family to the `ci-select` command
   - Set up the default symlink in `/var/ci-tools/active/`

3. Users can then select a version using:
   - Environment variable: `SELECT_HELM_VERSION=helmv3`
   - Runtime command: `ci-select helm helmv3` or `select-helm helmv3`

Only one tool in a family should have `family_default: true`.

## Adding a New Go Version

1. Add a new entry under `images:` in `deps.yaml` with the desired Go base image (e.g. `registry.suse.com/bci/golang:1.X.Y@sha256:...`) and a name like `go1.X`.
2. Run `make generate` and commit the new Dockerfile.

## Base image tags

Base tags are read as `VERSION[-VARIANT][-REVISION]`: `24.18.1-base-13.2` is version `24.18.1`, variant `base`, build revision `13.2`; `1.26.10-dev-36.3` is `1.26.10`, `dev`, `36.3`; `1.27.2-1.1` is `1.27.2` with revision `1.1`. `manual-updates --base-images` moves a tag to the newest version in the same `X.Y` line and, within that, the newest revision, so `1.26.9-dev-34.1` can become `1.26.10-dev-36.3` and `24.18.1-base-13.1` can become `24.18.1-base-13.2`. The variant must match, and a tag with a revision only moves to tags that also have one. Moving to a new `X.Y` line is a deliberate manual change. Prerelease tags such as `1.27.3-rc1` are never matched.

## Archive

When `make generate` detects that a `dockerfiles/Dockerfile.<name>` no longer has a corresponding image in `deps.yaml`, it moves the file to `archive/Dockerfile.<name>.<YYYYMMDD-HHMMSS>` (UTC). Commit the archived file alongside the removal in `deps.yaml`.

## Local Development

```bash
# Generate Dockerfiles from templates and deps.yaml
make generate

# Build all images locally
make build

# Build and push to registry
make push REPO=ghcr.io/rancher/ci-image

# Run tests, generate, and build
make all
```

## Development Workflow

Generated Dockerfiles are checked in to the repository — they are not built on-the-fly. The expected workflow for any change to `deps.yaml` or the templates is:

1. Make your changes.
2. Run `make generate` to regenerate `dockerfiles/` and `images-lock.yaml`.
3. Commit both the source changes and the generated output together.

`make verify` (run in CI) fails if there are uncommitted generated files, so a PR with stale output will not pass.

## Generated Files

- **`dockerfiles/Dockerfile.<name>`** — the rendered Dockerfile for each image. Do not edit these directly; edit the templates instead.
- **`images-lock.yaml`** — a compiled index generated by `make generate`. Records the active image names, resolved base images, platforms, tool set, and resolved versions for `latest`-pinned tools. Do not edit manually.
- **`archive/Dockerfile.<name>.<YYYYMMDD-HHMMSS>`** — when `make generate` detects that a Dockerfile no longer has a corresponding image in `deps.yaml`, it moves it here rather than deleting it. Commit these alongside any image removal.

## Template System

Dockerfiles are generated by the Go program in [`main.go`](main.go) using Go's `text/template` package. The templates live in [`internal/dockerfile/tmpl/`](internal/dockerfile/tmpl/) and are embedded into the binary at compile time.

- **`dockerfile.tmpl`** — root template that assembles a complete Dockerfile. It calls the component templates below via `{{template ...}}` and `.Install.Render()`.
- **`zypper.tmpl`** — renders the `zypper install` layer for system packages.
- **`curl_binary.tmpl`**, **`curl_gzip.tmpl`**, **`curl_archive.tmpl`** — render the `curl`-based install layer for a tool, varying by download format (plain binary, `.gz`, or `.tar.gz`/`.zip`).

To change how Dockerfiles are structured, edit the relevant template. To add a new install format, add a new `curl_<format>.tmpl` and wire it up in [`internal/dockerfile/install.go`](internal/dockerfile/install.go).

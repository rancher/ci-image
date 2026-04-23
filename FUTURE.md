# Future Considerations

This is not a backlog or TODO list. It documents decisions we've already made (things that are explicitly out of scope) alongside ideas that could change the direction of this project but haven't been agreed on yet. Before any of the items in the second section get built, they need design discussion and alignment — not just implementation.

---

## Non-Goals

These are explicit decisions. They are not deferred work; they reflect deliberate scope choices that keep this tool focused.

| What | Why |
|---|---|
| **Local developer tooling** | This tool builds CI images. `dep-fetch` handles per-repo local tooling. The two are intentionally separate. |
| **Non-SUSE base images** | All target images are SUSE BCI. The zypper install block is generated directly from config with no distro abstraction. Adding `apt`/`dnf` support would generalize the tool beyond its purpose. |
| **Windows platform support** | CI workloads run on Linux. `darwin/*` is also not a target — this is not a local dev tool. |
| **Plugin system or tool registry** | Trust is anchored in the compiled-in install method list and `mode: pinned`. Runtime-configurable trust models introduce risk without clear benefit. |
| **`dep-fetch add` equivalent** | Adding a new tool requires a manual `deps.yaml` edit. Automating this is blocked by YAML comment preservation complexity, and the manual process is intentional — new tools should be reviewed. |

---

## Ideas That Need Design First

These are worth considering but should not be built without explicit design discussion and agreement. Some may turn out to be good ideas; others may reveal reasons to stay out of scope once examined more carefully.

### Receipt / audit trail system

dep-fetch writes per-tool receipt files after a successful install, recording the installed version and checksum hashes. A similar mechanism here could create a committed, git-diffable audit log of tool checksum changes over time — making it visible in PRs when an upstream release changes its published checksums.

Open questions before building:
- Where do receipt files live in the repo, and what do they look like?
- How do they interact with `generate` — are they always rewritten, or only on change?
- What is the story for `go-install` tools, which have no downloadable archive to hash? (`sum.golang.org` is one option but adds complexity.)
- Does a `verify` command that re-fetches upstream `checksums.txt` and compares against receipts belong here, or is it out of scope?

### Post-extract checksum verification

Currently checksums are always verified against the raw downloaded file (archive or compressed binary), matching what upstream release checksum files contain. Some tools may instead publish checksums of the extracted binary.

### `wget` as an additional install method

The install method registry is designed to make adding `wget` a low-effort change (one constant, one RUN block template nearly identical to the `curl` block). Currently deferred because `curl` covers all existing tools. Worth adding when there's a concrete tool that requires it, not speculatively.

### Additional `update` command scope

The `update` command targets `curl`-based `pinned` tools. Extensions worth considering:

- **`go-install` tools**: update `version:` and `version_commit:` by querying the GitHub API or the Go module proxy. Less clear-cut than asset-based tools.
- **Base image digest updates**: update `image.base` digests when a new BCI image is published. Would need a way to resolve the latest digest for a given BCI image tag.
- **Bulk update**: `update --all` to update every tool in one pass. Raises questions about atomicity (what if one tool fails mid-run).

Note: `release-checksums` + `version: latest` tools do not need `update` — their version is resolved automatically by `generate` and tracked in `deps.lock`.

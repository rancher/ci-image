#!/usr/bin/env bash
# changelog-local.sh — simulate or apply CI changelog generation locally.
#
# Environment variables (all optional):
#   FROM     git ref for the "before" state (default: merge-base of HEAD against origin/main)
#   TO       git ref for the "after" state  (default: HEAD)
#   VERSION  version string for the entry   (default: YYYYMMDD-local)
#   APPLY    set to 1 to commit the entry into the local changelog-dir worktree
#            (default: dry-run only)
#   PUSH     set to 1 (requires APPLY=1) to also push to origin/changelog
#
# Commit hashes, branch names, and tags all work for FROM/TO.
#
# Examples:
#   make changelog-local                                     # dry-run, current branch vs origin/main
#   FROM=abc1234 TO=def5678 make changelog-local             # dry-run, specific commits
#   VERSION=20260424-99 APPLY=1 make changelog-local          # commit to local worktree only
#   VERSION=20260424-99 APPLY=1 PUSH=1 make changelog-local  # commit and push to origin/changelog
set -euo pipefail

FROM="${FROM:-$(git merge-base HEAD origin/main)}"
TO="${TO:-HEAD}"
VERSION="${VERSION:-$(date -u +%Y%m%d)-local}"
APPLY="${APPLY:-0}"
PUSH="${PUSH:-0}"

WORKTREE="./changelog-dir"

setup_worktree() {
  # Always fetch first so both paths below start from the latest remote state.
  local branch_exists=false
  if git ls-remote --exit-code origin changelog &>/dev/null; then
    git fetch origin changelog --quiet
    branch_exists=true
  fi

  if [ -d "$WORKTREE" ]; then
    # Worktree already exists — refuse to proceed if there are uncommitted changes.
    if ! git -C "$WORKTREE" diff --quiet || ! git -C "$WORKTREE" diff --cached --quiet; then
      echo "Error: $WORKTREE has uncommitted changes. Commit or discard them first." >&2
      exit 1
    fi
    # Reset to the freshly-fetched remote state.
    git -C "$WORKTREE" reset --hard origin/changelog --quiet
  else
    # The orphan branch won't exist on forks or a brand-new repo on first run.
    if $branch_exists; then
      git worktree add "$WORKTREE" origin/changelog --quiet
    else
      git worktree add --orphan "$WORKTREE" changelog --quiet
    fi
  fi
}

cleanup_worktree() {
  git worktree remove "$WORKTREE" --force 2>/dev/null || true
}

echo "Changelog generation:"
echo "  FROM    = $(git rev-parse --short "$FROM") ($(git log -1 --format='%s' "$FROM"))"
echo "  TO      = $(git rev-parse --short "$TO") ($(git log -1 --format='%s' "$TO"))"
echo "  VERSION = $VERSION"
echo "  APPLY   = $APPLY"
echo "  PUSH    = $PUSH"
echo ""

if [ "$APPLY" = "1" ]; then
  # Write directly into the changelog worktree, then commit and push — same as CI.
  trap 'cleanup_worktree' EXIT
  setup_worktree

  go run main.go changelog generate \
    --version="$VERSION" \
    --from="$FROM" \
    --to="$TO" \
    --changelog="$WORKTREE/CHANGELOG.md"

  git -C "$WORKTREE" add CHANGELOG.md

  if git -C "$WORKTREE" diff --cached --quiet; then
    echo "CHANGELOG.md unchanged, nothing to commit."
    exit 0
  fi

  git -C "$WORKTREE" commit -m "chore: update CHANGELOG.md for ${VERSION}"

  if [ "$PUSH" = "1" ]; then
    git -C "$WORKTREE" push origin HEAD:changelog
    echo "Pushed to origin/changelog."
  else
    echo "Committed to local changelog-dir. Run with PUSH=1 or 'git push origin HEAD:changelog' from changelog-dir to publish."
  fi
else
  # Dry-run: generate into a temp file and print the new entry.
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  CHANGELOG="$TMPDIR/CHANGELOG.md"

  if git ls-remote --exit-code origin changelog &>/dev/null; then
    git fetch origin changelog --quiet
    git show origin/changelog:CHANGELOG.md > "$CHANGELOG" 2>/dev/null || true
  fi

  go run main.go changelog generate \
    --version="$VERSION" \
    --from="$FROM" \
    --to="$TO" \
    --changelog="$CHANGELOG"

  echo "--- Generated changelog entry (dry-run) ---"
  awk '/<!-- BEGIN ENTRIES -->/{found=1; next} found && /<!-- END ENTRIES -->/{exit} found{print}' "$CHANGELOG" \
    | awk 'NR>1 && /^## /{exit} {print}'
fi

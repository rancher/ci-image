#!/usr/bin/env bash
# changelog-local.sh — simulate CI changelog generation locally.
#
# Environment variables (all optional):
#   FROM     git ref for the "before" state (default: merge-base of HEAD against origin/main)
#   TO       git ref for the "after" state  (default: HEAD)
#   VERSION  version string for the entry   (default: YYYYMMDD-local)
#
# Commit hashes, branch names, and tags all work for FROM/TO.
#
# Examples:
#   bash scripts/changelog-local.sh
#   FROM=abc1234 TO=def5678 bash scripts/changelog-local.sh
#   VERSION=20260424-99 bash scripts/changelog-local.sh
set -euo pipefail

FROM="${FROM:-$(git merge-base HEAD origin/main)}"
TO="${TO:-HEAD}"
VERSION="${VERSION:-$(date -u +%Y%m%d)-local}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

CHANGELOG="$TMPDIR/CHANGELOG.md"

# Restore existing CHANGELOG.md from the orphan changelog branch — mirrors the CI step.
if git ls-remote --exit-code origin changelog &>/dev/null; then
  git fetch origin changelog --quiet
  git show origin/changelog:CHANGELOG.md > "$CHANGELOG" 2>/dev/null || true
fi

echo "Simulating changelog generation:"
echo "  FROM    = $(git rev-parse --short "$FROM") ($(git log -1 --format='%s' "$FROM"))"
echo "  TO      = $(git rev-parse --short "$TO") ($(git log -1 --format='%s' "$TO"))"
echo "  VERSION = $VERSION"
echo ""

go run main.go changelog generate \
  --version="$VERSION" \
  --from="$FROM" \
  --to="$TO" \
  --changelog="$CHANGELOG"

echo "--- Generated changelog entry ---"
# Extract just the new entry (everything between BEGIN ENTRIES marker and the second ## heading or END marker)
awk '/<!-- BEGIN ENTRIES -->/{found=1; next} found && /<!-- END ENTRIES -->/{exit} found{print}' "$CHANGELOG" \
  | awk 'NR>1 && /^## /{exit} {print}'

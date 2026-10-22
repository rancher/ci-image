// Package updater finds newer versions of the tools and base images declared
// in deps.yaml and applies reviewed changes back to the file in place.
//
// It deliberately has no non-interactive entry point that writes anything:
// discovery is read-only, and edits are only produced from explicit decisions
// made by the caller.
package updater

import (
	"strconv"
	"strings"
)

// Bump classifies how far a version moves.
type Bump int

const (
	BumpPatch Bump = iota
	BumpMinor
	BumpMajor // includes minor bumps of 0.x versions, which are breaking by convention
)

func (b Bump) String() string {
	switch b {
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "MAJOR"
	default:
		return "patch"
	}
}

// parseVersion parses strictly dotted-numeric versions. Callers strip any tag
// prefix (the tool's tag_prefix, e.g. "v" or "api/v") first; a leftover prefix
// makes the version unparseable rather than silently comparable.
func parseVersion(s string) ([]int, bool) {
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

func compareNums(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Classify reports how next differs from cur. ok is false when either version
// is not dotted-numeric or next is not strictly newer.
func Classify(cur, next string) (b Bump, ok bool) {
	c, ok1 := parseVersion(cur)
	n, ok2 := parseVersion(next)
	if !ok1 || !ok2 || compareNums(c, n) >= 0 {
		return 0, false
	}
	at := func(v []int, i int) int {
		if i < len(v) {
			return v[i]
		}
		return 0
	}
	switch {
	case at(c, 0) != at(n, 0):
		return BumpMajor, true
	case at(c, 1) != at(n, 1):
		if at(c, 0) == 0 {
			return BumpMajor, true
		}
		return BumpMinor, true
	default:
		return BumpPatch, true
	}
}

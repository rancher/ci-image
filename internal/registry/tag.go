package registry

import (
	"regexp"
	"strconv"
	"strings"
)

// Tag is a parsed image tag of the form VERSION[-VARIANT][-REVISION], as used
// by SUSE BCI and Application Collection:
//
//	1.26.10              version only
//	24.18.1-base         variant
//	24.18.1-base-13.2    variant and build revision
//	1.27.2-1.1           build revision, no variant
//	1.26.10-dev-36.3     variant and build revision
//
// The variant names a flavour of the image and must match when moving between
// tags. The revision is a build counter that is free to increment.
type Tag struct {
	Version  []int  // dotted-numeric version, e.g. [1 26 10]
	Variant  string // hyphen-joined words ("dev", "base"); empty if none
	Revision []int  // build revision, e.g. [36 3]; nil if none
}

// tagRe: a dotted version, then optional hyphen-joined alphabetic words (the
// variant), then an optional dotted build revision. Words are letters only so
// prereleases like "rc1" are not mistaken for a variant.
var tagRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)(?:-([A-Za-z]+(?:-[A-Za-z]+)*))?(?:-(\d+(?:\.\d+)*))?$`)

// ParseTag parses a tag, reporting false for anything that is not of that
// shape (prefixed versions, prereleases like "rc1", floating names like
// "latest").
func ParseTag(s string) (Tag, bool) {
	m := tagRe.FindStringSubmatch(s)
	if m == nil {
		return Tag{}, false
	}
	ver, ok := parseDotted(m[1])
	if !ok {
		return Tag{}, false
	}
	t := Tag{Version: ver, Variant: m[2]}
	if m[3] != "" {
		if t.Revision, ok = parseDotted(m[3]); !ok {
			return Tag{}, false
		}
	}
	return t, true
}

func parseDotted(s string) ([]int, bool) {
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// SameFlavour reports whether o is a candidate to replace t: the same variant,
// and either both or neither carry a revision.
func (t Tag) SameFlavour(o Tag) bool {
	return t.Variant == o.Variant && (t.Revision == nil) == (o.Revision == nil)
}

// Compare orders tags by version, then by revision: -1, 0 or 1.
func (t Tag) Compare(o Tag) int {
	if c := compareDotted(t.Version, o.Version); c != 0 {
		return c
	}
	return compareDotted(t.Revision, o.Revision)
}

// compareDotted compares numerically, treating missing components as zero.
func compareDotted(a, b []int) int {
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

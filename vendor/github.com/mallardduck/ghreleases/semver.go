package ghrelease

import (
	"strconv"
	"strings"
)

// parsedVersion is a semver 2.0.0 version with build metadata discarded.
type parsedVersion struct {
	major, minor, patch uint64
	pre                 []string
}

// parseVersion parses a release tag such as "v1.2.3-rc.1" or "cli/v1.2.3".
// Anything up to and including the last "/" is treated as a tag prefix and a
// leading "v" is optional. It reports false for tags that are not strict
// MAJOR.MINOR.PATCH semver.
func parseVersion(tag string) (parsedVersion, bool) {
	if i := strings.LastIndex(tag, "/"); i >= 0 {
		tag = tag[i+1:]
	}
	tag = strings.TrimPrefix(tag, "v")
	if i := strings.IndexByte(tag, '+'); i >= 0 {
		tag = tag[:i]
	}

	var v parsedVersion
	core := tag
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		core = tag[:i]
		v.pre = strings.Split(tag[i+1:], ".")
		for _, id := range v.pre {
			// Numeric identifiers must not have leading zeros (semver §9).
			if id == "" || (isDigits(id) && hasLeadingZero(id)) {
				return parsedVersion{}, false
			}
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}
	nums := [3]*uint64{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		// ParseUint also rejects the empty string and overflow; leading zeros
		// are forbidden by semver §2.
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || hasLeadingZero(p) {
			return parsedVersion{}, false
		}
		*nums[i] = n
	}
	return v, true
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func hasLeadingZero(s string) bool { return len(s) > 1 && s[0] == '0' }

// cmpNumeric compares two digit strings without parsing them, so identifiers
// too large for any integer type still order correctly. Neither has a leading zero.
func cmpNumeric(a, b string) int {
	if c := cmpUint(uint64(len(a)), uint64(len(b))); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// comparePre compares prerelease identifier lists per semver 2.0.0 §11.
// An empty list (a normal release) outranks any prerelease.
func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		an, bn := isDigits(a[i]), isDigits(b[i])
		var c int
		switch {
		case an && bn:
			c = cmpNumeric(a[i], b[i])
		case an:
			c = -1 // numeric identifiers sort below alphanumeric ones
		case bn:
			c = 1
		default:
			c = strings.Compare(a[i], b[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmpUint(uint64(len(a)), uint64(len(b)))
}

// CompareVersions compares two release tags by semver precedence: major,
// minor and patch first, then prerelease identifiers as a sub-sort. Build
// metadata is ignored, a leading "v" is optional, and a tag prefix such as
// "cli/" is stripped before parsing. It returns -1, 0 or 1, and ok is false
// when either tag is not valid semver.
func CompareVersions(a, b string) (cmp int, ok bool) {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	if !okA || !okB {
		return 0, false
	}
	if c := cmpUint(va.major, vb.major); c != 0 {
		return c, true
	}
	if c := cmpUint(va.minor, vb.minor); c != 0 {
		return c, true
	}
	if c := cmpUint(va.patch, vb.patch); c != 0 {
		return c, true
	}
	return comparePre(va.pre, vb.pre), true
}

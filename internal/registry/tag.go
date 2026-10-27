package registry

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

// ParseTag parses a tag, reporting false for anything that is not of that
// shape (prefixed versions, prereleases like "rc1", floating names like
// "latest").
func ParseTag(s string) (Tag, bool) {
	return Tag{}, false
}

// SameFlavour reports whether o is a candidate to replace t: the same variant,
// and either both or neither carry a revision.
func (t Tag) SameFlavour(o Tag) bool {
	return false
}

// Compare orders tags by version, then by revision: -1, 0 or 1.
func (t Tag) Compare(o Tag) int {
	return 0
}

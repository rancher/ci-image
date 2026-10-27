package registry

import (
	"slices"
	"testing"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		in       string
		ok       bool
		version  []int
		variant  string
		revision []int
	}{
		{"1.26.10", true, []int{1, 26, 10}, "", nil},
		{"24", true, []int{24}, "", nil},
		{"15.7", true, []int{15, 7}, "", nil},
		{"24.18.1-base", true, []int{24, 18, 1}, "base", nil},
		{"24.18.1-base-13.2", true, []int{24, 18, 1}, "base", []int{13, 2}},
		{"1.27.2-1.1", true, []int{1, 27, 2}, "", []int{1, 1}}, // BCI build number
		{"1.26.9-dev-34.1", true, []int{1, 26, 9}, "dev", []int{34, 1}},
		{"1.26.10-dev-36.3", true, []int{1, 26, 10}, "dev", []int{36, 3}},
		{"1.26.10-dev-slim-36", true, []int{1, 26, 10}, "dev-slim", []int{36}},

		{"latest", false, nil, "", nil},
		{"", false, nil, "", nil},
		{"v1.2.3", false, nil, "", nil},
		{"1.27.3-rc1", false, nil, "", nil},          // prerelease, not a variant
		{"1.27.4-dev-14.2-rc1", false, nil, "", nil}, // nothing may follow the revision
		{"1.27.4-14.2-dev", false, nil, "", nil},     // revision comes last
		{"1.27.4-", false, nil, "", nil},
		{"1..4", false, nil, "", nil},
	}
	for _, tt := range tests {
		got, ok := ParseTag(tt.in)
		if ok != tt.ok {
			t.Errorf("ParseTag(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if !slices.Equal(got.Version, tt.version) || got.Variant != tt.variant || !slices.Equal(got.Revision, tt.revision) {
			t.Errorf("ParseTag(%q) = %+v, want version=%v variant=%q revision=%v", tt.in, got, tt.version, tt.variant, tt.revision)
		}
	}
}

func mustTag(t *testing.T, s string) Tag {
	t.Helper()
	tag, ok := ParseTag(s)
	if !ok {
		t.Fatalf("ParseTag(%q) failed", s)
	}
	return tag
}

func TestTagSameFlavour(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.26.9-dev-34.1", "1.26.10-dev-36.2", true}, // version and revision both move
		{"1.26.9-dev-34.1", "1.26.9-dev-34.2", true},  // revision-only rebuild
		{"24.18.1-base-13.1", "24.18.1-base-13.2", true},
		{"1.27.2-1.1", "1.27.10-2.3", true},            // BCI build number
		{"24.18.1-base", "24.18.2-base", true},         // both unrevisioned
		{"1.26.9-dev-34.1", "1.26.9-slim-34.1", false}, // different variant
		{"1.26.9-dev-34.1", "1.26.9", false},           // plain tag is another flavour
		{"1.26.9-dev-34.1", "1.26.9-dev", false},       // revisioned never moves to unrevisioned
		{"1.26.9", "1.26.9-1.1", false},
	}
	for _, tt := range tests {
		if got := mustTag(t, tt.a).SameFlavour(mustTag(t, tt.b)); got != tt.want {
			t.Errorf("SameFlavour(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestTagCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.26.9-dev-34.1", "1.26.10-dev-33.9", -1}, // version beats revision
		{"1.26.10-dev-36.2", "1.26.10-dev-36.3", -1},
		{"1.26.10-dev-36.10", "1.26.10-dev-36.9", 1}, // numeric, not lexical
		{"1.26.10-dev-37.1", "1.26.10-dev-36.9", 1},
		{"1.26.10-dev-36.2", "1.26.10-dev-36.2", 0},
		{"1.26.10", "1.26.9", 1},
	}
	for _, tt := range tests {
		if got := mustTag(t, tt.a).Compare(mustTag(t, tt.b)); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

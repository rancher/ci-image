package updater

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		cur, next string
		bump      Bump
		ok        bool
	}{
		{"1.2.3", "1.2.4", BumpPatch, true},
		{"1.2.3", "1.3.0", BumpMinor, true},
		{"1.2.3", "2.0.0", BumpMajor, true},
		{"0.7.0", "0.8.0", BumpMajor, true}, // 0.x minor is breaking
		{"0.7.0", "0.7.1", BumpPatch, true},
		{"2.35.1", "2.36.0", BumpMinor, true},
		{"1.10.0", "1.9.9", 0, false}, // numeric, not lexical
		{"1.2.3", "1.2.3", 0, false},
		{"1.2.3", "1.2.3-rc1", 0, false},
		{"1.2.3", "database/v1.2.4", 0, false},
		{"1.2.3", "v1.2.4", 0, false}, // a leftover prefix is not a version
		{"v1.2.3", "v1.2.4", 0, false},
	}
	for _, tt := range tests {
		b, ok := Classify(tt.cur, tt.next)
		if ok != tt.ok || (ok && b != tt.bump) {
			t.Errorf("Classify(%q,%q) = %v,%v; want %v,%v", tt.cur, tt.next, b, ok, tt.bump, tt.ok)
		}
	}
}

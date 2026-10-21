package updater

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		cur, next string
		bump      Bump
		ok        bool
	}{
		{"v1.2.3", "v1.2.4", BumpPatch, true},
		{"v1.2.3", "v1.3.0", BumpMinor, true},
		{"v1.2.3", "v2.0.0", BumpMajor, true},
		{"v0.7.0", "v0.8.0", BumpMajor, true}, // 0.x minor is breaking
		{"v0.7.0", "v0.7.1", BumpPatch, true},
		{"2.35.1", "2.36.0", BumpMinor, true},
		{"v1.10.0", "v1.9.9", 0, false}, // numeric, not lexical
		{"v1.2.3", "v1.2.3", 0, false},
		{"v1.2.3", "v1.2.3-rc1", 0, false},
		{"v1.2.3", "database/v1.2.4", 0, false},
	}
	for _, tt := range tests {
		b, ok := Classify(tt.cur, tt.next)
		if ok != tt.ok || (ok && b != tt.bump) {
			t.Errorf("Classify(%q,%q) = %v,%v; want %v,%v", tt.cur, tt.next, b, ok, tt.bump, tt.ok)
		}
	}
}

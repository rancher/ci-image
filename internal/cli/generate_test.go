package cli

import "testing"

func TestExtractGoVersion(t *testing.T) {
	tests := []struct{ base, want string }{
		{"registry.suse.com/bci/golang:1.26.10@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "1.26.10"},
		{"registry.suse.com/bci/golang:1.26.10", "1.26.10"},
		{"registry.suse.com/bci/python:3.13.15", ""},
		{"registry.suse.com/bci/bci-base:15.7", ""},
	}
	for _, tt := range tests {
		if got := extractGoVersion(tt.base); got != tt.want {
			t.Errorf("extractGoVersion(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

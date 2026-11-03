package clierr

import (
	"errors"
	"testing"
)

func TestDetail(t *testing.T) {
	cause := errors.New("boom")
	d := &Detail{
		Headline: "base image tag and digest disagree",
		Fields:   []Field{{"base", "r/x:1"}, {"pinned", "sha256:a"}, {"current", "sha256:b"}},
		Err:      cause,
	}
	want := "base image tag and digest disagree\n" +
		"    base:    r/x:1\n" +
		"    pinned:  sha256:a\n" +
		"    current: sha256:b"
	if got := d.Error(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if !errors.Is(d, cause) {
		t.Error("Detail must unwrap to its cause")
	}
}

func TestProblems(t *testing.T) {
	p := &Problems{Headline: "config validation failed"}
	if p.Err() != nil {
		t.Fatal("empty Problems must be a nil error")
	}
	p.Add("tool %q: bad", "a")
	if got, want := p.Error(), "config validation failed (1 problem)\n  tool \"a\": bad"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	p.Add("tool %q: worse", "b")
	if got, want := p.Error(), "config validation failed (2 problems)\n  tool \"a\": bad\n  tool \"b\": worse"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRender(t *testing.T) {
	if got := Render("error", errors.New("x")); got != "error: x" {
		t.Errorf("got %q", got)
	}
}

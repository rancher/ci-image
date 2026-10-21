package cli

import (
	"bufio"
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/updater"
)

func session(input string, prep func(updater.Candidate) (updater.Proposal, error)) (*manualSession, *bytes.Buffer) {
	var out bytes.Buffer
	return &manualSession{in: bufio.NewReader(strings.NewReader(input)), out: &out, prepare: prep}, &out
}

func prepOK(c updater.Candidate) (updater.Proposal, error) {
	return updater.Proposal{Tool: &updater.ToolEdit{Name: c.Name, Version: c.Latest}}, nil
}

func report() updater.Report {
	return updater.Report{Candidates: []updater.Candidate{
		{Kind: updater.KindTool, Name: "a", Current: "v1.0.0", Latest: "v1.0.1", Bump: updater.BumpPatch},
		{Kind: updater.KindTool, Name: "b", Current: "v1.0.0", Latest: "v2.0.0", Bump: updater.BumpMajor},
		{Kind: updater.KindTool, Name: "c", Current: "v1.0.0", Latest: "v1.1.0", Bump: updater.BumpMinor},
	}}
}

func names(es []updater.ToolEdit) []string {
	var n []string
	for _, e := range es {
		n = append(n, e.Name)
	}
	return n
}

func TestReview_AcceptSkipAndMajorNeedsYes(t *testing.T) {
	// a: y; b (major): "y" is rejected, then "n"; c: y
	s, _ := session("y\ny\nn\ny\n", prepOK)
	tools, _, quit := s.review(report())
	if quit || strings.Join(names(tools), ",") != "a,c" {
		t.Fatalf("staged %v quit=%v", names(tools), quit)
	}
}

func TestReview_MajorAcceptedOnlyByTypingYes(t *testing.T) {
	s, _ := session("n\nyes\nn\n", prepOK)
	tools, _, _ := s.review(report())
	if strings.Join(names(tools), ",") != "b" {
		t.Fatalf("staged %v", names(tools))
	}
}

func TestReview_QuitKeepsEarlierDecisionsOnly(t *testing.T) {
	s, _ := session("y\nq\n", prepOK)
	tools, _, quit := s.review(report())
	if !quit || strings.Join(names(tools), ",") != "a" {
		t.Fatalf("staged %v quit=%v", names(tools), quit)
	}
}

func TestReview_EOFQuits(t *testing.T) {
	s, _ := session("", prepOK)
	tools, _, quit := s.review(report())
	if !quit || len(tools) != 0 {
		t.Fatalf("staged %v quit=%v", names(tools), quit)
	}
}

func TestReview_PrepareErrorSkipsWithoutPrompting(t *testing.T) {
	prep := func(c updater.Candidate) (updater.Proposal, error) {
		if c.Name == "a" {
			return updater.Proposal{}, errors.New("boom")
		}
		return prepOK(c)
	}
	// Only two prompts are consumed (b, c).
	s, out := session("n\nn\n", prep)
	tools, _, _ := s.review(report())
	if len(tools) != 0 || !strings.Contains(out.String(), "boom") {
		t.Fatalf("staged %v\n%s", names(tools), out)
	}
}

func TestRunManualUpdates_RejectsAutomation(t *testing.T) {
	for _, arg := range []string{"--yes", "--all", "-y"} {
		if err := runManualUpdates([]string{arg}); err == nil || !strings.Contains(err.Error(), "interactive") {
			t.Errorf("%s: err = %v", arg, err)
		}
	}
	// Under `go test` stdin is not a terminal.
	if err := runManualUpdates(nil); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Errorf("non-tty err = %v", err)
	}
}

func prepFailsLocalOK(c updater.Candidate) (updater.Proposal, error) {
	return updater.Proposal{}, errors.New("fetching checksum file: unexpected status 404")
}

func localOK(c updater.Candidate) (updater.Proposal, error) {
	return updater.Proposal{
		Tool:      &updater.ToolEdit{Name: c.Name, Version: c.Latest},
		LocalHash: true,
		Sources:   []string{"https://example.test/" + c.Name},
	}, nil
}

func TestReview_LocalHashFallback_AcceptedIsLabelled(t *testing.T) {
	rep := report()
	rep.Candidates = rep.Candidates[:1]
	// y = download locally, y = stage
	s, out := session("y\ny\n", prepFailsLocalOK)
	s.prepareLocal = localOK
	tools, _, _ := s.review(rep)
	if strings.Join(names(tools), ",") != "a" {
		t.Fatalf("staged %v\n%s", names(tools), out)
	}
	if !strings.Contains(out.String(), "COMPUTED LOCALLY") || !strings.Contains(out.String(), "https://example.test/a") {
		t.Fatalf("unverified checksums must be flagged with their source:\n%s", out)
	}
}

func TestReview_LocalHashFallback_DeclinedSkips(t *testing.T) {
	rep := report()
	rep.Candidates = rep.Candidates[:1]
	called := false
	s, _ := session("n\n", prepFailsLocalOK)
	s.prepareLocal = func(c updater.Candidate) (updater.Proposal, error) { called = true; return localOK(c) }
	tools, _, _ := s.review(rep)
	if len(tools) != 0 || called {
		t.Fatalf("declined fallback must not download or stage (called=%v staged=%v)", called, names(tools))
	}
}

func TestReview_LocalHashFallback_NotOfferedForBases(t *testing.T) {
	rep := updater.Report{Candidates: []updater.Candidate{{Kind: updater.KindBase, Name: "b", Current: "1", Latest: "1", Refresh: true}}}
	s, out := session("", prepFailsLocalOK)
	s.prepareLocal = localOK
	s.review(rep)
	if strings.Contains(out.String(), "hash it here") {
		t.Fatalf("fallback offered for a base image:\n%s", out)
	}
}

func TestRunManualUpdates_AcceptsBaseImagesFlag(t *testing.T) {
	// Parsed as a known flag: the failure is the missing terminal, not the argument.
	err := runManualUpdates([]string{"--base-images"})
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveToolArgs(t *testing.T) {
	cfg, err := config.Load("../../deps.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if got, err := resolveToolArgs(cfg, nil); err != nil || got != nil {
		t.Fatalf("no args = %v, %v; want nil (all tools)", got, err)
	}

	got, err := resolveToolArgs(cfg, []string{"goreleaser", "oras", "goreleaser"})
	if err != nil || !slices.Equal(got, []string{"goreleaser", "oras"}) {
		t.Fatalf("got %v, %v", got, err)
	}

	// A family name expands to its members.
	got, err = resolveToolArgs(cfg, []string{"helm"})
	if err != nil || !slices.Equal(got, []string{"helmv3", "helmv4"}) {
		t.Fatalf("family helm = %v, %v", got, err)
	}

	// A typo is an error, not an empty selection.
	_, err = resolveToolArgs(cfg, []string{"goreleaser", "gorelaser"})
	if err == nil || !strings.Contains(err.Error(), "gorelaser") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunManualUpdates_ToolNamesAndBaseImagesAreExclusive(t *testing.T) {
	err := runManualUpdates([]string{"goreleaser", "--base-images"})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("err = %v", err)
	}
	// Names alone are accepted as arguments (failure is the missing terminal).
	if err := runManualUpdates([]string{"goreleaser", "oras"}); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("err = %v", err)
	}
	// Unknown flags are still rejected, not treated as tool names.
	if err := runManualUpdates([]string{"-y"}); err == nil || !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("err = %v", err)
	}
}

func TestReview_NamesToolsOutsideScope(t *testing.T) {
	rep := updater.Report{NotGitHub: []string{"helmv3", "kubectl1.33"}}
	s, out := session("", prepOK)
	s.review(rep)
	got := out.String()
	if !strings.Contains(got, "Not GitHub-sourced (out of scope, not checked): helmv3, kubectl1.33") {
		t.Fatalf("out-of-scope tools must be named on one line:\n%s", got)
	}
}

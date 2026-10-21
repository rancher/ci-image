package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/fileutil"
	"github.com/rancher/ci-image/internal/updater"
)

// runManualUpdates walks the user through newer versions of the tools and base
// images declared in deps.yaml, one at a time, and edits deps.yaml only for
// updates they explicitly accept.
//
// It is intentionally interactive-only: there is no --yes/--all flag and it
// refuses to run without a terminal, so a pipeline cannot rewrite pinned
// versions and checksums unattended.
func runManualUpdates(args []string) error {
	configPath := defaultConfig
	baseImages := false
	var toolArgs []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--base-images":
			baseImages = true
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case strings.HasPrefix(args[i], "-"):
			return fmt.Errorf("manual-updates is interactive and only accepts --config and --base-images; unknown argument %q", args[i])
		default:
			toolArgs = append(toolArgs, args[i])
		}
	}
	if baseImages && len(toolArgs) > 0 {
		return errors.New("--base-images cannot be combined with tool names: names select tools, --base-images checks every base image; run them separately")
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return errors.New("manual-updates is interactive: it needs a terminal on stdin and stdout and cannot be automated")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}
	cfg, err := config.Parse(data, configPath)
	if err != nil {
		return err
	}

	selected, err := resolveToolArgs(cfg, toolArgs)
	if err != nil {
		return err
	}

	s := &manualSession{
		in:      bufio.NewReader(os.Stdin),
		out:     os.Stdout,
		color:   os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb",
		cfg:     cfg,
		prepare: func(c updater.Candidate) (updater.Proposal, error) { return c.Prepare(cfg) },
		prepareLocal: func(c updater.Candidate) (updater.Proposal, error) {
			return c.PrepareLocalHash(cfg)
		},
	}

	switch {
	case selected != nil:
		s.printf("Checking %d selected tool(s) for updates: %s…\n", len(selected), strings.Join(selected, ", "))
	case baseImages:
		s.printf("Checking GitHub-sourced tools and the base images of %d images for updates…\n", len(cfg.Images))
	default:
		s.printf("Checking GitHub-sourced tools for updates (base images skipped; pass --base-images to include them)…\n")
	}
	// Set GITHUB_TOKEN to avoid the unauthenticated GitHub API rate limit.
	report := updater.Discover(context.Background(), cfg, updater.Options{BaseImages: baseImages, Tools: selected})

	tools, bases, _ := s.review(report)
	if len(tools)+len(bases) == 0 {
		s.printf("\nNo changes staged; deps.yaml untouched.\n")
		return nil
	}

	if !s.confirm(fmt.Sprintf("\nWrite %d change(s) to %s?", len(tools)+len(bases), configPath)) {
		s.printf("Nothing written.\n")
		return nil
	}
	newData, err := updater.Apply(data, tools, bases)
	if err != nil {
		return err
	}
	if err := updater.Verify(newData, tools, bases); err != nil {
		return fmt.Errorf("refusing to write: %w", err)
	}
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(configPath); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := fileutil.AtomicWrite(configPath, newData, perm); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}
	s.printf("%s updated.\n", configPath)

	if s.confirm("Run generate now to refresh Dockerfiles and images-lock.yaml?") {
		return runGenerate([]string{"--config", configPath})
	}
	s.printf("Remember to run 'make generate' before committing.\n")
	return nil
}

// resolveToolArgs turns command-line tool names into concrete tool names. A
// family name (e.g. kubectl) expands to its members. Returns nil when no names
// were given, meaning "all tools". Unknown names are an error so a typo can
// never silently narrow the review to nothing.
func resolveToolArgs(cfg *config.Config, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	names := []string{}
	var unknown []string
	for _, a := range args {
		tools := cfg.ResolveToolReference(a)
		if len(tools) == 0 {
			unknown = append(unknown, a)
			continue
		}
		for _, t := range tools {
			if !seen[t.Name] {
				seen[t.Name] = true
				names = append(names, t.Name)
			}
		}
	}
	if len(unknown) > 0 {
		var known []string
		for _, t := range cfg.Tools {
			known = append(known, t.Name)
		}
		return nil, fmt.Errorf("unknown tool(s) %s; known tools: %s", strings.Join(unknown, ", "), strings.Join(known, ", "))
	}
	return names, nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// manualSession holds the terminal I/O and the staged decisions.
type manualSession struct {
	in      *bufio.Reader
	out     io.Writer
	color   bool
	cfg     *config.Config
	prepare func(updater.Candidate) (updater.Proposal, error)
	// prepareLocal is the opt-in fallback that hashes our own download when no
	// upstream checksum can be fetched.
	prepareLocal func(updater.Candidate) (updater.Proposal, error)
}

func (s *manualSession) printf(format string, a ...any) { fmt.Fprintf(s.out, format, a...) }

func (s *manualSession) paint(code, text string) string {
	if !s.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s *manualSession) bold(t string) string   { return s.paint("1", t) }
func (s *manualSession) dim(t string) string    { return s.paint("2", t) }
func (s *manualSession) red(t string) string    { return s.paint("31", t) }
func (s *manualSession) green(t string) string  { return s.paint("32", t) }
func (s *manualSession) yellow(t string) string { return s.paint("33", t) }

// ask prints prompt and returns the trimmed, lower-cased answer. EOF (Ctrl-D)
// is reported as "q" so the review ends cleanly rather than looping.
func (s *manualSession) ask(prompt string) string {
	s.printf("%s ", prompt)
	line, err := s.in.ReadString('\n')
	if err != nil && line == "" {
		s.printf("\n")
		return "q"
	}
	return strings.ToLower(strings.TrimSpace(line))
}

func (s *manualSession) confirm(question string) bool {
	return s.ask(question+" [y/N]") == "y"
}

func (s *manualSession) bumpLabel(c updater.Candidate) string {
	switch {
	case c.Refresh:
		return s.dim("digest refresh")
	case c.Bump == updater.BumpMajor:
		return s.red(c.Bump.String())
	case c.Bump == updater.BumpMinor:
		return s.yellow(c.Bump.String())
	default:
		return s.green(c.Bump.String())
	}
}

// review shows the discovery summary then walks each candidate. It returns the
// accepted edits and whether the user quit early.
func (s *manualSession) review(rep updater.Report) (tools []updater.ToolEdit, bases []updater.BaseEdit, quit bool) {
	s.printf("\n%s\n", s.bold(fmt.Sprintf("%d update(s) available, %d up to date, %d skipped", len(rep.Candidates), len(rep.UpToDate), len(rep.Skipped))))
	for _, c := range rep.Candidates {
		label := "tool"
		if c.Kind == updater.KindBase {
			label = "base"
		}
		s.printf("  %-5s %-34s %s → %s  %s\n", label, c.Name, c.Current, c.Latest, s.bumpLabel(c))
	}
	if len(rep.Skipped) > 0 {
		s.printf("\n%s\n", s.dim("Skipped:"))
		for _, sk := range rep.Skipped {
			s.printf("  %s %s\n", s.dim(sk.Name+":"), sk.Reason)
		}
	}
	if len(rep.NotGitHub) > 0 {
		s.printf("\n%s %s\n", s.dim("Not GitHub-sourced (out of scope, not checked):"), strings.Join(rep.NotGitHub, ", "))
	}
	if len(rep.Info) > 0 {
		s.printf("\n%s\n", s.yellow("For your attention (never proposed automatically):"))
		for _, in := range rep.Info {
			s.printf("  • %s\n", in)
		}
	}

	for i, c := range rep.Candidates {
		s.printf("\n%s\n", s.bold(fmt.Sprintf("── [%d/%d] %s  %s → %s  (%s)", i+1, len(rep.Candidates), c.Name, c.Current, c.Latest, s.bumpLabel(c))))
		if c.ReleaseURL != "" {
			s.printf("   release notes: %s\n", c.ReleaseURL)
		}
		if c.Kind == updater.KindBase {
			s.printf("   used by: %s\n", strings.Join(c.Images, ", "))
		}

		s.printf("%s\n", s.dim("   fetching details…"))
		p, err := s.prepare(c)
		if err != nil {
			s.printf("   %s %v\n", s.red("error:"), err)
			if p, err = s.offerLocalHash(c); err != nil {
				s.printf("   skipped.\n")
				continue
			}
		}
		s.describe(c, p)

		switch s.decide(c) {
		case decisionAccept:
			if p.Tool != nil {
				tools = append(tools, *p.Tool)
			}
			if p.Base != nil {
				bases = append(bases, *p.Base)
			}
			s.printf("   %s\n", s.green("staged"))
		case decisionQuit:
			return tools, bases, true
		}
	}
	return tools, bases, false
}

// offerLocalHash is reached when published checksums could not be fetched. It
// only proceeds if the user explicitly agrees to trust our own download.
func (s *manualSession) offerLocalHash(c updater.Candidate) (updater.Proposal, error) {
	if c.Kind != updater.KindTool || s.prepareLocal == nil {
		return updater.Proposal{}, errors.New("no fallback")
	}
	s.printf("   %s\n", s.yellow("No published checksum could be fetched. The alternative is to download each artifact and hash it here."))
	s.printf("   %s\n", s.yellow("That only proves we got the same bytes we hashed; upstream does not vouch for them."))
	if s.ask("   Download and hash locally? [y/N]:") != "y" {
		return updater.Proposal{}, errors.New("declined")
	}
	s.printf("%s\n", s.dim("   downloading and hashing…"))
	p, err := s.prepareLocal(c)
	if err != nil {
		s.printf("   %s %v\n", s.red("error:"), err)
	}
	return p, err
}

func (s *manualSession) describe(c updater.Candidate, p updater.Proposal) {
	if p.Base != nil {
		s.printf("   %s %s\n   %s %s\n", s.red("-"), p.Base.Old, s.green("+"), p.Base.New)
		return
	}
	if p.Tool.VersionCommit != "" {
		s.printf("   version_commit → %s\n", p.Tool.VersionCommit)
	}
	if p.LocalHash {
		s.printf("   %s\n", s.red("checksums COMPUTED LOCALLY from our own download; not verified against any upstream-published checksum"))
		s.printf("   downloaded from:\n")
		for _, src := range p.Sources {
			s.printf("     %s\n", src)
		}
	} else if len(p.Sources) > 0 {
		s.printf("   checksums read from:\n")
		for _, src := range p.Sources {
			s.printf("     %s\n", src)
		}
	}
	for _, ch := range p.Changes {
		s.printf("   %s\n", ch.Platform)
		if ch.Old == ch.New {
			s.printf("     %s\n", s.yellow("unchanged "+ch.New))
			continue
		}
		s.printf("     %s %s\n     %s %s\n", s.red("-"), ch.Old, s.green("+"), ch.New)
	}
}

type decision int

const (
	decisionSkip decision = iota
	decisionAccept
	decisionQuit
)

func (s *manualSession) decide(c updater.Candidate) decision {
	for {
		if !c.Refresh && c.Bump == updater.BumpMajor {
			switch a := s.ask(s.red("   MAJOR bump. Type 'yes' to stage, [n]o to skip, [q]uit:")); a {
			case "yes":
				return decisionAccept
			case "n", "no":
				return decisionSkip
			case "q", "quit":
				return decisionQuit
			}
			continue
		}
		switch a := s.ask("   Stage this update? [y]es / [n]o / [q]uit:"); a {
		case "y", "yes":
			return decisionAccept
		case "n", "no":
			return decisionSkip
		case "q", "quit":
			return decisionQuit
		}
	}
}

package updater

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	ghrelease "github.com/mallardduck/ghreleases"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/registry"
	"github.com/rancher/ci-image/internal/resolver"
)

// Kind distinguishes what a Candidate updates.
type Kind int

const (
	KindTool Kind = iota
	KindBase
)

// Candidate is an available update. It carries no decision: nothing is
// written until the caller turns a Prepared proposal into an edit.
type Candidate struct {
	Kind       Kind
	Name       string   // tool name, or repo:tag of the base image
	Current    string   // current version / tag
	Latest     string   // proposed version / tag
	Bump       Bump     // how far Latest is from Current
	Refresh    bool     // base only: same tag, new digest
	Images     []string // base only: images that use this base
	ReleaseURL string   // upstream release notes, when known
	Notes      []string

	tool    *config.Tool
	ref     registry.Ref
	commit  string // go-install: commit for Latest
	newBase string // base: full new reference
}

// Skipped is a tool or base that could not be checked, with the reason.
type Skipped struct{ Name, Reason string }

// Report is the result of a discovery pass.
type Report struct {
	Candidates []Candidate
	UpToDate   []string
	Skipped    []Skipped
	// NotGitHub lists tools whose source is not a GitHub repository. They are
	// out of scope for this tool and are reported, not checked or modified.
	NotGitHub []string
	Info      []string // newer release lines that are never proposed automatically
}

var (
	newRegistry = registry.New

	// githubClient is the GitHub client used for release and commit lookups.
	// It is the resolver's shared client (GitHub hosts only, token scoped to
	// them); tests replace it.
	githubClient = resolver.GitHubClient
)

// upstream is the GitHub repository whose releases are searched for a tool.
type upstream struct {
	owner, repo, prefix string
}

// upstreamFor returns the GitHub release feed for a tool, following the
// tool's own tag_prefix rules. ok is false when the tool's source is not a
// GitHub repository: those tools are out of scope.
func upstreamFor(t *config.Tool) (up upstream, ok bool) {
	owner, repo, err := ghrelease.ParseSource(t.Source)
	if err != nil {
		return upstream{}, false
	}
	return upstream{owner: owner, repo: repo, prefix: t.TagPrefix()}, true
}

func (u upstream) latest(ctx context.Context) (string, error) {
	tag, err := githubClient().LatestRelease(ghrelease.WithTagPrefix(ctx, u.prefix), u.owner, u.repo)
	if err != nil {
		return "", err
	}
	if tag == "" {
		return "", fmt.Errorf("GitHub returned an empty tag for %s/%s", u.owner, u.repo)
	}
	return tag, nil
}

func (u upstream) releaseURL(tag string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", u.owner, u.repo, tag)
}

// Options selects what Discover checks.
type Options struct {
	// BaseImages also checks image base references against their registry.
	// Off by default: tools only.
	BaseImages bool
	// Tools restricts the tool check to these names. Nil checks every tool.
	Tools []string
}

// result is the outcome of checking one tool or base image.
type result struct {
	cand      *Candidate
	upToDate  string
	skip      *Skipped
	notGitHub string // tool name, when its source is not a GitHub repository
	info      []string
}

// Discover checks, in order, every GitHub-sourced pinned/static tool and, with
// opts.BaseImages, every image base, for updates. Tools whose source is not a
// GitHub repository are reported in Report.NotGitHub and otherwise ignored. It
// only reads from the network; it never modifies anything.
func Discover(ctx context.Context, cfg *config.Config, opts Options) Report {
	var jobs []func() result

	only := map[string]bool{}
	for _, n := range opts.Tools {
		only[n] = true
	}
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if opts.Tools != nil && !only[t.Name] {
			continue
		}
		jobs = append(jobs, func() result { return discoverTool(ctx, t) })
	}

	// Group images by identical base so one decision covers all of them.
	var baseOrder []string
	bases := map[string][]string{}
	for _, img := range cfg.Images {
		if _, ok := bases[img.Base]; !ok {
			baseOrder = append(baseOrder, img.Base)
		}
		bases[img.Base] = append(bases[img.Base], img.Name)
	}
	tagCache := &tagLists{lists: map[string]*tagList{}}
	for _, b := range baseOrder {
		if !opts.BaseImages {
			break
		}
		imgs := bases[b]
		jobs = append(jobs, func() result {
			c, up, sk, info := discoverBase(ctx, tagCache, b, imgs)
			return result{cand: c, upToDate: up, skip: sk, info: info}
		})
	}

	results := make([]result, len(jobs))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = job()
		}()
	}
	wg.Wait()

	var rep Report
	for _, r := range results {
		switch {
		case r.cand != nil:
			rep.Candidates = append(rep.Candidates, *r.cand)
		case r.skip != nil:
			rep.Skipped = append(rep.Skipped, *r.skip)
		case r.notGitHub != "":
			rep.NotGitHub = append(rep.NotGitHub, r.notGitHub)
		case r.upToDate != "":
			rep.UpToDate = append(rep.UpToDate, r.upToDate)
		}
		rep.Info = append(rep.Info, r.info...)
	}
	return rep
}

func discoverTool(ctx context.Context, t *config.Tool) (r result) {
	skip := func(format string, a ...any) {
		r.skip = &Skipped{Name: t.Name, Reason: fmt.Sprintf(format, a...)}
	}
	if t.EffectiveMode() == config.ModeReleaseChecksums {
		skip("tracked in deps.lock; use 'update'")
		return
	}
	up, ok := upstreamFor(t)
	if !ok {
		r.notGitHub = t.Name
		return
	}
	tag, err := up.latest(ctx)
	if err != nil {
		skip("looking up latest release: %v", err)
		return
	}
	if tag == t.Version {
		r.upToDate = t.Name
		return
	}
	tagStripped, tagHas := strings.CutPrefix(tag, up.prefix)
	toolVersionStripped, verHas := strings.CutPrefix(t.Version, up.prefix)
	if tagHas != verHas {
		skip("latest tag %q has a different version format than %q", tag, t.Version)
		return
	}
	bump, ok := Classify(toolVersionStripped, tagStripped)
	if !ok {
		if cmp, ok2 := compareOrder(tagStripped, toolVersionStripped); ok2 && cmp < 0 {
			r.upToDate = t.Name // pinned ahead of the feed's latest
			return
		}
		skip("cannot compare %q with latest %q", t.Version, tag)
		return
	}
	c := &Candidate{
		Kind: KindTool, Name: t.Name, Current: t.Version, Latest: tag, Bump: bump,
		ReleaseURL: up.releaseURL(tag), tool: t,
	}
	if t.VersionCommit != "" {
		if c.commit, err = githubClient().ResolveTagCommit(ctx, up.owner, up.repo, tag); err != nil {
			skip("resolving commit for %s: %v", tag, err)
			return
		}
	}
	r.cand = c
	return
}

func compareOrder(a, b string) (int, bool) {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	return compareNums(x, y), true
}

// ---- base images ----------------------------------------------------------

type tagList struct {
	once sync.Once
	tags []string
	err  error
}

type tagLists struct {
	mu    sync.Mutex
	lists map[string]*tagList
}

func (c *tagLists) get(ctx context.Context, cl *registry.Client, host, repo string) ([]string, error) {
	key := host + "/" + repo
	c.mu.Lock()
	l, ok := c.lists[key]
	if !ok {
		l = &tagList{}
		c.lists[key] = l
	}
	c.mu.Unlock()
	l.once.Do(func() { l.tags, l.err = cl.Tags(ctx, repo) })
	return l.tags, l.err
}

// chooseTag picks the tag to move a base image to. Tags are parsed as
// VERSION[-VARIANT][-REVISION] (see registry.ParseTag).
//
//   - Tags with three or more version components (1.27.2) advance to the newest
//     version within the same leading components (1.27.x). Within a version the
//     newest build revision wins, so 1.26.9-dev-34.1 can move to
//     1.26.10-dev-36.3 or 24.18.1-base-13.1 to 24.18.1-base-13.2.
//   - Shorter or unparseable tags (24, 15.7, latest) are floating pins: the tag
//     stays and only the digest is refreshed.
//   - The variant must match, and a tag with a revision only moves to tags that
//     also have one (and the reverse), so a different flavour is never crossed
//     into.
//
// newer lists tags of the same flavour beyond that line, reported but never
// proposed — moving to a new Go/Node/OS line is a deliberate, manual change.
func chooseTag(cur string, tags []string) (chosen string, newer string) {
	curTag, ok := registry.ParseTag(cur)
	if !ok {
		return cur, ""
	}
	nums := curTag.Version
	best, bestTag := cur, curTag
	over, overTag := "", curTag
	for _, t := range tags {
		tag, ok := registry.ParseTag(t)
		if !ok || !curTag.SameFlavour(tag) || len(tag.Version) != len(nums) {
			continue
		}
		// Only fully-patched tags (3+ components) advance within their line.
		sameLine := len(nums) >= 3 && slices.Equal(tag.Version[:len(nums)-1], nums[:len(nums)-1])
		switch {
		case sameLine && tag.Compare(bestTag) > 0:
			best, bestTag = t, tag
		case !sameLine && tag.Compare(overTag) > 0:
			over, overTag = t, tag
		}
	}
	return best, over
}

func discoverBase(ctx context.Context, cache *tagLists, base string, images []string) (c *Candidate, upToDate string, sk *Skipped, info []string) {
	skip := func(format string, a ...any) *Skipped {
		return &Skipped{Name: "base " + base, Reason: fmt.Sprintf(format, a...)}
	}
	ref, err := registry.ParseRef(base)
	if err != nil {
		return nil, "", skip("%v", err), nil
	}
	if ref.Tag == "" {
		return nil, "", skip("no tag to track"), nil
	}
	cl, err := newRegistry(ref.Registry)
	if err != nil {
		return nil, "", skip("%v", err), nil
	}

	tags, err := cache.get(ctx, cl, ref.Registry, ref.Repo)
	if err != nil {
		return nil, "", skip("listing tags: %v", err), nil
	}
	chosen, newer := chooseTag(ref.Tag, tags)
	short := ref.Repo + ":" + ref.Tag
	if newer != "" {
		info = append(info, fmt.Sprintf("%s: newer line %s:%s exists (not proposed; change the tag manually)", short, ref.Repo, newer))
	}

	digest, err := cl.Digest(ctx, ref.Repo, chosen)
	if err != nil {
		return nil, "", skip("resolving digest of %s: %v", chosen, err), info
	}
	if chosen == ref.Tag && digest == ref.Digest {
		return nil, short, nil, info
	}

	next := ref
	next.Tag, next.Digest = chosen, digest
	cand := &Candidate{
		Kind: KindBase, Name: short, Current: ref.Tag, Latest: chosen, Images: images,
		Refresh: chosen == ref.Tag, ref: ref, newBase: next.String(),
	}
	if !cand.Refresh {
		cand.Bump = classifyTags(ref.Tag, chosen)
	}
	return cand, "", nil, info
}

// ---- preparing a reviewable proposal ---------------------------------------

// ChecksumChange is one platform's checksum before and after.
type ChecksumChange struct {
	Platform, Old, New string
}

// Proposal is everything a reviewer needs to see to accept a Candidate, plus
// the edit that would be applied.
type Proposal struct {
	Tool    *ToolEdit
	Base    *BaseEdit
	Changes []ChecksumChange
	Sources []string // where the new checksums were read from (or, if LocalHash, downloaded from)
	// LocalHash marks checksums computed by hashing our own download because
	// upstream publishes none. They are not independently verified.
	LocalHash bool
}

// ErrNoPublishedChecksum is returned (wrapped) by Prepare only when upstream
// answered 404 for the checksum file, i.e. it publishes none at that location.
// Other failures (5xx, malformed file, disallowed host, template errors) are
// not this condition: a published checksum may exist, so callers must not fall
// back to locally computed hashes for them.
var ErrNoPublishedChecksum = errors.New("no published checksum file")

// Prepare gathers what the reviewer must see before deciding. For tools this
// downloads the new checksums using the tool's own release configuration.
// Nothing is written.
func (c *Candidate) Prepare(cfg *config.Config) (Proposal, error) {
	if c.Kind == KindBase {
		return Proposal{Base: &BaseEdit{Old: c.ref.String(), New: c.newBase}}, nil
	}
	edit := &ToolEdit{Name: c.Name, Version: c.Latest, VersionCommit: c.commit}
	p := Proposal{Tool: edit}
	if c.tool.Install.EffectiveMethod() != config.MethodCurl {
		return p, nil
	}
	sums, err := resolver.FetchToolChecksums(cfg, c.tool, c.Latest)
	if err != nil {
		var statusErr *ghrelease.HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return p, fmt.Errorf("fetching checksums: %w: %w", ErrNoPublishedChecksum, err)
		}
		return p, fmt.Errorf("fetching checksums: %w", err)
	}
	p.Sources, err = resolver.ChecksumLocation(cfg, c.tool, c.Latest)
	if err != nil {
		return p, err
	}
	c.fillChecksums(&p, sums)
	return p, nil
}

// PrepareLocalHash is the fallback when Prepare cannot fetch published
// checksums: it downloads each artifact and hashes it. The returned Proposal
// has LocalHash set so the UI can say the checksums are unverified.
func (c *Candidate) PrepareLocalHash(cfg *config.Config) (Proposal, error) {
	edit := &ToolEdit{Name: c.Name, Version: c.Latest, VersionCommit: c.commit}
	p := Proposal{Tool: edit, LocalHash: true}
	if c.Kind != KindTool || c.tool.Install.EffectiveMethod() != config.MethodCurl {
		return p, fmt.Errorf("local hashing only applies to curl-installed tools")
	}
	sums, urls, err := resolver.HashDownloads(cfg, c.tool, c.Latest)
	if err != nil {
		return p, fmt.Errorf("hashing downloads: %w", err)
	}
	p.Sources = urls
	c.fillChecksums(&p, sums)
	return p, nil
}

func (c *Candidate) fillChecksums(p *Proposal, sums map[string]string) {
	p.Tool.Checksums = sums
	for _, plat := range slices.Sorted(maps.Keys(sums)) {
		p.Changes = append(p.Changes, ChecksumChange{Platform: plat, Old: c.tool.Checksums[plat], New: sums[plat]})
	}
}

// classifyTags sizes a base tag move from its version alone; a revision-only
// rebuild (same version) is a patch.
func classifyTags(cur, next string) Bump {
	c, ok1 := registry.ParseTag(cur)
	n, ok2 := registry.ParseTag(next)
	if !ok1 || !ok2 {
		return BumpPatch
	}
	b, ok := Classify(dotted(c.Version), dotted(n.Version))
	if !ok {
		return BumpPatch
	}
	return b
}

func dotted(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

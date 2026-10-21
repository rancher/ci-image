package ghrelease

import (
	"context"
	"strings"
	"time"
)

type releaseFilterKey struct{}

// ReleaseFilter configures how LatestRelease selects a release in repositories
// that publish multiple release lines under different tag prefixes (e.g. "v*"
// for a library and "cli/v*" for a CLI tool, following Go multi-module
// conventions).
type ReleaseFilter struct {
	// TagPrefix restricts results to releases whose tag starts with this string.
	// An empty prefix disables filtering and preserves the default behavior.
	TagPrefix string
}

// WithTagPrefix returns a context that instructs LatestRelease to scan all
// releases and return the most recently published one whose tag starts with
// prefix. When prefix is non-empty, LatestRelease uses the paginated
// /releases endpoint instead of /releases/latest, because GitHub's
// /releases/latest does not support prefix filtering.
//
// Example — find the latest CLI release in a multi-prefix repo:
//
//	ctx := ghrelease.WithTagPrefix(context.Background(), "cli/")
//	tag, err := client.LatestRelease(ctx, "rancher", "ob-charts-tool")
func WithTagPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, releaseFilterKey{}, ReleaseFilter{TagPrefix: prefix})
}

func releaseFilterFromContext(ctx context.Context) (ReleaseFilter, bool) {
	f, ok := ctx.Value(releaseFilterKey{}).(ReleaseFilter)
	return f, ok
}

// ExcludePrereleases returns releases that are not marked as prerelease.
func ExcludePrereleases(releases []*Release) []*Release {
	return filterReleases(releases, func(r *Release) bool { return !r.Prerelease })
}

// ExcludeDrafts returns releases that are not drafts.
func ExcludeDrafts(releases []*Release) []*Release {
	return filterReleases(releases, func(r *Release) bool { return !r.Draft })
}

// FilterByDateRange returns releases whose PublishedAt falls within [from, to] inclusive.
// A zero value for from or to means no lower or upper bound respectively.
func FilterByDateRange(releases []*Release, from, to time.Time) []*Release {
	return filterReleases(releases, func(r *Release) bool {
		if r.PublishedAt.IsZero() {
			return false
		}
		if !from.IsZero() && r.PublishedAt.Before(from) {
			return false
		}
		if !to.IsZero() && r.PublishedAt.After(to) {
			return false
		}
		return true
	})
}

// FilterByTagPrefix returns releases whose TagName starts with prefix.
// An empty prefix returns all releases unchanged.
func FilterByTagPrefix(releases []*Release, prefix string) []*Release {
	if prefix == "" {
		return releases
	}
	return filterReleases(releases, func(r *Release) bool {
		return strings.HasPrefix(r.TagName, prefix)
	})
}

// LatestPerGroup returns the most recently published release per group key.
// groupFn assigns each release a group key; releases with an empty key are skipped.
// Result order matches the first appearance of each group key in the input.
func LatestPerGroup(releases []*Release, groupFn func(*Release) string) []*Release {
	best := make(map[string]*Release)
	var order []string

	for _, r := range releases {
		key := groupFn(r)
		if key == "" {
			continue
		}
		existing, ok := best[key]
		if !ok {
			best[key] = r
			order = append(order, key)
		} else if r.PublishedAt.After(existing.PublishedAt) {
			best[key] = r
		}
	}

	result := make([]*Release, 0, len(order))
	for _, key := range order {
		result = append(result, best[key])
	}
	return result
}

// GroupByMajorVersion is a groupFn for LatestPerGroup that groups releases by their
// major semver version (e.g. "v1.2.3" and "v1.4.0" both map to "v1").
// Tags that don't follow semver are grouped under their full tag name.
func GroupByMajorVersion(r *Release) string {
	return semverPrefix(r.TagName, 1)
}

// GroupByMinorVersion is a groupFn for LatestPerGroup that groups releases by their
// major.minor semver version (e.g. "v1.2.3" and "v1.2.9" both map to "v1.2").
// Tags that don't follow semver are grouped under their full tag name.
func GroupByMinorVersion(r *Release) string {
	return semverPrefix(r.TagName, 2)
}

func filterReleases(releases []*Release, keep func(*Release) bool) []*Release {
	out := make([]*Release, 0, len(releases))
	for _, r := range releases {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// semverPrefix extracts the first n dot-separated numeric components from a semver-like tag.
// Any monorepo path prefix (e.g. "sub-pkg/" in "sub-pkg/v1.2.3") is stripped before
// parsing and re-attached to the result, so the group key remains unique per component.
// Returns the full original tag if the semver portion doesn't look like semver.
func semverPrefix(fullTag string, n int) string {
	pathPrefix := ""
	s := fullTag
	if i := strings.LastIndex(s, "/"); i >= 0 {
		pathPrefix = s[:i+1]
		s = s[i+1:]
	}

	vPrefix := ""
	if len(s) > 0 && s[0] == 'v' {
		vPrefix = "v"
		s = s[1:]
	}

	parts := strings.SplitN(s, ".", n+1)
	if len(parts) < n {
		return fullTag
	}

	for i := 0; i < n; i++ {
		if !isDigits(parts[i]) {
			return fullTag
		}
	}

	return pathPrefix + vPrefix + strings.Join(parts[:n], ".")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

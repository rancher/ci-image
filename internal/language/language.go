// Package language identifies the programming language and full version an
// image ships from its digest-pinned base reference, and derives the rolling
// tag alias for it.
//
// Everything here is derived from the base reference, never hand-written, so
// an alias cannot drift from the version the image is actually built on.
package language

import (
	"context"
	"errors"
	"fmt"

	"github.com/rancher/ci-image/internal/clierr"
	"github.com/rancher/ci-image/internal/registry"
)

// repos maps allowlisted BCI base repositories to the language they ship.
var repos = map[string]string{
	"registry.suse.com/bci/golang": "go",
	"registry.suse.com/bci/python": "python",
	"registry.suse.com/bci/nodejs": "node",

	// Application Collection; the -dev-<os> tags include zypper.
	"dp.apps.rancher.io/containers/go":     "go",
	"dp.apps.rancher.io/containers/python": "python",
	"dp.apps.rancher.io/containers/node":   "node",
}

// Info describes what a base reference ships.
type Info struct {
	// Language is "go", "python" or "node"; empty when the base is not a
	// known language image.
	Language string
	// Version is the full X.Y.Z of the base tag; empty unless the tag has one.
	Version string
	// AliasOK reports whether a rolling tag alias can be derived safely.
	AliasOK bool
	// Reason explains why AliasOK is false, for the generator warning.
	Reason string
}

// Detect identifies the language and full version of a base reference.
func Detect(base string) Info {
	ref, err := registry.ParseRef(base)
	if err != nil {
		return Info{Reason: err.Error()}
	}
	lang, ok := repos[ref.Registry+"/"+ref.Repo]
	if !ok {
		return Info{Reason: fmt.Sprintf("%s/%s is not a known language base image", ref.Registry, ref.Repo)}
	}
	if ref.Tag == "" {
		return Info{Language: lang, Reason: "base has no tag"}
	}
	// The variant and build revision (24.18.1-base-13.2, 1.26.10-dev-36.3) are
	// not part of the version; the alias is the bare X.Y.Z.
	tag, ok := registry.ParseTag(ref.Tag)
	if !ok || len(tag.Version) != 3 {
		return Info{Language: lang, Reason: fmt.Sprintf("base tag %q is not a full X.Y.Z version with an optional variant and build revision", ref.Tag)}
	}
	return Info{Language: lang, Version: fmt.Sprintf("%d.%d.%d", tag.Version[0], tag.Version[1], tag.Version[2]), AliasOK: true}
}

// Alias returns the rolling tag for a base reference: the bare full version
// (e.g. "1.26.10"). The image repository already identifies the language.
// ok is false when no safe alias can be derived.
func Alias(base string) (alias string, info Info) {
	info = Detect(base)
	if !info.AliasOK {
		return "", info
	}
	return info.Version, info
}

// DigestResolver resolves repo:tag to a manifest digest. *registry.Client
// satisfies it.
type DigestResolver interface {
	Digest(ctx context.Context, repo, tag string) (string, error)
}

// ErrDigestDrift is matched (via errors.Is) by the error VerifyDigest returns
// when a base's tag resolves to a different digest than the one it is pinned
// to. The pinned digest still builds reproducibly; it only means the base has
// a newer build to consider, so callers may treat it as a warning.
var ErrDigestDrift = errors.New("tag and digest disagree")

// VerifyDigest checks that the tag of a pinned base reference still resolves
// to the digest it is pinned to. Docker ignores the tag when a digest is
// present, so without this a hand edit could leave the two disagreeing and the
// alias would name a version the image is not built on.
func VerifyDigest(ctx context.Context, r DigestResolver, base string) error {
	ref, err := registry.ParseRef(base)
	if err != nil {
		return err
	}
	if ref.Tag == "" || ref.Digest == "" {
		return &clierr.Detail{
			Headline: "base must carry both a tag and a digest",
			Fields:   []clierr.Field{{Key: "base", Value: base}},
		}
	}
	got, err := r.Digest(ctx, ref.Repo, ref.Tag)
	if err != nil {
		return clierr.WithFields(err, clierr.Field{Key: "base", Value: ref.Registry + "/" + ref.Repo + ":" + ref.Tag})
	}
	if got != ref.Digest {
		return &clierr.Detail{
			Headline: "base image tag and digest disagree",
			Fields: []clierr.Field{
				{Key: "base", Value: ref.Registry + "/" + ref.Repo + ":" + ref.Tag},
				{Key: "pinned", Value: ref.Digest},
				{Key: "current", Value: got},
			},
			Err: ErrDigestDrift,
		}
	}
	return nil
}

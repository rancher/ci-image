// Package language identifies the programming language and full version an
// image ships from its digest-pinned base reference, and derives the rolling
// tag alias for it.
//
// Everything here is derived from the base reference, never hand-written, so
// an alias cannot drift from the version the image is actually built on.
package language

import "context"

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
	return Info{}
}

// Alias returns the rolling tag for a base reference: the bare full version
// (e.g. "1.26.10"). The image repository already identifies the language.
// The alias is empty when no safe alias can be derived.
func Alias(base string) (alias string, info Info) {
	return "", Info{}
}

// DigestResolver resolves repo:tag to a manifest digest. *registry.Client
// satisfies it.
type DigestResolver interface {
	Digest(ctx context.Context, repo, tag string) (string, error)
}

// VerifyDigest checks that the tag of a pinned base reference still resolves
// to the digest it is pinned to. Docker ignores the tag when a digest is
// present, so without this a hand edit could leave the two disagreeing and the
// alias would name a version the image is not built on.
func VerifyDigest(ctx context.Context, r DigestResolver, base string) error {
	return nil
}

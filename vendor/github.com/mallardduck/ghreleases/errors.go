// Package ghrelease provides primitives for fetching and extracting GitHub release assets.
package ghrelease

import (
	"errors"
	"fmt"
)

// Package errors
var (
	ErrInvalidSource     = errors.New("invalid GitHub source format")
	ErrReleaseNotFound   = errors.New("release not found")
	ErrRefNotFound       = errors.New("ref not found")
	ErrAssetNotFound     = errors.New("asset not found")
	ErrChecksumMismatch  = errors.New("checksum verification failed")
	ErrUnknownVariable   = errors.New("unknown template variable")
	ErrUnknownModifier   = errors.New("unknown template modifier")
	ErrInvalidModifier   = errors.New("invalid modifier syntax")
	ErrMultipleFiles     = errors.New("archive contains multiple files, ExtractPath required")
	ErrFileNotFound      = errors.New("file not found in archive")
	ErrUnsupportedFormat = errors.New("unsupported archive format")
	ErrChecksumFormat    = errors.New("invalid checksum file format")
	ErrHostNotAllowed    = errors.New("host not allowed")
)

// HTTPStatusError is returned when a request completes with an unexpected HTTP
// status code. Use errors.As to inspect it; it may be wrapped alongside a
// sentinel such as ErrReleaseNotFound.
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status code: %d (%s)", e.StatusCode, e.URL)
}

// wrapped attaches a sentinel to a cause so that errors.Is matches the sentinel
// and errors.As/Is can still reach the cause (Go 1.18 allows only one %w).
type wrapped struct {
	sentinel, cause error
}

func (w *wrapped) Error() string { return w.sentinel.Error() + ": " + w.cause.Error() }

func (w *wrapped) Unwrap() error { return w.cause }

func (w *wrapped) Is(target error) bool { return target == w.sentinel }

package ghrelease

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

// DownloadOptions configures asset download behavior.
type DownloadOptions struct {
	// If set, validates downloaded content against this hash (SHA-256 hex, case-insensitive)
	ExpectedHash string

	// Context for cancellation (optional)
	Context context.Context
}

// DownloadResult contains the outcome of a download operation.
type DownloadResult struct {
	Hash string // Computed SHA-256 hex string
	Size int64  // Bytes downloaded
}

// Download fetches an asset and writes it to w while computing SHA-256.
// Always computes hash (returned in result).
// If opts.ExpectedHash is set, validates before returning.
func (c *Client) Download(url string, w io.Writer, opts DownloadOptions) (*DownloadResult, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// No timeout is applied here: the body is bounded only by ctx so large
	// artifacts are not cut off mid-stream.
	req, err := c.newRequest(ctx, url, "application/octet-stream")
	if err != nil {
		return nil, err
	}

	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
	}

	// Stream to writer while computing hash
	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)

	size, err := io.Copy(w, tee)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))

	// Validate if expected hash provided
	if opts.ExpectedHash != "" {
		if err := ValidateHash(computedHash, opts.ExpectedHash); err != nil {
			return nil, err
		}
	}

	return &DownloadResult{
		Hash: computedHash,
		Size: size,
	}, nil
}

// DownloadToBytes is a convenience wrapper that downloads to a byte slice.
func (c *Client) DownloadToBytes(url string, opts DownloadOptions) ([]byte, *DownloadResult, error) {
	var buf bytes.Buffer
	result, err := c.Download(url, &buf, opts)
	if err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), result, nil
}

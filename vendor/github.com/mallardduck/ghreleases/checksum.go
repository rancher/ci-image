package ghrelease

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ParseChecksumFile parses GNU coreutils style checksum files.
// Format: <hash>  <filename>
// Supports both single and double space separators.
// Ignores blank lines and comments (lines starting with #).
func ParseChecksumFile(r io.Reader) (map[string]string, error) {
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on whitespace (handles both single and double space)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid checksum format at line %d: expected '<hash> <filename>'", lineNum)
		}

		hash := parts[0]
		filename := parts[1]

		// Validate hash is hex (SHA-256 = 64 chars)
		if len(hash) != 64 {
			return nil, fmt.Errorf("invalid hash length at line %d: expected 64 chars, got %d", lineNum, len(hash))
		}

		// Validate hex characters
		for _, c := range hash {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return nil, fmt.Errorf("invalid hash at line %d: non-hex character %c", lineNum, c)
			}
		}

		checksums[filename] = strings.ToLower(hash)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading checksum file: %w", err)
	}

	return checksums, nil
}

// ValidateHash compares a computed hash against an expected hash.
// Both hashes are normalized to lowercase for comparison.
func ValidateHash(computed, expected string) error {
	computed = strings.ToLower(strings.TrimSpace(computed))
	expected = strings.ToLower(strings.TrimSpace(expected))

	if computed != expected {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, computed)
	}

	return nil
}

// isHex64 reports whether s is a 64-character hex digest.
func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// ParseChecksumFileLenient parses checksum files tolerantly. Unlike
// ParseChecksumFile it never fails on malformed content:
//   - blank lines, "#" comments, and lines whose first field is not a 64-char
//     hex digest are skipped silently;
//   - the hash is lower-cased;
//   - the filename is the last field, with a leading "*" (binary-mode marker)
//     stripped;
//   - a line holding only a hash stores the digest under the key "" (used by
//     single-entry .sha256sum files).
//
// The only error returned is a read error from r.
func ParseChecksumFileLenient(r io.Reader) (map[string]string, error) {
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if !isHex64(fields[0]) {
			continue
		}
		name := ""
		if len(fields) > 1 {
			name = strings.TrimPrefix(fields[len(fields)-1], "*")
		}
		checksums[name] = strings.ToLower(fields[0])
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading checksum file: %w", err)
	}
	return checksums, nil
}

// sha256Column is the exact column name selected from multi-hash checksum files.
const sha256Column = "SHA-256"

// ParseHashOrder parses a hash-order file (e.g. mikefarah/yq's
// checksums_hashes_order): one algorithm name per line, blank lines skipped.
func ParseHashOrder(r io.Reader) ([]string, error) {
	var order []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if name := strings.TrimSpace(scanner.Text()); name != "" {
			order = append(order, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading hash order file: %w", err)
	}
	return order, nil
}

// ParseMultiHashChecksumFile parses checksum files of the form
//
//	<filename>  <hash> <hash> ...
//
// where order (see ParseHashOrder) names each hash column. It returns the
// "SHA-256" column keyed by filename. The column is selected by its exact name,
// never by value shape, because many other algorithms (SHA3-256, BLAKE2s,
// GOST12-256, ...) are also 64 hex characters.
//
// Errors wrap ErrChecksumFormat when "SHA-256" is absent from order or appears
// more than once, when a line's column count differs from len(order), or when
// the selected value is not 64 hex characters. Blank lines and "#" comments are
// skipped.
func ParseMultiHashChecksumFile(r io.Reader, order []string) (map[string]string, error) {
	col := -1
	for i, name := range order {
		if name != sha256Column {
			continue
		}
		if col >= 0 {
			return nil, fmt.Errorf("%w: column %s appears more than once", ErrChecksumFormat, sha256Column)
		}
		col = i
	}
	if col < 0 {
		return nil, fmt.Errorf("%w: column %s not found in hash order", ErrChecksumFormat, sha256Column)
	}

	checksums := make(map[string]string)
	scanner := bufio.NewScanner(r)
	// Lines carry ~30 hashes (several KiB); allow well beyond the default 64 KiB token limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields)-1 != len(order) {
			return nil, fmt.Errorf("%w: line %d has %d hash columns, expected %d",
				ErrChecksumFormat, lineNum, len(fields)-1, len(order))
		}
		hash := fields[col+1]
		if !isHex64(hash) {
			return nil, fmt.Errorf("%w: line %d: %s value is not 64 hex characters", ErrChecksumFormat, lineNum, sha256Column)
		}
		checksums[fields[0]] = strings.ToLower(hash)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading checksum file: %w", err)
	}
	return checksums, nil
}

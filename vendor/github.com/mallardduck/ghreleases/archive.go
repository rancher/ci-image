package ghrelease

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ArchiveFormat represents supported archive types.
type ArchiveFormat string

// Archive format constants
const (
	FormatTarGz ArchiveFormat = "tar.gz"
	FormatTgz   ArchiveFormat = "tgz"
	FormatZip   ArchiveFormat = "zip"
	FormatGzip  ArchiveFormat = "gz"
	FormatPlain ArchiveFormat = "plain" // no archive
)

// ExtractOptions configures archive extraction behavior.
type ExtractOptions struct {
	// Format override (auto-detect from filename if empty)
	Format ArchiveFormat

	// Specific file path within archive (empty = auto-select single file)
	ExtractPath string
}

// Extract extracts content from an archive.
// Auto-detects format from filename unless opts.Format specified.
// If opts.ExtractPath set, extracts specific file.
// If archive contains single file and no ExtractPath, auto-extracts it.
func Extract(data []byte, filename string, opts ExtractOptions) ([]byte, error) {
	format := opts.Format
	if format == "" {
		format = detectFormat(filename)
	}

	switch format {
	case FormatTarGz, FormatTgz:
		return extractTarGz(data, opts.ExtractPath)
	case FormatZip:
		return extractZip(data, opts.ExtractPath)
	case FormatGzip:
		return extractGzip(data)
	case FormatPlain:
		return data, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

func detectFormat(filename string) ArchiveFormat {
	lower := strings.ToLower(filename)

	if strings.HasSuffix(lower, ".tar.gz") {
		return FormatTarGz
	}
	if strings.HasSuffix(lower, ".tgz") {
		return FormatTgz
	}
	if strings.HasSuffix(lower, ".zip") {
		return FormatZip
	}
	if strings.HasSuffix(lower, ".gz") {
		return FormatGzip
	}

	return FormatPlain
}

func extractTarGz(data []byte, extractPath string) ([]byte, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	var files []archiveFile
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// If specific path requested, check for match
		if extractPath != "" {
			if header.Name == extractPath || filepath.Base(header.Name) == extractPath {
				content, err := io.ReadAll(tarReader)
				if err != nil {
					return nil, fmt.Errorf("failed to read file: %w", err)
				}
				return content, nil
			}
			continue
		}

		// Collect all files for auto-selection
		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		files = append(files, archiveFile{name: header.Name, content: content})
	}

	// If specific path requested but not found
	if extractPath != "" {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, extractPath)
	}

	// Auto-select if single file
	if len(files) == 1 {
		return files[0].content, nil
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in archive")
	}

	return nil, fmt.Errorf("%w: found %d files", ErrMultipleFiles, len(files))
}

func extractZip(data []byte, extractPath string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	var files []archiveFile
	for _, file := range reader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// If specific path requested, check for match
		if extractPath != "" {
			if file.Name == extractPath || filepath.Base(file.Name) == extractPath {
				rc, err := file.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open file: %w", err)
				}
				defer func() { _ = rc.Close() }()

				content, err := io.ReadAll(rc)
				if err != nil {
					return nil, fmt.Errorf("failed to read file: %w", err)
				}
				return content, nil
			}
			continue
		}

		// Collect all files for auto-selection
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		files = append(files, archiveFile{name: file.Name, content: content})
	}

	// Same auto-selection logic as tar.gz
	if extractPath != "" {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, extractPath)
	}

	if len(files) == 1 {
		return files[0].content, nil
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in archive")
	}

	return nil, fmt.Errorf("%w: found %d files", ErrMultipleFiles, len(files))
}

func extractGzip(data []byte) ([]byte, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	return io.ReadAll(gzReader)
}

type archiveFile struct {
	name    string
	content []byte
}

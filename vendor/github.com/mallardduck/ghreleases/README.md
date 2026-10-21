# ghreleases

A zero-dependency Go package for fetching and extracting GitHub release assets.

## Features

- **GitHub API Integration** - Fetch latest releases and assets
- **Template Rendering** - Dynamic asset name generation with variables and modifiers
- **Checksum Validation** - Parse and verify SHA-256 checksums
- **Archive Extraction** - Extract from .tar.gz, .tgz, .zip, and .gz archives
- **Streaming Downloads** - Efficient downloads with built-in hash computation
- **Zero Dependencies** - Standard library only for maximum portability

## Installation

```bash
go get github.com/mallardduck/ghreleases
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/mallardduck/ghreleases"
)

func main() {
    // Create client (uses GITHUB_TOKEN env var if available)
    client := ghrelease.NewClient("")
    
    // Get latest release
    tag, _ := client.LatestRelease(context.Background(), "owner", "repo")
    fmt.Println("Latest release:", tag)
    
    // Get release details
    release, _ := client.GetRelease(context.Background(), "owner", "repo", tag)
    for _, asset := range release.Assets {
        fmt.Println("Asset:", asset.Name)
    }
}
```

## Core Concepts

The package provides **composable primitives** that work independently or together:

- **Client** - GitHub API interactions (releases, assets, checksums)
- **Template** - Dynamic string rendering with variables and modifiers
- **Checksum** - Parse and validate SHA-256 checksums
- **Archive** - Extract files from archives
- **Download** - Streaming downloads with hash computation

## API Reference

### Client

#### Creating Clients

```go
// Use GITHUB_TOKEN environment variable
client := ghrelease.NewClient("")

// Use explicit token
client := ghrelease.NewClient("ghp_yourtoken")

// Custom HTTP client (for proxies, timeouts, etc.)
httpClient := &http.Client{Timeout: 60 * time.Second}
client := ghrelease.NewClientWithHTTP(httpClient, "")

// Options (both constructors accept them)
client := ghrelease.NewClient("",
    ghrelease.WithBaseURL("https://ghe.example.com/api/v3"),
    ghrelease.WithAllowedHosts("github.com", "example.com"),
)
```

#### Host Policy and Timeouts

- **Token scoping (always on):** the `Authorization: Bearer` header is sent only to the
  API base host, `github.com`, `*.github.com`, `githubusercontent.com` and
  `*.githubusercontent.com` (dot-boundary match, so `evilgithub.com` never receives it).
  It is also removed on redirects to any other host.
- **`WithAllowedHosts(hosts...)`:** when set, requests and redirects to other hosts fail
  with `ErrHostNotAllowed`. An entry matches the host itself or any subdomain
  (`github.com` allows `api.github.com`). The base URL host is always allowed.
  The default is unrestricted.
- **Timeouts:** the default HTTP client has dial, TLS and response-header timeouts but no
  total timeout. API and checksum calls get a 30s timeout when the context has no deadline.
  `Download` is bounded only by its context, so large artifacts are not cut off mid-body.

#### Fetching Releases

```go
ctx := context.Background()

// Get latest release tag
tag, err := client.LatestRelease(ctx, "owner", "repo")

// Get specific release with assets
release, err := client.GetRelease(ctx, "owner", "repo", "v1.0.0")
for _, asset := range release.Assets {
    fmt.Printf("%s - %d bytes\n", asset.Name, asset.Size)
}
```

#### Fetching Checksums

```go
// Download and parse checksums file
checksums, err := client.FetchChecksums(ctx, checksumURL)
expectedHash := checksums["myapp-linux-amd64"]

// Lenient: skips junk lines, strips "*" binary markers, and stores a bare
// hash under the key "" (single-entry .sha256sum files)
checksums, err := client.FetchChecksumsLenient(ctx, checksumURL)

// Multi-hash files (e.g. mikefarah/yq): "<filename> <hash> <hash> ..." plus a
// sibling file naming the columns. The exact "SHA-256" column is selected.
checksums, err := client.FetchMultiHashChecksums(ctx, checksumsURL, hashesOrderURL)
```

### Template Rendering

Dynamically generate asset names using variables and modifiers.

#### Variables

- `{name}` - Binary/application name
- `{version}` - Release version/tag
- `{os}` - Operating system (linux, darwin, windows)
- `{arch}` - Architecture (amd64, arm64, etc)
- `{ext}` - File extension (tar.gz, zip, etc)
- Any key in `TemplateVars.Extra` (`map[string]string`), e.g. `{version_commit}`.
  Built-ins win over `Extra`; a key with an empty value is found and renders as `""`.

In `TemplateStrict` mode `Render` returns the partially rendered string together with the
error; always check the error before using the result. A modifier that requires an
argument (`trimprefix`, `trimsuffix`, `replace`) is an error without one.

#### Modifiers

Modifiers are applied with pipe separators and chained left-to-right:

- `upper` - Convert to uppercase
- `lower` - Convert to lowercase
- `title` - Capitalize first letter
- `trimprefix:PREFIX` - Remove prefix
- `trimsuffix:SUFFIX` - Remove suffix
- `replace:FROM=TO` - Replace exact value (not substring)

#### Examples

```go
vars := ghrelease.TemplateVars{
    Name:    "myapp",
    Version: "v1.0.0",
    OS:      "linux",
    Arch:    "amd64",
    Ext:     "tar.gz",
}

// Basic template
name, _ := ghrelease.Render(
    "{name}_{version}_{os}_{arch}.{ext}",
    vars,
    ghrelease.TemplatePermissive,
)
// Result: myapp_v1.0.0_linux_amd64.tar.gz

// With modifiers
name, _ := ghrelease.Render(
    "{name|upper}-{version|trimprefix:v}.tar.gz",
    vars,
    ghrelease.TemplatePermissive,
)
// Result: MYAPP-1.0.0.tar.gz

// Replace OS name (exact match)
name, _ := ghrelease.Render(
    "{name}-{os|replace:darwin=macos}-{arch}",
    TemplateVars{Name: "myapp", OS: "darwin", Arch: "amd64"},
    ghrelease.TemplatePermissive,
)
// Result: myapp-macos-amd64

// Chained modifiers
name, _ := ghrelease.Render(
    "{os|replace:darwin=macos|upper}",
    TemplateVars{OS: "darwin"},
    ghrelease.TemplatePermissive,
)
// Result: MACOS
```

#### Template Modes

- **TemplatePermissive** - Unknown variables/modifiers pass through unchanged (for flexible matching)
- **TemplateStrict** - Unknown variables/modifiers return error (for validation)

### Downloading

Download assets with automatic hash computation and validation.

```go
// Download to writer
var buf bytes.Buffer
result, err := client.Download(assetURL, &buf, ghrelease.DownloadOptions{})
fmt.Println("SHA-256:", result.Hash)
fmt.Println("Size:", result.Size)

// Download with checksum validation
result, err := client.Download(assetURL, &buf, ghrelease.DownloadOptions{
    ExpectedHash: "abc123...",  // SHA-256 hex
})
// Returns ErrChecksumMismatch if hash doesn't match

// Download to byte slice (convenience wrapper)
data, result, err := client.DownloadToBytes(assetURL, ghrelease.DownloadOptions{})

// With context for cancellation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := client.Download(assetURL, &buf, ghrelease.DownloadOptions{
    Context: ctx,
})
```

### Archive Extraction

Extract files from various archive formats.

```go
// Auto-detect format from filename
binary, err := ghrelease.Extract(archiveData, "myapp.tar.gz", ghrelease.ExtractOptions{})

// Extract specific file from multi-file archive
binary, err := ghrelease.Extract(archiveData, "release.tar.gz", ghrelease.ExtractOptions{
    ExtractPath: "bin/myapp",  // Full path or just basename
})

// Override format detection
binary, err := ghrelease.Extract(data, "archive", ghrelease.ExtractOptions{
    Format: ghrelease.FormatTarGz,
})
```

#### Supported Formats

- `.tar.gz` / `.tgz` - Gzip-compressed tar archive
- `.zip` - ZIP archive
- `.gz` - Gzip-compressed file
- Plain files (no extraction)

#### Extraction Behavior

- **Single file archive + no ExtractPath**: Automatically extracts the file
- **Multiple files + no ExtractPath**: Returns `ErrMultipleFiles`
- **ExtractPath specified**: Extracts specific file (matches full path or basename)
- **Directories**: Automatically skipped

### Checksum Validation

Parse and validate GNU coreutils style checksum files.

```go
// Parse checksum file
file, _ := os.Open("checksums.txt")
checksums, err := ghrelease.ParseChecksumFile(file)

hash := checksums["myapp-linux-amd64"]

// Validate hash (case-insensitive)
err := ghrelease.ValidateHash(computedHash, expectedHash)
if errors.Is(err, ghrelease.ErrChecksumMismatch) {
    // Handle mismatch
}
```

#### Checksum File Format

```
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  file1.txt
abc0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  file2.txt

# Comments and blank lines are ignored
def0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  file3.txt
```

Lenient and multi-hash parsing are also available directly:
`ParseChecksumFileLenient(r)`, `ParseHashOrder(r)` and
`ParseMultiHashChecksumFile(r, order)`. The multi-hash parser never infers the column from
value shape (SHA3-256, BLAKE2s, etc. are also 64 hex); it returns `ErrChecksumFormat` if
`SHA-256` is missing or duplicated, a line's column count differs from the order file, or
the value is not 64 hex.

### Source Parsing

Parse GitHub repository identifiers.

```go
owner, repo, err := ghrelease.ParseSource("owner/repo")
owner, repo, err := ghrelease.ParseSource("https://github.com/owner/repo")
owner, repo, err := ghrelease.ParseSource("https://github.com/owner/repo.git")
owner, repo, err := ghrelease.ParseSource("git@github.com:owner/repo.git")
```

## Complete Examples

### Download Latest Release Binary

```go
package main

import (
    "context"
    "fmt"
    "os"
    "runtime"
    
    "github.com/mallardduck/ghreleases"
)

func main() {
    client := ghrelease.NewClient("")
    ctx := context.Background()
    
    // Get latest release
    tag, err := client.LatestRelease(ctx, "owner", "repo")
    if err != nil {
        panic(err)
    }
    
    release, err := client.GetRelease(ctx, "owner", "repo", tag)
    if err != nil {
        panic(err)
    }
    
    // Generate asset name for current platform
    vars := ghrelease.TemplateVars{
        Name:    "myapp",
        Version: tag,
        OS:      runtime.GOOS,
        Arch:    runtime.GOARCH,
        Ext:     "tar.gz",
    }
    
    assetName, err := ghrelease.Render(
        "{name}_{version}_{os}_{arch}.{ext}",
        vars,
        ghrelease.TemplatePermissive,
    )
    if err != nil {
        panic(err)
    }
    
    // Find matching asset
    var assetURL string
    for _, asset := range release.Assets {
        if asset.Name == assetName {
            assetURL = asset.URL
            break
        }
    }
    
    if assetURL == "" {
        panic("asset not found")
    }
    
    // Download and extract
    data, result, err := client.DownloadToBytes(assetURL, ghrelease.DownloadOptions{})
    if err != nil {
        panic(err)
    }
    
    fmt.Println("Downloaded:", result.Size, "bytes")
    fmt.Println("SHA-256:", result.Hash)
    
    binary, err := ghrelease.Extract(data, assetName, ghrelease.ExtractOptions{
        ExtractPath: "myapp",
    })
    if err != nil {
        panic(err)
    }
    
    // Write binary to disk
    os.WriteFile("myapp", binary, 0755)
    fmt.Println("Binary extracted successfully")
}
```

### Download with Checksum Verification

```go
package main

import (
    "context"
    "os"
    
    "github.com/mallardduck/ghreleases"
)

func main() {
    client := ghrelease.NewClient("")
    ctx := context.Background()
    
    // Fetch checksums file
    checksumURL := "https://github.com/owner/repo/releases/download/v1.0.0/checksums.txt"
    checksums, err := client.FetchChecksums(ctx, checksumURL)
    if err != nil {
        panic(err)
    }
    
    // Get expected hash for our asset
    assetName := "myapp-linux-amd64.tar.gz"
    expectedHash := checksums[assetName]
    
    // Download with verification
    assetURL := "https://github.com/owner/repo/releases/download/v1.0.0/" + assetName
    data, result, err := client.DownloadToBytes(assetURL, ghrelease.DownloadOptions{
        ExpectedHash: expectedHash,
    })
    if err != nil {
        panic(err) // Will error if checksum mismatch
    }
    
    // Extract
    binary, err := ghrelease.Extract(data, assetName, ghrelease.ExtractOptions{})
    if err != nil {
        panic(err)
    }
    
    os.WriteFile("myapp", binary, 0755)
}
```

## Error Handling

The package provides sentinel errors for common cases. Use `errors.Is()` for checking:

```go
import "errors"

_, err := ghrelease.ParseSource("invalid")
if errors.Is(err, ghrelease.ErrInvalidSource) {
    // Handle invalid source format
}

_, err = client.LatestRelease(ctx, "owner", "nonexistent")
if errors.Is(err, ghrelease.ErrReleaseNotFound) {
    // Handle release not found
}

_, err = ghrelease.Extract(data, "multi-file.tar.gz", ghrelease.ExtractOptions{})
if errors.Is(err, ghrelease.ErrMultipleFiles) {
    // Specify ExtractPath
}

err = ghrelease.ValidateHash(computed, expected)
if errors.Is(err, ghrelease.ErrChecksumMismatch) {
    // Handle checksum failure
}
```

### Typed Errors

HTTP failures return `*HTTPStatusError` (with `URL` and `StatusCode`). Where a sentinel
also applies, both match:

```go
var se *ghrelease.HTTPStatusError
if errors.Is(err, ghrelease.ErrReleaseNotFound) && errors.As(err, &se) {
    fmt.Println(se.StatusCode)
}
```

### Available Errors

- `ErrInvalidSource` - Invalid GitHub source format
- `ErrReleaseNotFound` - Release not found
- `ErrRefNotFound` - Git ref not found
- `ErrAssetNotFound` - Asset not found
- `ErrChecksumMismatch` - Checksum verification failed
- `ErrUnknownVariable` - Unknown template variable (strict mode)
- `ErrUnknownModifier` - Unknown template modifier (strict mode)
- `ErrInvalidModifier` - Invalid modifier syntax
- `ErrMultipleFiles` - Archive has multiple files, ExtractPath required
- `ErrFileNotFound` - File not found in archive
- `ErrUnsupportedFormat` - Unsupported archive format
- `ErrChecksumFormat` - Malformed multi-hash checksum file
- `ErrHostNotAllowed` - Request or redirect blocked by `WithAllowedHosts`

## Testing

The package includes comprehensive tests using `httptest` for mocking GitHub API:

```bash
go test ./...
go test -cover ./...
```

## License

MIT - See LICENSE file for details.

## Contributing

This package is designed to be stable and dependency-free. Contributions should maintain:

- Zero external dependencies
- Standard library only
- Comprehensive test coverage
- Clear error messages
- Simple, focused APIs

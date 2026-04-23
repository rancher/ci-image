package dockerfile

import "strings"

// urlExt strips the query string from url before extension checks.
func urlExt(url string) string {
	if i := strings.IndexByte(url, '?'); i >= 0 {
		return url[:i]
	}
	return url
}

// archiveExt returns the archive extension of url (e.g. ".tar.gz", ".zip"),
// or "" if not a recognised archive format.
func archiveExt(url string) string {
	u := urlExt(url)
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".zip"} {
		if strings.HasSuffix(u, ext) {
			return ext
		}
	}
	return ""
}

// archiveExtractCmd returns the shell command to extract a temp file for the
// given archive extension. The caller must substitute ${TMP_FILE} and ${TMP_DIR}.
func archiveExtractCmd(ext string) string {
	switch ext {
	case ".tar.gz", ".tgz":
		return "tar xzf \"${TMP_FILE}\" -C \"${TMP_DIR}\""
	case ".tar.bz2":
		return "tar xjf \"${TMP_FILE}\" -C \"${TMP_DIR}\""
	case ".tar.xz":
		return "tar xJf \"${TMP_FILE}\" -C \"${TMP_DIR}\""
	case ".zip":
		return "unzip -q \"${TMP_FILE}\" -d \"${TMP_DIR}\""
	default:
		return "tar xzf \"${TMP_FILE}\" -C \"${TMP_DIR}\"" // fallback
	}
}

// isGzipBinaryURL reports whether url is a plain gzip-compressed binary
// (ends in .gz but not .tar.gz — a single file, not an archive).
func isGzipBinaryURL(url string) bool {
	u := urlExt(url)
	return strings.HasSuffix(u, ".gz") && !strings.HasSuffix(u, ".tar.gz")
}

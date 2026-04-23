package config

// allowlist is the compile-time allowlist for release-checksums mode.
// Entries are org/repo shorthand matching the tool's source field.
//
// Adding a new entry requires a code change, making it visible in PR review.
// A config-only PR cannot introduce a new trusted source.
var allowlist = []string{
	"rancher/charts-build-scripts",
	"rancher/ob-charts-tool",
}

func inAllowlist(source string) bool {
	for _, s := range allowlist {
		if s == source {
			return true
		}
	}
	return false
}

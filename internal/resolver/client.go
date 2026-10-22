package resolver

import (
	"sync"

	ghrelease "github.com/mallardduck/ghreleases"
)

// githubHosts is the set of hosts the resolver may contact. The API host is
// covered by github.com; release assets redirect to *.githubusercontent.com.
// Anything else (including redirects) fails with ghrelease.ErrHostNotAllowed.
var githubHosts = []string{"github.com", "githubusercontent.com"}

var defaultClient = sync.OnceValue(func() *ghrelease.Client {
	// The token comes from GITHUB_TOKEN and is only ever sent to GitHub hosts.
	return ghrelease.NewClient("", ghrelease.WithAllowedHosts(githubHosts...))
})

// ghClient returns the client used for every GitHub request. It is a variable
// so tests can substitute a client pointed at a local server.
var ghClient = defaultClient

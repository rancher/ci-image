package registry

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// credHelperFunc runs the credential helper docker-credential-<name> for host.
type credHelperFunc func(name, host string) (user, pass string, err error)

// authFile is the subset of docker's config.json and containers-auth.json(5)
// that holds registry credentials.
type authFile struct {
	Auths map[string]struct {
		Auth     string `json:"auth"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auths"`
	CredHelpers map[string]string `json:"credHelpers"`
	CredsStore  string            `json:"credsStore"`
}

// helperNameRe keeps a helper name from becoming a path: it is joined onto
// "docker-credential-" and looked up on PATH.
var helperNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// storedCredentials looks host up in the auth files and credential helpers
// that docker, podman, skopeo and buildah write on login, so a prior
// `docker login` is enough and no environment variables are needed. Only host
// itself is ever looked up; other registries' credentials are never read out.
func storedCredentials(host string, env func(string) string, helper credHelperFunc) (user, pass string, found bool) {
	for _, path := range authFilePaths(env) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f authFile
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if u, p, ok := f.lookup(host, helper); ok {
			return u, p, true
		}
	}
	return "", "", false
}

// authFilePaths lists the candidate files in the order the tools consult them.
func authFilePaths(env func(string) string) []string {
	var paths []string
	if p := env("REGISTRY_AUTH_FILE"); p != "" {
		paths = append(paths, p)
	}
	home := env("HOME")
	if d := env("DOCKER_CONFIG"); d != "" {
		paths = append(paths, filepath.Join(d, "config.json"))
	} else if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "config.json"))
	}
	if d := env("XDG_RUNTIME_DIR"); d != "" {
		paths = append(paths, filepath.Join(d, "containers", "auth.json"))
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "containers", "auth.json"))
	}
	return paths
}

// lookup resolves host the way docker does: a per-registry credential helper,
// then an inline auths entry, then the default credsStore helper.
func (f authFile) lookup(host string, helper credHelperFunc) (user, pass string, found bool) {
	if name := f.CredHelpers[host]; name != "" {
		if u, p, ok := runHelper(helper, name, host); ok {
			return u, p, true
		}
	}
	for key, a := range f.Auths {
		if hostOfKey(key) != host {
			continue
		}
		if a.Auth != "" {
			raw, err := base64.StdEncoding.DecodeString(a.Auth)
			if u, p, ok := strings.Cut(string(raw), ":"); err == nil && ok && u != "" {
				return u, p, true
			}
		}
		if a.Username != "" && a.Password != "" {
			return a.Username, a.Password, true
		}
	}
	if f.CredsStore != "" {
		return runHelper(helper, f.CredsStore, host)
	}
	return "", "", false
}

// hostOfKey strips the scheme and path docker allows in an auths key.
func hostOfKey(key string) string {
	key = strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	host, _, _ := strings.Cut(key, "/")
	return host
}

// runHelper treats any helper failure, an unsafe name, or an identity token
// (user "<token>", which is not a basic-auth password) as "no credentials".
func runHelper(helper credHelperFunc, name, host string) (user, pass string, ok bool) {
	if !helperNameRe.MatchString(name) {
		return "", "", false
	}
	user, pass, err := helper(name, host)
	if err != nil || user == "" || user == "<token>" || pass == "" {
		return "", "", false
	}
	return user, pass, true
}

// execCredHelper runs docker-credential-<name> get for host.
func execCredHelper(name, host string) (user, pass string, err error) {
	cmd := exec.Command("docker-credential-"+name, "get")
	cmd.Stdin = strings.NewReader(host)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return "", "", err
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return "", "", fmt.Errorf("docker-credential-%s timed out", name)
	}
	var creds struct{ Username, Secret string }
	if err := json.Unmarshal(out.Bytes(), &creds); err != nil {
		return "", "", fmt.Errorf("docker-credential-%s: %w", name, err)
	}
	return creds.Username, creds.Secret, nil
}

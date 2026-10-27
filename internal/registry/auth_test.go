package registry

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const appco = "dp.apps.rancher.io"

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func noHelper(name, host string) (string, string, error) {
	return "", "", errors.New("helper must not run")
}

func TestStoredCredentials_DockerConfigAuths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"`+appco+`":{"auth":"`+basicAuth("svc", "s3cret")+`"}}}`)
	u, p, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper)
	if !ok || u != "svc" || p != "s3cret" {
		t.Errorf("got (%q, %q, %v), want (svc, s3cret, true)", u, p, ok)
	}
}

func TestStoredCredentials_UsernamePasswordFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"`+appco+`":{"username":"svc","password":"s3cret"}}}`)
	u, p, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper)
	if !ok || u != "svc" || p != "s3cret" {
		t.Errorf("got (%q, %q, %v), want (svc, s3cret, true)", u, p, ok)
	}
}

func TestStoredCredentials_SchemeAndPathInKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"https://`+appco+`/v1/":{"auth":"`+basicAuth("svc", "s3cret")+`"}}}`)
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper); !ok {
		t.Error("key with scheme and path should match the host")
	}
}

func TestStoredCredentials_PasswordMayContainColon(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"`+appco+`":{"auth":"`+basicAuth("svc", "a:b")+`"}}}`)
	_, p, _ := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper)
	if p != "a:b" {
		t.Errorf("password = %q, want a:b", p)
	}
}

func TestStoredCredentials_HomeDockerConfig(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".docker", "config.json"),
		`{"auths":{"`+appco+`":{"auth":"`+basicAuth("svc", "s3cret")+`"}}}`)
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"HOME": home}), noHelper); !ok {
		t.Error("~/.docker/config.json was not consulted")
	}
}

func TestStoredCredentials_PodmanLocations(t *testing.T) {
	auth := `{"auths":{"` + appco + `":{"auth":"` + basicAuth("svc", "s3cret") + `"}}}`
	tmp := t.TempDir()
	cases := map[string]struct {
		env  map[string]string
		path string
	}{
		"REGISTRY_AUTH_FILE": {map[string]string{"REGISTRY_AUTH_FILE": filepath.Join(tmp, "a", "auth.json")}, filepath.Join(tmp, "a", "auth.json")},
		"XDG_RUNTIME_DIR":    {map[string]string{"XDG_RUNTIME_DIR": filepath.Join(tmp, "b")}, filepath.Join(tmp, "b", "containers", "auth.json")},
		"XDG config":         {map[string]string{"HOME": filepath.Join(tmp, "c")}, filepath.Join(tmp, "c", ".config", "containers", "auth.json")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			writeFile(t, tc.path, auth)
			if _, _, ok := storedCredentials(appco, envOf(tc.env), noHelper); !ok {
				t.Errorf("%s was not consulted", tc.path)
			}
		})
	}
}

func TestStoredCredentials_CredsStoreHelper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"credsStore":"osxkeychain"}`)
	var gotName, gotHost string
	helper := func(name, host string) (string, string, error) {
		gotName, gotHost = name, host
		return "svc", "s3cret", nil
	}
	u, p, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), helper)
	if !ok || u != "svc" || p != "s3cret" {
		t.Errorf("got (%q, %q, %v), want (svc, s3cret, true)", u, p, ok)
	}
	if gotName != "osxkeychain" || gotHost != appco {
		t.Errorf("helper called with (%q, %q), want (osxkeychain, %s)", gotName, gotHost, appco)
	}
}

func TestStoredCredentials_CredHelpersBeatCredsStore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"credsStore":"other","credHelpers":{"`+appco+`":"pass"}}`)
	var gotName string
	helper := func(name, host string) (string, string, error) {
		gotName = name
		return "svc", "s3cret", nil
	}
	storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), helper)
	if gotName != "pass" {
		t.Errorf("helper = %q, want the per-registry credHelper (pass)", gotName)
	}
}

func TestStoredCredentials_IdentityTokenHelperSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"credsStore":"osxkeychain"}`)
	helper := func(name, host string) (string, string, error) { return "<token>", "abc", nil }
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), helper); ok {
		t.Error("an identity token is not a basic-auth password and must be ignored")
	}
}

func TestStoredCredentials_HelperFailureFallsThrough(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"credsStore":"osxkeychain","auths":{"`+appco+`":{"auth":"`+basicAuth("svc", "s3cret")+`"}}}`)
	helper := func(name, host string) (string, string, error) { return "", "", errors.New("not found") }
	if u, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), helper); !ok || u != "svc" {
		t.Errorf("got (%q, %v), want the auths entry (svc, true)", u, ok)
	}
}

func TestStoredCredentials_UnsafeHelperNameRefused(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"credsStore":"../../bin/sh"}`)
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper); ok {
		t.Error("a helper name with path separators must be refused")
	}
}

func TestStoredCredentials_OtherHostIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"ghcr.io":{"auth":"`+basicAuth("someone", "else")+`"}}}`)
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper); ok {
		t.Error("credentials for another registry were returned")
	}
}

func TestStoredCredentials_MissingOrMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{not json`)
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"DOCKER_CONFIG": dir}), noHelper); ok {
		t.Error("malformed config reported credentials")
	}
	if _, _, ok := storedCredentials(appco, envOf(map[string]string{"HOME": t.TempDir()}), noHelper); ok {
		t.Error("absent config reported credentials")
	}
}

func TestNew_AppCoFallsBackToStoredCredentials(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"`+appco+`":{"auth":"`+basicAuth("svc", "s3cret")+`"}}}`)
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("APPCO_USERNAME", "")
	t.Setenv("APPCO_PASSWORD", "")
	c, err := New(appco)
	if err != nil {
		t.Fatal(err)
	}
	if c.Username != "svc" || c.Password != "s3cret" {
		t.Errorf("got (%q, %q), want the docker login credentials", c.Username, c.Password)
	}
}

func TestNew_EnvCredentialsBeatStored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"),
		`{"auths":{"`+appco+`":{"auth":"`+basicAuth("stored", "stored")+`"}}}`)
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("APPCO_USERNAME", "env")
	t.Setenv("APPCO_PASSWORD", "envpass")
	c, err := New(appco)
	if err != nil {
		t.Fatal(err)
	}
	if c.Username != "env" || c.Password != "envpass" {
		t.Errorf("got (%q, %q), want the environment credentials", c.Username, c.Password)
	}
}

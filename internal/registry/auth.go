package registry

// credHelperFunc runs the credential helper docker-credential-<name> for host.
type credHelperFunc func(name, host string) (user, pass string, err error)

// storedCredentials looks host up in the auth files and credential helpers
// that docker, podman, skopeo and buildah write on login, so a prior
// `docker login` is enough and no environment variables are needed.
func storedCredentials(host string, env func(string) string, helper credHelperFunc) (user, pass string, found bool) {
	return "", "", false
}

// execCredHelper runs docker-credential-<name> get for host.
func execCredHelper(name, host string) (user, pass string, err error) {
	return "", "", nil
}

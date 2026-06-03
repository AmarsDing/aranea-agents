package auth

import (
	"fmt"
	"os"
	"strings"
)

// authSecretFromEnv retrieves the JWT signing secret from the environment variable.
// EP-SEC-01: If the secret is not configured outside of dev/test/CI contexts, the
// process panics at startup to prevent using a predictable timestamp-derived key.
func authSecretFromEnv(key string) string {
	// Allow --version to work without any env vars.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			return "version-placeholder"
		}
	}
	if secret := os.Getenv(key); secret != "" {
		return secret
	}
	if isGoTestBinary() {
		return "test-placeholder-secret-not-for-production"
	}
	// Auth bypass skips JWT validation entirely; the signing key is unused.
	bypass := strings.TrimSpace(strings.ToLower(os.Getenv("KRATOS_HTTP_AUTH_DISABLED")))
	if bypass == "1" || bypass == "true" || bypass == "yes" {
		return "dev-bypass-placeholder"
	}
	// Allow test and CI environments to run without a secret (key is not used for request auth).
	deployEnv := strings.TrimSpace(strings.ToLower(os.Getenv("DEPLOY_ENV")))
	ci := strings.TrimSpace(os.Getenv("CI"))
	if deployEnv == "dev" || deployEnv == "development" || deployEnv == "test" ||
		ci == "true" || ci == "1" {
		return "test-placeholder-secret-not-for-production"
	}
	panic(fmt.Sprintf(
		"[FATAL] %s is not set. "+
			"Set it to a random string of at least 32 characters. "+
			"For local development only, set KRATOS_HTTP_AUTH_DISABLED=1 (with DEPLOY_ENV=dev) "+
			"to skip JWT validation.",
		key,
	))
}

func isGoTestBinary() bool {
	name := strings.ToLower(os.Args[0])
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe")
}

// cookieNameFromEnv retrieves the cookie name from the environment variable.
func cookieNameFromEnv(key string) string {
	if name := os.Getenv(key); name != "" {
		return name
	}
	return "access_token"
}

func ParseTokenFromRequest(tokenStr string) (*Auth, error) {
	return ParseToken(tokenStr, authSecretKey)
}

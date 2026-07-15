package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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
	// Dev/test mode: use a persistent file-based secret so JWT tokens survive
	// backend restarts and auth-bypass toggles. Eliminates "token signature
	// invalid" failures caused by secret rotating between runs.
	if isDevOrTestEnv() {
		return persistentDevSecret(mustDevSecretPath())
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

// isDevOrTestEnv returns true when running in dev/test/CI environments.
// Matches the condition in HTTPAuthBypassEnabled for consistency:
// bypass is only allowed when DEPLOY_ENV is dev/development/test or CI=true.
func isDevOrTestEnv() bool {
	deployEnv := strings.TrimSpace(strings.ToLower(os.Getenv("DEPLOY_ENV")))
	ci := strings.TrimSpace(os.Getenv("CI"))
	return deployEnv == "dev" || deployEnv == "development" || deployEnv == "test" ||
		ci == "true" || ci == "1"
}

// persistentDevSecret returns a stable JWT signing secret for dev/test.
// The secret is generated once (32 random bytes, base64-encoded) and
// persisted to secretPath with 0600 permissions. Subsequent calls load
// the existing secret, so backend restarts no longer invalidate JWTs.
func persistentDevSecret(secretPath string) string {
	if data, err := os.ReadFile(secretPath); err == nil {
		if s := strings.TrimSpace(string(data)); len(s) >= 32 {
			return s
		}
	}
	secret := generateRandomSecret(32)
	_ = os.MkdirAll(filepath.Dir(secretPath), 0o700)
	_ = os.WriteFile(secretPath, []byte(secret), 0o600)
	return secret
}

// mustDevSecretPath returns the persistent dev secret file path.
// Uses os.UserConfigDir()/aranea/ (consistent with internal/cli/config/paths.go).
// Falls back to a local relative path if the user config dir is unavailable.
func mustDevSecretPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return filepath.Join(".aranea", "dev-jwt-secret")
	}
	return filepath.Join(dir, "aranea", "dev-jwt-secret")
}

// generateRandomSecret returns a base64-encoded random secret of n bytes.
func generateRandomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback (should not happen in practice): deterministic but sufficient length.
		return base64.StdEncoding.EncodeToString([]byte(
			"aranea-dev-fallback-secret-not-for-production-min32chars",
		))
	}
	return base64.StdEncoding.EncodeToString(b)
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

// SetSecretForTesting overrides the package-level JWT signing secret
// (authSecretKey) used by Middleware and ParseTokenFromRequest. It returns
// the previous secret so callers can restore it via defer. Intended ONLY for
// use in tests that exercise the production auth path end-to-end and need
// generated tokens to pass verification. Production code must NEVER call this.
func SetSecretForTesting(newSecret string) string {
	prev := authSecretKey
	authSecretKey = newSecret
	return prev
}

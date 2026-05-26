package artifact

import (
	"errors"
	"testing"
	"time"
)

func TestDownloadTokenRoundTrip(t *testing.T) {
	// Ensure the dev key fallback is available for this round-trip test.
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")
	t.Setenv("DEPLOY_ENV", "dev")
	t.Setenv("KRATOS_ENV", "")
	t.Setenv("APP_ENV", "")

	id := "art-123"
	version := 2
	expires := time.Now().UTC().Add(5 * time.Minute)
	token, err := DownloadToken(id, version, expires)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := VerifyDownloadToken(id, version, expires.Unix(), token)
	if err != nil || !ok {
		t.Fatalf("expected valid token: ok=%v err=%v", ok, err)
	}
	bad, err := VerifyDownloadToken(id, version, expires.Unix(), "bad")
	if err != nil || bad {
		t.Fatalf("expected invalid token rejected: ok=%v err=%v", bad, err)
	}
	expired, err := VerifyDownloadToken(id, version, time.Now().Add(-time.Minute).Unix(), token)
	if err != nil || expired {
		t.Fatalf("expected expired token rejected: ok=%v err=%v", expired, err)
	}
}

// OUT-05 / ART-02: in production, missing signing key MUST fail closed.
func TestSignKeyFailClosedInProduction(t *testing.T) {
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")
	t.Setenv("DEPLOY_ENV", "production")
	t.Setenv("KRATOS_ENV", "")
	t.Setenv("APP_ENV", "")

	if _, err := SignKey(); !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected ErrSignKeyMissing, got %v", err)
	}
	if _, err := DownloadToken("art", 1, time.Now().Add(time.Minute)); !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected ErrSignKeyMissing from DownloadToken, got %v", err)
	}
	ok, err := VerifyDownloadToken("art", 1, time.Now().Add(time.Minute).Unix(), "deadbeef")
	if ok || !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected (false, ErrSignKeyMissing), got (%v, %v)", ok, err)
	}
}

// REV-A: staging and unrecognised envs must also fail-closed (whitelist approach).
func TestSignKeyFailClosedInStaging(t *testing.T) {
	for _, env := range []string{"staging", "pre-prod", "uat", "release", ""} {
		t.Run("DEPLOY_ENV="+env, func(t *testing.T) {
			t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
			t.Setenv("KRATOS_AUTH_SECRET", "")
			t.Setenv("DEPLOY_ENV", env)
			t.Setenv("KRATOS_ENV", "")
			t.Setenv("APP_ENV", "")

			if _, err := SignKey(); !errors.Is(err, ErrSignKeyMissing) {
				t.Fatalf("DEPLOY_ENV=%q: expected ErrSignKeyMissing, got %v", env, err)
			}
		})
	}
}

// In an explicit dev/local/test environment, SignKey falls back to the dev key
// so zero-config local development stays friction-free.
func TestSignKeyDevFallback(t *testing.T) {
	for _, env := range []string{"dev", "development", "local", "test"} {
		t.Run("DEPLOY_ENV="+env, func(t *testing.T) {
			t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
			t.Setenv("KRATOS_AUTH_SECRET", "")
			t.Setenv("DEPLOY_ENV", env)
			t.Setenv("KRATOS_ENV", "")
			t.Setenv("APP_ENV", "")

			key, err := SignKey()
			if err != nil {
				t.Fatalf("DEPLOY_ENV=%q: unexpected error: %v", env, err)
			}
			if len(key) == 0 {
				t.Fatalf("DEPLOY_ENV=%q: expected non-empty dev key", env)
			}
		})
	}
}

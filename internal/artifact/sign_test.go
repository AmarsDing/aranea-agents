package artifact

import (
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func newTestSigner() *Signer {
	return NewSigner(loggateway.NewNoop())
}

func TestDownloadTokenRoundTrip(t *testing.T) {
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")
	t.Setenv("DEPLOY_ENV", "dev")
	t.Setenv("KRATOS_ENV", "")
	t.Setenv("APP_ENV", "")

	s := newTestSigner()
	id := "art-123"
	version := 2
	ws := "ws-tenant"
	expires := time.Now().UTC().Add(5 * time.Minute)
	token, err := s.DownloadToken(id, version, expires, ws)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := s.VerifyDownloadToken(id, version, expires.Unix(), ws, token)
	if err != nil || !ok {
		t.Fatalf("expected valid token: ok=%v err=%v", ok, err)
	}
	bad, err := s.VerifyDownloadToken(id, version, expires.Unix(), ws, "bad")
	if err != nil || bad {
		t.Fatalf("expected invalid token rejected: ok=%v err=%v", bad, err)
	}
	expired, err := s.VerifyDownloadToken(id, version, time.Now().Add(-time.Minute).Unix(), ws, token)
	if err != nil || expired {
		t.Fatalf("expected expired token rejected: ok=%v err=%v", expired, err)
	}
	crossWS, err := s.VerifyDownloadToken(id, version, expires.Unix(), "ws-other", token)
	if err != nil || crossWS {
		t.Fatalf("expected workspace mismatch rejected: ok=%v err=%v", crossWS, err)
	}
}

func TestSignKeyFailClosedInProduction(t *testing.T) {
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")
	t.Setenv("DEPLOY_ENV", "production")
	t.Setenv("KRATOS_ENV", "")
	t.Setenv("APP_ENV", "")

	s := newTestSigner()
	if _, err := s.SignKey(); !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected ErrSignKeyMissing, got %v", err)
	}
	if _, err := s.DownloadToken("art", 1, time.Now().Add(time.Minute), "ws"); !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected ErrSignKeyMissing from DownloadToken, got %v", err)
	}
	ok, err := s.VerifyDownloadToken("art", 1, time.Now().Add(time.Minute).Unix(), "ws", "deadbeef")
	if ok || !errors.Is(err, ErrSignKeyMissing) {
		t.Fatalf("expected (false, ErrSignKeyMissing), got (%v, %v)", ok, err)
	}
}

func TestSignKeyFailClosedInStaging(t *testing.T) {
	for _, env := range []string{"staging", "pre-prod", "uat", "release", ""} {
		t.Run("DEPLOY_ENV="+env, func(t *testing.T) {
			t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
			t.Setenv("KRATOS_AUTH_SECRET", "")
			t.Setenv("DEPLOY_ENV", env)
			t.Setenv("KRATOS_ENV", "")
			t.Setenv("APP_ENV", "")

			s := newTestSigner()
			if _, err := s.SignKey(); !errors.Is(err, ErrSignKeyMissing) {
				t.Fatalf("DEPLOY_ENV=%q: expected ErrSignKeyMissing, got %v", env, err)
			}
		})
	}
}

func TestSignKeyDevFallback(t *testing.T) {
	for _, env := range []string{"dev", "development", "local", "test"} {
		t.Run("DEPLOY_ENV="+env, func(t *testing.T) {
			t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
			t.Setenv("KRATOS_AUTH_SECRET", "")
			t.Setenv("DEPLOY_ENV", env)
			t.Setenv("KRATOS_ENV", "")
			t.Setenv("APP_ENV", "")

			s := newTestSigner()
			key, err := s.SignKey()
			if err != nil {
				t.Fatalf("DEPLOY_ENV=%q: unexpected error: %v", env, err)
			}
			if len(key) == 0 {
				t.Fatalf("DEPLOY_ENV=%q: expected non-empty dev key", env)
			}
		})
	}
}

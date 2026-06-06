// Package artifact provides signed download URL helpers for artifact access.
package artifact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

const defaultDownloadTTL = 15 * time.Minute

// ErrSignKeyMissing is returned when no artifact signing key is configured in a
// production environment. OUT-05 / ART-02: prior behavior fell back to a
// hardcoded "aranea-artifact-dev-key" which makes every artifact URL forgeable.
var ErrSignKeyMissing = errors.New("artifact: signing key missing in production (set KRATOS_ARTIFACT_SIGN_KEY or KRATOS_AUTH_SECRET)")

// isDevEnv returns true ONLY when the process explicitly declares itself as a
// local development or test environment. All other env values (including staging,
// pre-prod, uat, release, or anything unrecognised) are treated as non-dev so
// that a missing KRATOS_ARTIFACT_SIGN_KEY causes fail-closed behaviour.
//
// Whitelist (allow dev key): dev | development | local | test
// Everything else (including empty / staging / prod / production) → fail-closed.
func isDevEnv() bool {
	for _, key := range []string{"DEPLOY_ENV", "KRATOS_ENV", "APP_ENV"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "dev" || v == "development" || v == "local" || v == "test" {
			return true
		}
	}
	return false
}

// Signer manages HMAC signing for artifact download tokens.
// The signing key is resolved once from environment variables and cached for
// the lifetime of the process. Key rotation requires a process restart.
type Signer struct {
	lg       loggateway.Logger
	once     sync.Once
	cached   []byte
	cacheErr error
}

func NewSigner(lg loggateway.Logger) *Signer {
	return &Signer{lg: lg}
}

func (s *Signer) SignKey() ([]byte, error) {
	s.once.Do(func() {
		if v := strings.TrimSpace(os.Getenv("KRATOS_ARTIFACT_SIGN_KEY")); v != "" {
			s.cached = []byte(v)
			return
		}
		if v := strings.TrimSpace(os.Getenv("KRATOS_AUTH_SECRET")); v != "" {
			s.cached = []byte(v)
			return
		}
		if isDevEnv() {
			s.lg.Warn("artifact: no signing key configured (KRATOS_ARTIFACT_SIGN_KEY / KRATOS_AUTH_SECRET); using insecure dev key — set a strong key before going to production",
				loggateway.StepID("auth.bypass_warn"),
			)
			s.cached = []byte("aranea-artifact-dev-key")
			return
		}
		s.cacheErr = ErrSignKeyMissing
	})
	return s.cached, s.cacheErr
}

func (s *Signer) DownloadToken(id string, version int, expires time.Time) (string, error) {
	key, err := s.SignKey()
	if err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%d|%d", strings.TrimSpace(id), version, expires.Unix())
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *Signer) VerifyDownloadToken(id string, version int, expiresUnix int64, token string) (bool, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" {
		return false, nil
	}
	if time.Now().Unix() > expiresUnix {
		return false, nil
	}
	expected, err := s.DownloadToken(id, version, time.Unix(expiresUnix, 0))
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(token))), nil
}

// DefaultDownloadExpiry returns the default signed URL expiry.
func DefaultDownloadExpiry() time.Time {
	return time.Now().UTC().Add(defaultDownloadTTL)
}

// ParseExpires parses expires query parameter as unix timestamp.
func ParseExpires(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("expires is required")
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid expires")
	}
	return n, nil
}

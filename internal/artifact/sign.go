// Package artifact provides signed download URL helpers for artifact access.
package artifact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultDownloadTTL = 15 * time.Minute

// SignKey returns the HMAC secret used for artifact download tokens.
func SignKey() []byte {
	if v := strings.TrimSpace(os.Getenv("KRATOS_ARTIFACT_SIGN_KEY")); v != "" {
		return []byte(v)
	}
	if v := strings.TrimSpace(os.Getenv("KRATOS_AUTH_SECRET")); v != "" {
		return []byte(v)
	}
	return []byte("aranea-artifact-dev-key")
}

// DownloadToken builds an HMAC-SHA256 token for artifact download.
// payload: id|version|expiresUnix
func DownloadToken(id string, version int, expires time.Time) string {
	payload := fmt.Sprintf("%s|%d|%d", strings.TrimSpace(id), version, expires.Unix())
	mac := hmac.New(sha256.New, SignKey())
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyDownloadToken checks token validity for the given artifact and expiry.
func VerifyDownloadToken(id string, version int, expiresUnix int64, token string) bool {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" {
		return false
	}
	if time.Now().Unix() > expiresUnix {
		return false
	}
	expected := DownloadToken(id, version, time.Unix(expiresUnix, 0))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(token)))
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

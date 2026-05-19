package artifact

import (
	"testing"
	"time"
)

func TestDownloadTokenRoundTrip(t *testing.T) {
	id := "art-123"
	version := 2
	expires := time.Now().UTC().Add(5 * time.Minute)
	token := DownloadToken(id, version, expires)
	if !VerifyDownloadToken(id, version, expires.Unix(), token) {
		t.Fatal("expected valid token")
	}
	if VerifyDownloadToken(id, version, expires.Unix(), "bad") {
		t.Fatal("expected invalid token rejected")
	}
	if VerifyDownloadToken(id, version, time.Now().Add(-time.Minute).Unix(), token) {
		t.Fatal("expected expired token rejected")
	}
}

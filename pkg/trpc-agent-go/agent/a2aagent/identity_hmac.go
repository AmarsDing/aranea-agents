package a2aagent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// identity_hmac.go — client half of the signed server-to-server identity
// channel (aranea V-2). The signing scheme MUST stay byte-compatible with
// the server-side twin in pkg/trpc-agent-go/server/a2a/identity_hmac.go
// (hex of HMAC-SHA256(secret, userID)); both sides carry a cross-reference
// comment and the a2a package pins the pairing.

// SignUserIDForHeader computes hex(HMAC-SHA256(secret, userID)).
func SignUserIDForHeader(secret, userID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

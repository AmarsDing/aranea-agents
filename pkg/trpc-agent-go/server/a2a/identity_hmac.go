package a2a

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// identity_hmac.go — signed server-to-server identity channel (aranea V-2).
//
// The unsigned UserID header is honored only inside the anonymous-cookie
// namespace (aranea E2E-P0 G3). Trusted mesh callers (aranea→aranea A2A
// proxy agents) can instead present an HMAC-SHA256 signature of the user ID
// in the "<userIDHeader>-Signature" header, keyed by a shared secret
// configured via WithUserIDHeaderHMACSecret. A valid signature proves the
// caller holds the fleet secret, so the claimed identity is honored as-is.

// userIDSignatureHeader derives the companion signature header name.
func userIDSignatureHeader(userIDHeader string) string {
	return strings.TrimSpace(userIDHeader) + "-Signature"
}

// SignUserIDForHeader computes hex(HMAC-SHA256(secret, userID)).
func SignUserIDForHeader(secret, userID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyUserIDSignature compares a presented signature in constant time.
func verifyUserIDSignature(secret, userID, presented string) bool {
	if secret == "" || userID == "" || presented == "" {
		return false
	}
	expected := SignUserIDForHeader(secret, userID)
	if len(expected) != len(presented) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

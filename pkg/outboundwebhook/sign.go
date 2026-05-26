// Package outboundwebhook provides HMAC-SHA256 signing helpers for outbound webhook deliveries.
//
// Both Hook notify and Gateway webhook paths use this package to ensure consistent
// v1=<hex-hmac> signature format with replay protection via X-Webhook-Timestamp.
//
// Signature format: HMAC-SHA256 over "<timestamp>\n<body>", delivered as
// the header "X-Webhook-Signature: v1=<hex>", with timestamp in
// "X-Webhook-Timestamp: <unix-seconds>".
//
// Receivers validate by recomputing HMAC over the same payload and checking
// that the timestamp is within an acceptable window (e.g. ±5 min).
package outboundwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// HeaderSignature is the HTTP header carrying the HMAC signature.
	HeaderSignature = "X-Webhook-Signature"
	// HeaderTimestamp is the HTTP header carrying the Unix timestamp used in the HMAC.
	HeaderTimestamp = "X-Webhook-Timestamp"

	signatureVersion = "v1"
)

// SignBody computes HMAC-SHA256 over "<timestampSec>\n<body>" using secret
// and returns a "v1=<hex>" string.
func SignBody(secret string, timestampSec int64, body []byte) string {
	message := fmt.Sprintf("%d\n", timestampSec)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	_, _ = mac.Write(body)
	return signatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}

// AddSignatureHeaders sets X-Webhook-Signature and X-Webhook-Timestamp on req.
// If secret is empty, no headers are added.
func AddSignatureHeaders(req *http.Request, secret string, body []byte) {
	if req == nil || strings.TrimSpace(secret) == "" {
		return
	}
	ts := time.Now().Unix()
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, SignBody(secret, ts, body))
}

// Verify checks that sig (from X-Webhook-Signature) matches the HMAC computed
// over the received body using secret. It also checks that the timestamp is
// within maxAge of now to prevent replay attacks.
// Returns nil if valid, a descriptive error otherwise.
func Verify(secret string, timestampSec int64, body []byte, sig string, maxAge time.Duration) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("outboundwebhook: no secret configured")
	}
	age := time.Since(time.Unix(timestampSec, 0))
	if age < 0 {
		age = -age
	}
	if maxAge > 0 && age > maxAge {
		return fmt.Errorf("outboundwebhook: timestamp too old (%s)", age.Round(time.Second))
	}
	expected := SignBody(secret, timestampSec, body)
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(sig)))) {
		return fmt.Errorf("outboundwebhook: signature mismatch")
	}
	return nil
}

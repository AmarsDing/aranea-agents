package teams

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound_MessageActivity(t *testing.T) {
	raw := []byte(`{
		"type": "message",
		"id": "act123",
		"timestamp": "2026-01-01T00:00:00Z",
		"serviceUrl": "https://smba.trafficmanager.net/amer/",
		"channelId": "msteams",
		"from": {"id": "user1", "name": "Test User"},
		"conversation": {"id": "conv1", "conversationType": "personal"},
		"recipient": {"id": "bot1", "name": "Test Bot"},
		"text": " hello world "
	}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello world" {
		t.Fatalf("text: got %q", msg.Text)
	}
	if msg.FromID != "user1" {
		t.Fatalf("from_id: got %q", msg.FromID)
	}
	if msg.ConversationID != "conv1" {
		t.Fatalf("conversation_id: got %q", msg.ConversationID)
	}
	if msg.ServiceURL != "https://smba.trafficmanager.net/amer/" {
		t.Fatalf("service_url: got %q", msg.ServiceURL)
	}
	if msg.ActivityID != "act123" {
		t.Fatalf("activity_id: got %q", msg.ActivityID)
	}
	if msg.RecipientID != "bot1" {
		t.Fatalf("recipient_id: got %q", msg.RecipientID)
	}
}

func TestParseInbound_NonMessageIgnored(t *testing.T) {
	raw := []byte(`{
		"type": "conversationUpdate",
		"id": "act456",
		"channelId": "msteams"
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for non-message activity")
	}
}

func TestParseInbound_EmptyText(t *testing.T) {
	raw := []byte(`{
		"type": "message",
		"id": "act789",
		"channelId": "msteams",
		"from": {"id": "u1"},
		"conversation": {"id": "c1"},
		"text": "   "
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestTextSenderID(t *testing.T) {
	s := &TextSender{}
	if s.ID() != "teams" {
		t.Fatalf("id: got %q", s.ID())
	}
}

func TestVerifyRequest_EmptyCredentials(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer some-token")
	err := VerifyRequest(context.Background(), "", "", h, nil)
	if !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}

func TestVerifyRequest_RS256Valid(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-kid"
	const appID = "app-123"

	keysSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksDocument{Keys: []jwkKey{{
			Kid: kid,
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
			N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
		}}})
	}))
	defer keysSrv.Close()

	metaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openIDMetadata{
			Issuer:  botFrameworkIssuer,
			JWKSURI: keysSrv.URL,
		})
	}))
	defer metaSrv.Close()

	ResetJWKSCacheForTest()
	defaultJWKS.mu.Lock()
	defaultJWKS.metaURL = metaSrv.URL
	defaultJWKS.client = metaSrv.Client()
	defaultJWKS.mu.Unlock()
	t.Cleanup(func() {
		ResetJWKSCacheForTest()
		defaultJWKS.mu.Lock()
		defaultJWKS.metaURL = botFrameworkOpenIDURL
		defaultJWKS.client = &http.Client{Timeout: jwksFetchTimeout}
		defaultJWKS.mu.Unlock()
	})

	claims := jwt.MapClaims{
		"aud": appID,
		"iss": botFrameworkIssuer,
		"exp": float64(time.Now().Add(time.Hour).Unix()),
		"iat": float64(time.Now().Unix()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+signed)
	if err := VerifyRequest(context.Background(), appID, "", h, nil); err != nil {
		t.Fatalf("expected RS256 verify success, got %v", err)
	}
}

func TestVerifyRequest_RS256BadAudience(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-kid-2"

	keysSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksDocument{Keys: []jwkKey{{
			Kid: kid,
			Kty: "RSA",
			N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
		}}})
	}))
	defer keysSrv.Close()
	metaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openIDMetadata{Issuer: botFrameworkIssuer, JWKSURI: keysSrv.URL})
	}))
	defer metaSrv.Close()

	ResetJWKSCacheForTest()
	defaultJWKS.mu.Lock()
	defaultJWKS.metaURL = metaSrv.URL
	defaultJWKS.client = metaSrv.Client()
	defaultJWKS.mu.Unlock()
	t.Cleanup(func() {
		ResetJWKSCacheForTest()
		defaultJWKS.mu.Lock()
		defaultJWKS.metaURL = botFrameworkOpenIDURL
		defaultJWKS.client = &http.Client{Timeout: jwksFetchTimeout}
		defaultJWKS.mu.Unlock()
	})

	claims := jwt.MapClaims{
		"aud": "other-app",
		"iss": botFrameworkIssuer,
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+signed)
	if err := VerifyRequest(context.Background(), "app-123", "", h, nil); err == nil {
		t.Fatal("expected audience mismatch error")
	}
}

package a2a

import (
	"net/http/httptest"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/agent/a2aagent"
)

// V-2 pins for the signed server-to-server identity channel: a caller holding
// the fleet shared secret proves the claimed UserID with an HMAC signature and
// is honored as-is; unsigned/tampered identities keep being dropped into the
// anonymous namespace (P0 G3 behavior preserved).

func TestDefaultAuthProviderHonorsSignedIdentity(t *testing.T) {
	secret := "fleet-secret"
	p := &defaultAuthProvider{userIDHeader: "X-User-ID", hmacSecret: secret}

	r := httptest.NewRequest("POST", "/a2a", nil)
	r.Header.Set("X-User-ID", "user-42")
	r.Header.Set("X-User-ID-Signature", SignUserIDForHeader(secret, "user-42"))

	u, err := p.Authenticate(r)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.ID != "user-42" {
		t.Fatalf("signed identity must be honored, got %+v", u)
	}
	if u.Claims[anonymousAuthProvenanceClaim] == true {
		t.Fatal("signed identity must not carry anonymous provenance (no cookie minting)")
	}
}

func TestDefaultAuthProviderRejectsBadOrMissingSignature(t *testing.T) {
	secret := "fleet-secret"
	p := &defaultAuthProvider{userIDHeader: "X-User-ID", hmacSecret: secret}

	cases := map[string]string{
		"missing signature": "",
		"wrong secret":      SignUserIDForHeader("other-secret", "user-42"),
		"tampered user id":  SignUserIDForHeader(secret, "user-7"),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/a2a", nil)
			r.Header.Set("X-User-ID", "user-42")
			if sig != "" {
				r.Header.Set("X-User-ID-Signature", sig)
			}
			u, err := p.Authenticate(r)
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if u == nil || u.ID == "user-42" {
				t.Fatalf("forged identity must be dropped, got %+v", u)
			}
			if !isAnonymousUserIDForScope(u.ID, "") {
				t.Fatalf("expected anonymous fallback principal, got %q", u.ID)
			}
		})
	}
}

func TestDefaultAuthProviderNoSecretKeepsP0Drop(t *testing.T) {
	// Secret not configured → signatures are not consulted; unsigned
	// non-anonymous header identities are dropped exactly as P0 mandated.
	p := &defaultAuthProvider{userIDHeader: "X-User-ID"}
	r := httptest.NewRequest("POST", "/a2a", nil)
	r.Header.Set("X-User-ID", "user-42")
	r.Header.Set("X-User-ID-Signature", SignUserIDForHeader("any", "user-42"))

	u, err := p.Authenticate(r)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.ID == "user-42" {
		t.Fatalf("without a configured secret the header must still be dropped, got %+v", u)
	}
}

// TestIdentityHMACClientServerPairing pins byte-compatibility between the
// client signer (a2aagent) and the server verifier: drift here silently
// re-breaks aranea→aranea identity propagation.
func TestIdentityHMACClientServerPairing(t *testing.T) {
	secret, userID := "fleet-secret", "user-42"
	sig := a2aagent.SignUserIDForHeader(secret, userID)
	if !verifyUserIDSignature(secret, userID, sig) {
		t.Fatalf("client signature %q not accepted by server verifier", sig)
	}
}

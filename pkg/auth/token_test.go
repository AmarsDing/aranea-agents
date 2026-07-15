package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenExpired(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	claims := jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed, secret); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestParseTokenBadSignature(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	claims := jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed, secret+"x"); err == nil {
		t.Fatal("expected bad signature error")
	}
}

func TestGenerateTokenIncludesWorkspace(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	token, err := GenerateToken(7, "user", secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("expected workspace %q, got %q", DefaultWorkspaceID, claims.WorkspaceID)
	}
	if claims.EffectiveWorkspaceID() != DefaultWorkspaceID {
		t.Fatalf("EffectiveWorkspaceID = %q", claims.EffectiveWorkspaceID())
	}
}

func TestGenerateTokenForWorkspace(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	token, err := GenerateTokenForWorkspace(7, "user", "ws-tenant-a", secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.WorkspaceID != "ws-tenant-a" {
		t.Fatalf("expected ws-tenant-a, got %q", claims.WorkspaceID)
	}
}

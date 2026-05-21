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

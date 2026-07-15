package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// GenerateToken generates a JWT token for the given user.
// Workspace is bound to DefaultWorkspaceID (B-01: membership comes from JWT, not client headers).
func GenerateToken(userID int64, access, secret string, expiresAt time.Time) (string, error) {
	return GenerateTokenForWorkspace(userID, access, DefaultWorkspaceID, secret, expiresAt)
}

// GenerateTokenForWorkspace generates a JWT bound to an explicit workspace membership.
func GenerateTokenForWorkspace(userID int64, access, workspaceID, secret string, expiresAt time.Time) (string, error) {
	if workspaceID == "" {
		workspaceID = DefaultWorkspaceID
	}
	now := time.Now()
	claims := Auth{
		UserID:      userID,
		Access:      access,
		WorkspaceID: workspaceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Issuer:    "kratos",
			Subject:   "user",
			Audience:  []string{"admin"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken parses the JWT token string and returns the Auth claims.
func ParseToken(tokenStr, secret string) (*Auth, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Auth{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, ErrUnauthorized
	}
	auth, ok := token.Claims.(*Auth)
	if !ok {
		return nil, ErrUnauthorized
	}
	return auth, nil
}

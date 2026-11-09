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
	return GenerateTokenForPrincipal(Principal{
		UserID:      userID,
		Access:      access,
		WorkspaceID: workspaceID,
	}, secret, expiresAt)
}

// Principal carries the full P1 claim set stamped into a JWT
// (P0 详设 01 §5：{id, roles[], clearance, workspace_id, tv}）。
// Access 为过渡字段，随 admins.access 列迁移完成后删除。
type Principal struct {
	UserID       int64
	Access       string
	WorkspaceID  string
	Roles        []string
	Clearance    int
	TokenVersion int64
	// ForceReset 随签发烙入 fr claim（P1 补漏·强制改密门）。
	ForceReset bool
}

// GenerateTokenForPrincipal generates a JWT carrying the full P1 claim set.
// TokenVersion 取自签发时刻的 admins.token_version；服务端每次校验时与
// Redis（write-through）/DB 当前值比对，不一致即拒绝（P1-3）。
func GenerateTokenForPrincipal(p Principal, secret string, expiresAt time.Time) (string, error) {
	workspaceID := p.WorkspaceID
	if workspaceID == "" {
		workspaceID = DefaultWorkspaceID
	}
	now := time.Now()
	claims := Auth{
		UserID:       p.UserID,
		Access:       p.Access,
		WorkspaceID:  workspaceID,
		Roles:        p.Roles,
		Clearance:    p.Clearance,
		TokenVersion: p.TokenVersion,
		ForceReset:   p.ForceReset,
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

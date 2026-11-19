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

// TestGenerateTokenForPrincipalRoundTrip pins the P1 claim set
// (P0 详设 01 §5：{id, roles[], clearance, workspace_id, tv}）。
func TestGenerateTokenForPrincipalRoundTrip(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	p := Principal{
		UserID:       42,
		Access:       "admin",
		WorkspaceID:  "ws-tenant-b",
		Roles:        []string{"superadmin", "auditor"},
		Clearance:    4,
		TokenVersion: 7,
	}
	token, err := GenerateTokenForPrincipal(p, secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 || claims.WorkspaceID != "ws-tenant-b" {
		t.Fatalf("id/workspace mismatch: %+v", claims)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "superadmin" || claims.Roles[1] != "auditor" {
		t.Fatalf("roles mismatch: %v", claims.Roles)
	}
	if claims.Clearance != 4 {
		t.Fatalf("clearance = %d, want 4", claims.Clearance)
	}
	if claims.TokenVersion != 7 {
		t.Fatalf("tv = %d, want 7", claims.TokenVersion)
	}
	if !claims.HasRole("auditor") || !claims.HasPermission("audit.read") || !claims.HasClearance(3) {
		t.Fatal("round-tripped claims must drive the P1 predicates")
	}
}

// fr claim(P1 补漏·强制改密门)必须随签发烙入并随解析还原;默认零值不带出。
func TestGenerateTokenForceResetRoundTrip(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	token, err := GenerateTokenForPrincipal(Principal{UserID: 9, Access: "user", ForceReset: true}, secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.ForceReset {
		t.Fatal("fr claim 丢失:强制改密门会失效")
	}

	plain, err := GenerateTokenForPrincipal(Principal{UserID: 9, Access: "user"}, secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plainClaims, err := ParseToken(plain, secret)
	if err != nil {
		t.Fatal(err)
	}
	if plainClaims.ForceReset {
		t.Fatal("零值 fr 应解析为 false")
	}
}

// TestGenerateTokenForPrincipalDefaults：空 workspace 归一 default；零值
// roles/clearance/tv 兼容旧 token 语义（L1 / tv=0）。
func TestGenerateTokenForPrincipalDefaults(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	token, err := GenerateTokenForPrincipal(Principal{UserID: 1, Access: "user"}, secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("empty workspace → %q, want %q", claims.WorkspaceID, DefaultWorkspaceID)
	}
	if len(claims.Roles) != 0 || claims.ClearanceLevel() != 1 || claims.TokenVersion != 0 {
		t.Fatalf("defaults mismatch: %+v", claims)
	}
}

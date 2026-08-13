package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpconfig "aranea-agents/internal/mcp/config"
)

func clearOAuth2CacheEntry(tokenURL, clientID string) {
	oauth2CacheMu.Lock()
	delete(oauth2Cache, oauth2CacheKey(tokenURL, clientID))
	oauth2CacheMu.Unlock()
}

// 轮换场景：provider 返回新 refresh_token 时必须经 persister 回写持久层，
// 否则进程重启后旧（已撤销）refresh token 导致永久鉴权失败。
func TestResolveMCPAuthToken_RefreshRotationPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token sent = %q, want old-refresh", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-new",
			"expires_in":    3600,
			"refresh_token": "rotated-refresh",
		})
	}))
	defer srv.Close()
	defer clearOAuth2CacheEntry(srv.URL, "cid")

	var gotKey, gotRefresh string
	SetMCPRefreshTokenPersister(func(_ context.Context, serverKey, refreshToken string) error {
		gotKey, gotRefresh = serverKey, refreshToken
		return nil
	})
	defer SetMCPRefreshTokenPersister(nil)

	token, err := ResolveMCPAuthToken(context.Background(), "srv1", mcpconfig.AuthConfig{
		Type:         "oauth2_refresh",
		TokenURL:     srv.URL,
		ClientID:     "cid",
		RefreshToken: "old-refresh",
	})
	if err != nil {
		t.Fatalf("ResolveMCPAuthToken: %v", err)
	}
	if token != "at-new" {
		t.Fatalf("access token = %q, want at-new", token)
	}
	if gotKey != "srv1" || gotRefresh != "rotated-refresh" {
		t.Fatalf("persister called with (%q, %q), want (srv1, rotated-refresh)", gotKey, gotRefresh)
	}
}

// 非轮换场景：provider 不返回新 refresh_token 时不得触发回写。
func TestResolveMCPAuthToken_RefreshNoRotationNoPersist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-plain",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()
	defer clearOAuth2CacheEntry(srv.URL, "cid")

	called := false
	SetMCPRefreshTokenPersister(func(_ context.Context, _, _ string) error {
		called = true
		return nil
	})
	defer SetMCPRefreshTokenPersister(nil)

	if _, err := ResolveMCPAuthToken(context.Background(), "srv1", mcpconfig.AuthConfig{
		Type:         "oauth2_refresh",
		TokenURL:     srv.URL,
		ClientID:     "cid",
		RefreshToken: "old-refresh",
	}); err != nil {
		t.Fatalf("ResolveMCPAuthToken: %v", err)
	}
	if called {
		t.Fatal("persister must not be called when provider returns no new refresh_token")
	}
}

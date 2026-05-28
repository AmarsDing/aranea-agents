package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpconfig "aranea-agents/internal/mcp/config"
)

func TestFetchOAuth2ClientCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-abc"})
	}))
	defer srv.Close()

	tok, err := fetchOAuth2ClientCredentials(context.Background(), mcpconfig.AuthConfig{
		Type:         "oauth2_client_credentials",
		TokenURL:     srv.URL,
		ClientID:     "cid",
		ClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-abc" {
		t.Fatalf("token=%q", tok)
	}
}

func TestFetchOAuth2RefreshToken_NoFallback(t *testing.T) {
	_, err := fetchOAuth2RefreshToken(context.Background(), mcpconfig.AuthConfig{
		Type:         "oauth2_refresh",
		AccessToken:  "stale-tok",
		RefreshToken: "",
		TokenURL:     "",
	})
	if err == nil {
		t.Fatal("expected error when refresh_token and token_url are empty; should not fallback to stale access_token")
	}
}

func TestFetchOAuth2RefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "refreshed-tok"})
	}))
	defer srv.Close()

	tok, err := fetchOAuth2RefreshToken(context.Background(), mcpconfig.AuthConfig{
		Type:         "oauth2_refresh",
		TokenURL:     srv.URL,
		RefreshToken: "old-refresh",
		ClientID:     "cid",
		ClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok != "refreshed-tok" {
		t.Fatalf("token=%q", tok)
	}
}

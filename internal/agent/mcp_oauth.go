package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpconfig "aranea-agents/internal/mcp/config"
	mcpdefaults "aranea-agents/internal/mcp"
)

func resolveMCPAuthToken(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
	authType := strings.ToLower(strings.TrimSpace(auth.Type))
	switch authType {
	case "oauth2", "oauth2_client_credentials":
		return fetchOAuth2ClientCredentials(ctx, auth)
	case "oauth2_refresh":
		return fetchOAuth2RefreshToken(ctx, auth)
	case "oauth2_static":
		return strings.TrimSpace(auth.AccessToken), nil
	default:
		return strings.TrimSpace(auth.APIKey), nil
	}
}

func fetchOAuth2ClientCredentials(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
	if t := strings.TrimSpace(auth.AccessToken); t != "" && strings.TrimSpace(auth.TokenURL) == "" {
		return t, nil
	}
	tokenURL := strings.TrimSpace(auth.TokenURL)
	clientID := strings.TrimSpace(auth.ClientID)
	clientSecret := strings.TrimSpace(auth.ClientSecret)
	if tokenURL == "" || clientID == "" {
		return "", fmt.Errorf("oauth2: token_url and client_id are required")
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if scope := strings.TrimSpace(auth.Scope); scope != "" {
		form.Set("scope", scope)
	}
	return postOAuth2Token(ctx, tokenURL, form, clientID, clientSecret)
}

func fetchOAuth2RefreshToken(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
	tokenURL := strings.TrimSpace(auth.TokenURL)
	refresh := strings.TrimSpace(auth.RefreshToken)
	if tokenURL == "" || refresh == "" {
		return "", fmt.Errorf("oauth2_refresh: token_url and refresh_token are required")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	if scope := strings.TrimSpace(auth.Scope); scope != "" {
		form.Set("scope", scope)
	}
	return postOAuth2Token(ctx, tokenURL, form, strings.TrimSpace(auth.ClientID), strings.TrimSpace(auth.ClientSecret))
}

func postOAuth2Token(ctx context.Context, tokenURL string, form url.Values, clientID, clientSecret string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientID != "" && clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}
	client := &http.Client{Timeout: time.Duration(mcpdefaults.DefaultOAuth2TimeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("oauth2 token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("oauth2 token: empty access_token")
	}
	return parsed.AccessToken, nil
}

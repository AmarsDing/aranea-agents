package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type TextSender struct {
	AppID     string
	AppSecret string
	HTTP      *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

func (s *TextSender) ID() string { return "teams" }

func (s *TextSender) SendText(ctx context.Context, recipient, text string) error {
	recipient = strings.TrimSpace(recipient)
	text = strings.TrimSpace(text)
	if recipient == "" || text == "" {
		return nil
	}
	token, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	return SendToConversation(ctx, s.HTTP, token, recipient, recipient, text)
}

func SendToConversation(ctx context.Context, httpClient *http.Client, token, serviceURL, conversationID, text string) error {
	serviceURL = strings.TrimSpace(serviceURL)
	conversationID = strings.TrimSpace(conversationID)
	text = strings.TrimSpace(text)
	if serviceURL == "" || conversationID == "" || text == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"type": "message",
		"text": text,
	})
	base := strings.TrimRight(serviceURL, "/")
	url := base + "/v3/conversations/" + conversationID + "/activities"
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("teams outbound: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s *TextSender) AccessToken(ctx context.Context) (string, error) {
	return s.accessToken(ctx)
}

func (s *TextSender) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.exp) {
		return s.token, nil
	}
	appID := strings.TrimSpace(s.AppID)
	appSecret := strings.TrimSpace(s.AppSecret)
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("teams: app_id and app_secret required")
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "https://api.botframework.com/.default")
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("teams token: %s", strings.TrimSpace(out.Description))
	}
	s.token = out.AccessToken
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 3600 * time.Second
	}
	buffer := 5 * time.Minute
	if ttl <= buffer {
		buffer = ttl / 2
	}
	s.exp = time.Now().Add(ttl - buffer)
	return out.AccessToken, nil
}

package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/safego"

	"github.com/gorilla/websocket"
)

func init() {
	runtime.RegisterStarter("mattermost", "websocket", RunWebSocket)
}

func RunWebSocket(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler runtime.InboundHandler,
) error {
	serverURL, err := lookup(ctx, creds, "server_url")
	if err != nil {
		return err
	}
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	serverURL = strings.TrimSpace(serverURL)
	botToken = strings.TrimSpace(botToken)
	if serverURL == "" || botToken == "" {
		return fmt.Errorf("mattermost websocket: server_url and bot_token required")
	}

	botUserID, err := fetchBotUserID(ctx, serverURL, botToken)
	if err != nil {
		return fmt.Errorf("mattermost websocket: get user: %w", err)
	}

	wsURL := buildWSURL(serverURL)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+botToken)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("mattermost websocket: dial: %w", err)
	}
	defer conn.Close()

	chRow := ch
	readErr := make(chan error, 1)
	safego.Go(ctx, "channel.mattermost.ws.inbound", func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}
			ev, ok := parseWSMessage(message, botUserID)
			if !ok {
				continue
			}
			ev.PlatformType = "mattermost"
			_ = handler.ProcessInbound(ctx, chRow, ev)
		}
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return fmt.Errorf("mattermost websocket: read failed: %w", err)
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"seq":0,"action":"ping"}`)); err != nil {
				return fmt.Errorf("mattermost websocket: ping failed: %w", err)
			}
		}
	}
}

func fetchBotUserID(ctx context.Context, serverURL, token string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v4/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var user struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &user)
	return strings.TrimSpace(user.ID), nil
}

func buildWSURL(serverURL string) string {
	u := strings.TrimSpace(serverURL)
	u = strings.TrimRight(u, "/")
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return u + "/api/v4/websocket"
}

func parseWSMessage(raw []byte, botUserID string) (port.InboundEvent, bool) {
	var envelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return port.InboundEvent{}, false
	}
	if envelope.Event != "posted" {
		return port.InboundEvent{}, false
	}
	var data struct {
		Post       string `json:"post"`
		SenderType string `json:"sender_type"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return port.InboundEvent{}, false
	}
	if strings.EqualFold(data.SenderType, "bot") {
		return port.InboundEvent{}, false
	}
	var post struct {
		Message   string `json:"message"`
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal([]byte(data.Post), &post); err != nil {
		return port.InboundEvent{}, false
	}
	text := strings.TrimSpace(post.Message)
	if text == "" {
		return port.InboundEvent{}, false
	}
	if strings.TrimSpace(post.UserID) == strings.TrimSpace(botUserID) {
		return port.InboundEvent{}, false
	}
	channelID := strings.TrimSpace(post.ChannelID)
	userID := strings.TrimSpace(post.UserID)
	return port.InboundEvent{
		PeerID:         firstNonEmpty(userID, channelID),
		Text:           text,
		IdempotencyKey: "mattermost:" + strings.TrimSpace(post.ID),
		OutboundMeta: map[string]string{
			"recipient": channelID,
			"chat_id":   channelID,
		},
	}, true
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

package onebot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TextSender posts send_private_msg / send_group_msg to OneBot HTTP API.
type TextSender struct {
	HTTPServer string
	SendToken  string
	HTTP       *http.Client
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "personal_qq" }

// SendText delivers text; recipient is user_id or group_id depending on meta.
func (s *TextSender) SendText(ctx context.Context, recipient, text string, groupID string) error {
	recipient = strings.TrimSpace(recipient)
	text = strings.TrimSpace(text)
	if recipient == "" || text == "" {
		return nil
	}
	base := strings.TrimRight(strings.TrimSpace(s.HTTPServer), "/")
	if base == "" {
		return fmt.Errorf("onebot outbound: http_server required")
	}
	action := "send_private_msg"
	params := map[string]any{
		"user_id": recipient,
		"message": text,
	}
	if strings.TrimSpace(groupID) != "" {
		action = "send_group_msg"
		params = map[string]any{
			"group_id": groupID,
			"message":  text,
		}
	}
	body, _ := json.Marshal(map[string]any{
		"action": action,
		"params": params,
	})
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(s.SendToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("onebot outbound: %s", strings.TrimSpace(string(raw)))
	}
	return nil
}

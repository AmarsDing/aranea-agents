package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TextSender posts replies via Slack chat.postMessage.
type TextSender struct {
	BotToken string
	HTTP     *http.Client
	Lg       loggateway.Logger
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "slack" }

// SendText delivers plain text to a Slack channel ID.
func (s *TextSender) SendText(ctx context.Context, channelID, text string) error {
	channelID = strings.TrimSpace(channelID)
	text = strings.TrimSpace(text)
	if channelID == "" {
		return errStreamChannelRequired
	}
	if text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return errBotTokenRequired
	}
	body, _ := json.Marshal(map[string]any{
		"channel": channelID,
		"text":    text,
	})
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		s.Lg.Warn("解析 slack outbound 响应失败", loggateway.StepID("channel.slack.outbound"), loggateway.Err(err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
		msg := strings.TrimSpace(out.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return slackAPIError("slack outbound", msg)
	}
	return nil
}

// AuthTest calls auth.test for connection checks.
func AuthTest(ctx context.Context, client *http.Client, botToken string, lg loggateway.Logger) error {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return errBotTokenRequired
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/auth.test", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+botToken)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		lg.Warn("解析 slack auth.test 响应失败", loggateway.StepID("channel.slack.auth_test"), loggateway.Err(err))
	}
	if !out.OK {
		if out.Error != "" {
			return slackAPIError("slack auth.test", out.Error)
		}
		return slackAPIError("slack auth.test", "failed")
	}
	return nil
}

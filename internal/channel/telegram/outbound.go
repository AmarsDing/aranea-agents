package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TextSender posts replies via Telegram sendMessage.
type TextSender struct {
	BotToken string
	HTTP     *http.Client
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "telegram" }

// SendText delivers plain text to a Telegram chat id.
func (s *TextSender) SendText(ctx context.Context, chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" || text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return fmt.Errorf("telegram outbound: bot_token required")
	}
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram outbound: bad chat id")
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id": id,
		"text":    text,
	})
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
		msg := strings.TrimSpace(out.Description)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return fmt.Errorf("telegram outbound: %s", msg)
	}
	return nil
}

// GetMe calls getMe for connection checks.
func GetMe(ctx context.Context, client *http.Client, botToken string) error {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return fmt.Errorf("telegram: bot_token required")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	url := "https://api.telegram.org/bot" + botToken + "/getMe"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &out)
	if !out.OK {
		if out.Description != "" {
			return fmt.Errorf("telegram getMe: %s", out.Description)
		}
		return fmt.Errorf("telegram getMe failed")
	}
	return nil
}

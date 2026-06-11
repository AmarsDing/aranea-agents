package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TextSender posts replies via Telegram sendMessage.
type TextSender struct {
	BotToken string
	HTTP     *http.Client
	Lg       loggateway.Logger
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "telegram" }

// SendText delivers plain text to a Telegram chat id.
func (s *TextSender) SendText(ctx context.Context, chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" {
		return errBadChatID
	}
	if text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return errBotTokenRequired
	}
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return errBadChatID
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
	if err := json.Unmarshal(raw, &out); err != nil {
		s.Lg.Warn("解析 telegram outbound 响应失败", loggateway.StepID("channel.telegram.outbound"), loggateway.Err(err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
		msg := strings.TrimSpace(out.Description)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return telegramAPIError("telegram outbound", msg)
	}
	return nil
}

// GetMe calls getMe for connection checks.
func GetMe(ctx context.Context, client *http.Client, botToken string, lg loggateway.Logger) error {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return errBotTokenRequired
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
	if err := json.Unmarshal(raw, &out); err != nil {
		lg.Warn("解析 telegram getMe 响应失败", loggateway.StepID("channel.telegram.get_me"), loggateway.Err(err))
	}
	if !out.OK {
		if out.Description != "" {
			return telegramAPIError("telegram getMe", out.Description)
		}
		return telegramAPIError("telegram getMe", "failed")
	}
	return nil
}

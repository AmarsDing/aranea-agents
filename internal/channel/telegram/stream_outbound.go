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
	"sync"
	"time"
)

const defaultStreamEditInterval = 2 * time.Second

// StreamSender posts the first reply then edits in place (Telegram editMessageText).
type StreamSender struct {
	BotToken     string
	HTTP         *http.Client
	EditInterval time.Duration

	mu        sync.Mutex
	chatID    string
	messageID int64
	lastEdit  time.Time
}

// ID implements channel.Identified.
func (s *StreamSender) ID() string { return "telegram" }

// PreviewMessageID returns the Telegram message id after the first successful send.
func (s *StreamSender) PreviewMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageID == 0 {
		return ""
	}
	return strconv.FormatInt(s.messageID, 10)
}

// Update sends or edits the preview message. force bypasses throttle (final flush).
func (s *StreamSender) Update(ctx context.Context, chatID, text string, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) > 4096 {
		text = text[:4096]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageID == 0 || s.chatID != strings.TrimSpace(chatID) {
		id, err := s.sendMessage(ctx, chatID, text)
		if err != nil {
			return err
		}
		s.chatID = strings.TrimSpace(chatID)
		s.messageID = id
		s.lastEdit = time.Now()
		return nil
	}
	interval := s.EditInterval
	if interval <= 0 {
		interval = defaultStreamEditInterval
	}
	if !force && time.Since(s.lastEdit) < interval {
		return nil
	}
	if err := s.editMessage(ctx, s.chatID, s.messageID, text); err != nil {
		return err
	}
	s.lastEdit = time.Now()
	return nil
}

func (s *StreamSender) sendMessage(ctx context.Context, chatID, text string) (int64, error) {
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return 0, fmt.Errorf("telegram stream: bot_token required")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telegram stream: bad chat id")
	}
	body, _ := json.Marshal(map[string]any{"chat_id": id, "text": text})
	raw, err := s.post(ctx, token, "sendMessage", body)
	if err != nil {
		return 0, err
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if json.Unmarshal(raw, &out) != nil || !out.OK {
		msg := strings.TrimSpace(out.Description)
		if msg == "" {
			msg = string(raw)
		}
		return 0, fmt.Errorf("telegram stream send: %s", msg)
	}
	return out.Result.MessageID, nil
}

func (s *StreamSender) editMessage(ctx context.Context, chatID string, messageID int64, text string) error {
	token := strings.TrimSpace(s.BotToken)
	id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":    id,
		"message_id": messageID,
		"text":       text,
	})
	raw, err := s.post(ctx, token, "editMessageText", body)
	if err != nil {
		return err
	}
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &out)
	if !out.OK {
		desc := strings.ToLower(out.Description)
		if strings.Contains(desc, "message is not modified") {
			return nil
		}
		return fmt.Errorf("telegram stream edit: %s", out.Description)
	}
	return nil
}

func (s *StreamSender) post(ctx context.Context, token, method string, body []byte) ([]byte, error) {
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	url := "https://api.telegram.org/bot" + token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8192))
}

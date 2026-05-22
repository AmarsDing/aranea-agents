package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultStreamEditInterval = 2 * time.Second

const slackStreamTextLimit = 12000

// StreamSender posts the first reply then edits in place (chat.update).
type StreamSender struct {
	BotToken     string
	HTTP         *http.Client
	EditInterval time.Duration

	mu        sync.Mutex
	channelID string
	messageTS string
	lastEdit  time.Time
}

// ID implements channel.Identified.
func (s *StreamSender) ID() string { return "slack" }

// Update sends or edits the preview message. force bypasses throttle (final flush).
func (s *StreamSender) Update(ctx context.Context, channelID, text string, force bool) error {
	channelID = strings.TrimSpace(channelID)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if channelID == "" {
		return fmt.Errorf("slack stream: channel required")
	}
	if len(text) > slackStreamTextLimit {
		text = text[:slackStreamTextLimit]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageTS == "" || s.channelID != channelID {
		ts, err := s.postMessageLocked(ctx, channelID, text)
		if err != nil {
			return err
		}
		s.channelID = channelID
		s.messageTS = ts
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
	if err := s.updateMessageLocked(ctx, channelID, s.messageTS, text); err != nil {
		return err
	}
	s.lastEdit = time.Now()
	return nil
}

func (s *StreamSender) postMessageLocked(ctx context.Context, channelID, text string) (string, error) {
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return "", fmt.Errorf("slack stream: bot_token required")
	}
	body, _ := json.Marshal(map[string]any{
		"channel": channelID,
		"text":    text,
	})
	raw, err := s.apiPost(ctx, token, "https://slack.com/api/chat.postMessage", body)
	if err != nil {
		return "", err
	}
	var out struct {
		OK    bool   `json:"ok"`
		TS    string `json:"ts"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &out) != nil || !out.OK {
		msg := strings.TrimSpace(out.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return "", fmt.Errorf("slack stream post: %s", msg)
	}
	ts := strings.TrimSpace(out.TS)
	if ts == "" {
		return "", fmt.Errorf("slack stream post: empty ts")
	}
	return ts, nil
}

func (s *StreamSender) updateMessageLocked(ctx context.Context, channelID, ts, text string) error {
	token := strings.TrimSpace(s.BotToken)
	body, _ := json.Marshal(map[string]any{
		"channel": channelID,
		"ts":      ts,
		"text":    text,
	})
	raw, err := s.apiPost(ctx, token, "https://slack.com/api/chat.update", body)
	if err != nil {
		return err
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if !out.OK {
		desc := strings.ToLower(strings.TrimSpace(out.Error))
		if desc == "message_not_changed" {
			return nil
		}
		if out.Error != "" {
			return fmt.Errorf("slack stream update: %s", out.Error)
		}
		return fmt.Errorf("slack stream update failed")
	}
	return nil
}

func (s *StreamSender) apiPost(ctx context.Context, token, url string, body []byte) ([]byte, error) {
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8192))
}

package line

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
)

const defaultLineStreamEditInterval = 2 * time.Second

type StreamSender struct {
	ChannelToken string
	HTTP         *http.Client
	EditInterval time.Duration

	mu        sync.Mutex
	recipient string
	messageID string
	lastEdit  time.Time
}

func (s *StreamSender) ID() string { return "line" }

func (s *StreamSender) PreviewMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageID
}

func (s *StreamSender) Update(ctx context.Context, recipient, text string, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) > 5000 {
		text = text[:5000]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageID == "" || s.recipient != strings.TrimSpace(recipient) {
		id, err := s.pushMessage(ctx, recipient, text)
		if err != nil {
			return err
		}
		s.recipient = strings.TrimSpace(recipient)
		s.messageID = id
		s.lastEdit = time.Now()
		return nil
	}
	interval := s.EditInterval
	if interval <= 0 {
		interval = defaultLineStreamEditInterval
	}
	if !force && time.Since(s.lastEdit) < interval {
		return nil
	}
	// LINE does not support editing sent messages; send a new push message instead.
	id, err := s.pushMessage(ctx, recipient, text)
	if err != nil {
		return err
	}
	s.messageID = id
	s.lastEdit = time.Now()
	return nil
}

func (s *StreamSender) pushMessage(ctx context.Context, recipient, text string) (string, error) {
	token := strings.TrimSpace(s.ChannelToken)
	if token == "" {
		return "", apierror.BadRequest("LINE_CONFIG", "line stream: channel_token required")
	}
	body, _ := marshalMessages(recipient, []map[string]any{textMessage(text)})
	raw, err := doPost(ctx, s.HTTP, token, "https://api.line.me/v2/bot/message/push", body)
	if err != nil {
		return "", err
	}
	var out struct {
		SentMessages []struct {
			ID string `json:"id"`
		} `json:"sentMessages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", apierror.Internal("LINE_PROTOCOL", fmt.Sprintf("line stream: parse response: %s", err.Error()))
	}
	if len(out.SentMessages) > 0 {
		return strings.TrimSpace(out.SentMessages[0].ID), nil
	}
	return "", apierror.Internal("LINE_PROTOCOL", "line stream: push succeeded but no message id returned")
}

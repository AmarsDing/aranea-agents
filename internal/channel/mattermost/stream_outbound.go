package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultMattermostStreamEditInterval = 2 * time.Second

type StreamSender struct {
	ServerURL    string
	BotToken     string
	HTTP         *http.Client
	EditInterval time.Duration

	mu        sync.Mutex
	channelID string
	postID    string
	lastEdit  time.Time
}

func (s *StreamSender) ID() string { return "mattermost" }

func (s *StreamSender) PreviewMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.postID
}

func (s *StreamSender) Update(ctx context.Context, channelID, text string, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.postID == "" || s.channelID != strings.TrimSpace(channelID) {
		id, err := s.createPost(ctx, channelID, text)
		if err != nil {
			return err
		}
		s.channelID = strings.TrimSpace(channelID)
		s.postID = id
		s.lastEdit = time.Now()
		return nil
	}
	interval := s.EditInterval
	if interval <= 0 {
		interval = defaultMattermostStreamEditInterval
	}
	if !force && time.Since(s.lastEdit) < interval {
		return nil
	}
	if err := s.updatePost(ctx, s.postID, text); err != nil {
		return err
	}
	s.lastEdit = time.Now()
	return nil
}

func (s *StreamSender) createPost(ctx context.Context, channelID, text string) (string, error) {
	token := strings.TrimSpace(s.BotToken)
	base := strings.TrimRight(strings.TrimSpace(s.ServerURL), "/")
	if token == "" || base == "" {
		return "", fmt.Errorf("mattermost stream: bot_token and server_url required")
	}
	body, _ := marshalJSON(map[string]any{
		"channel_id": channelID,
		"message":    text,
	})
	raw, err := doPost(ctx, s.HTTP, token, base+"/api/v4/posts", body)
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &out)
	if strings.TrimSpace(out.ID) == "" {
		return "", fmt.Errorf("mattermost stream: empty post id")
	}
	return out.ID, nil
}

func (s *StreamSender) updatePost(ctx context.Context, postID, text string) error {
	token := strings.TrimSpace(s.BotToken)
	base := strings.TrimRight(strings.TrimSpace(s.ServerURL), "/")
	if token == "" || base == "" {
		return fmt.Errorf("mattermost stream: bot_token and server_url required")
	}
	body, _ := marshalJSON(map[string]any{
		"message": text,
	})
	_, err := doPost(ctx, s.HTTP, token, base+"/api/v4/posts/"+postID+"/patch", body)
	return err
}

package wecom

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// InboundMessage is parsed from a WeCom (企业微信) bot / intelligent robot callback.
type InboundMessage struct {
	Text           string
	SenderUserID   string
	ChatID         string
	ResponseURL    string
	MsgType        string
}

// ParseInbound decodes JSON callback bodies (text message).
func ParseInbound(raw []byte) (InboundMessage, error) {
	var body struct {
		MsgType     string `json:"msgtype"`
		MsgTypeAlt  string `json:"MsgType"`
		Text        struct {
			Content string `json:"content"`
		} `json:"text"`
		From struct {
			UserID string `json:"userid"`
		} `json:"from"`
		ChatID      string `json:"chatid"`
		ResponseURL string `json:"response_url"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return InboundMessage{}, err
	}
	msgType := strings.TrimSpace(body.MsgType)
	if msgType == "" {
		msgType = strings.TrimSpace(body.MsgTypeAlt)
	}
	if !strings.EqualFold(msgType, "text") {
		return InboundMessage{}, fmt.Errorf("wecom: unsupported msgtype %q", msgType)
	}
	text := strings.TrimSpace(body.Text.Content)
	if text == "" {
		return InboundMessage{}, fmt.Errorf("wecom: empty text")
	}
	return InboundMessage{
		Text:         text,
		SenderUserID: strings.TrimSpace(body.From.UserID),
		ChatID:       strings.TrimSpace(body.ChatID),
		ResponseURL:  strings.TrimSpace(body.ResponseURL),
		MsgType:      msgType,
	}, nil
}

// SignFor computes WeCom callback msg_signature (sha1 of sorted token+timestamp+nonce).
func SignFor(token, timestamp, nonce string) string {
	parts := []string{strings.TrimSpace(token), strings.TrimSpace(timestamp), strings.TrimSpace(nonce)}
	sort.Strings(parts)
	h := sha1.New()
	_, _ = h.Write([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature validates token+timestamp+nonce signature when token is configured.
func VerifySignature(token, timestamp, nonce, signature string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	want := SignFor(token, timestamp, nonce)
	if !strings.EqualFold(want, strings.TrimSpace(signature)) {
		return fmt.Errorf("wecom: bad signature")
	}
	return nil
}

// TextSender posts markdown/text replies to response_url or webhook_url.
type TextSender struct {
	WebhookURL string
	HTTP       *http.Client
}

// SendText delivers plain text; prefers responseURL from inbound when set.
func (s *TextSender) SendText(ctx context.Context, responseURL, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	target := strings.TrimSpace(responseURL)
	if target == "" {
		target = strings.TrimSpace(s.WebhookURL)
	}
	if target == "" {
		return fmt.Errorf("wecom outbound: webhook url required")
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	})
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("wecom outbound: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

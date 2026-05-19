package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// InboundMessage is parsed from a DingTalk custom robot callback.
type InboundMessage struct {
	Text           string
	SenderNick     string
	SenderStaffID  string
	ConversationID string
	SessionWebhook string
	MsgType        string
}

// ParseInbound decodes a DingTalk robot callback body (text message).
func ParseInbound(raw []byte) (InboundMessage, error) {
	var body struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
		SenderNick     string `json:"senderNick"`
		SenderStaffID  string `json:"senderStaffId"`
		ConversationID string `json:"conversationId"`
		SessionWebhook string `json:"sessionWebhook"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return InboundMessage{}, err
	}
	text := strings.TrimSpace(body.Text.Content)
	if text == "" || strings.ToLower(strings.TrimSpace(body.MsgType)) != "text" {
		return InboundMessage{}, fmt.Errorf("dingtalk: unsupported or empty message")
	}
	return InboundMessage{
		Text:           text,
		SenderNick:     body.SenderNick,
		SenderStaffID:  body.SenderStaffID,
		ConversationID: body.ConversationID,
		SessionWebhook: body.SessionWebhook,
		MsgType:        body.MsgType,
	}, nil
}

// VerifySign validates timestamp + sign query params when secret is configured.
func VerifySign(timestamp, sign, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return fmt.Errorf("dingtalk: bad timestamp")
	}
	now := time.Now().UnixMilli()
	if ts < now-3600_000 || ts > now+3600_000 {
		return fmt.Errorf("dingtalk: timestamp out of range")
	}
	want := signFor(secret, timestamp)
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sign))) {
		return fmt.Errorf("dingtalk: bad sign")
	}
	return nil
}

func signFor(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// TextSender posts replies to DingTalk session webhook or configured outbound URL.
type TextSender struct {
	WebhookURL string
	Secret     string
	HTTP       *http.Client
}

// SendText delivers plain text; prefers sessionWebhook from inbound when set.
func (s *TextSender) SendText(ctx context.Context, sessionWebhook, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	target := strings.TrimSpace(sessionWebhook)
	if target == "" {
		target = strings.TrimSpace(s.WebhookURL)
	}
	if target == "" {
		return fmt.Errorf("dingtalk outbound: webhook url required")
	}
	target, err := signedURL(target, s.Secret)
	if err != nil {
		return err
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
		return fmt.Errorf("dingtalk outbound: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func signedURL(webhookURL, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return webhookURL, nil
	}
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", err
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", signFor(secret, ts))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

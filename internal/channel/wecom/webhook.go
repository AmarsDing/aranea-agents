package wecom

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/channel/port"
)

// InboundMessage is parsed from a WeCom (企业微信) bot / intelligent robot callback.
type InboundMessage struct {
	Text         string
	SenderUserID string
	ChatID       string
	ResponseURL  string
	MsgType      string
	// MsgID is the platform-provided message ID when available.
	// Prefer this over URL timestamps for idempotency keys (COR-05).
	MsgID string
}

// ParseInbound decodes JSON callback bodies (text message).
func ParseInbound(raw []byte) (InboundMessage, error) {
	var body struct {
		MsgType    string `json:"msgtype"`
		MsgTypeAlt string `json:"MsgType"`
		Text       struct {
			Content string `json:"content"`
		} `json:"text"`
		From struct {
			UserID string `json:"userid"`
		} `json:"from"`
		ChatID      string `json:"chatid"`
		ResponseURL string `json:"response_url"`
		MsgID       string `json:"msg_id"`
		MsgIDAlt    string `json:"MsgId"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return InboundMessage{}, err
	}
	msgType := strings.TrimSpace(body.MsgType)
	if msgType == "" {
		msgType = strings.TrimSpace(body.MsgTypeAlt)
	}
	if !strings.EqualFold(msgType, "text") {
		return InboundMessage{}, wecomUnsupportedMsgTypeError(msgType)
	}
	text := strings.TrimSpace(body.Text.Content)
	if text == "" {
		return InboundMessage{}, errEmptyText
	}
	msgID := strings.TrimSpace(body.MsgID)
	if msgID == "" {
		msgID = strings.TrimSpace(body.MsgIDAlt)
	}
	return InboundMessage{
		Text:         text,
		SenderUserID: strings.TrimSpace(body.From.UserID),
		ChatID:       strings.TrimSpace(body.ChatID),
		ResponseURL:  strings.TrimSpace(body.ResponseURL),
		MsgType:      msgType,
		MsgID:        msgID,
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
		return port.ErrCredentialsNotConfigured
	}
	// Validate timestamp freshness (5-minute window)
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return errMissingTimestamp
	}
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadTimestamp
	}
	now := time.Now().Unix()
	if now-tsInt > port.WebhookTimestampToleranceSec || tsInt-now > port.WebhookTimestampToleranceSec {
		return errTimestampOutOfRange
	}
	want := SignFor(token, timestamp, nonce)
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(signature))) {
		return errBadSignature
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
		return errWebhookURLRequired
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
		return wecomAPIError("wecom outbound", fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b))))
	}
	return nil
}

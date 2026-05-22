// Package lark implements Feishu / Lark Open Platform HTTP callbacks.
package lark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

)

const (
	// RegionFeishu is the China Feishu API host.
	RegionFeishu = "feishu"
	// RegionLark is the international Lark API host.
	RegionLark = "lark"
)

// testAPIBase overrides APIBase in tests when non-nil.
var testAPIBase func(string) string

// APIBase returns the REST API origin for the region.
func APIBase(region string) string {
	if testAPIBase != nil {
		return testAPIBase(region)
	}
	switch strings.ToLower(strings.TrimSpace(region)) {
	case RegionLark:
		return "https://open.larksuite.com"
	default:
		return "https://open.feishu.cn"
	}
}

// EventSignature computes X-Lark-Signature (SHA256 hex of timestamp+nonce+encryptKey+rawBody).
func EventSignature(timestamp, nonce, encryptKey string, rawBody []byte) string {
	s := timestamp + nonce + encryptKey + string(rawBody)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// VerifyEventSignature compares the expected signature with X-Lark-Signature header.
func VerifyEventSignature(timestamp, nonce, encryptKey string, rawBody []byte, headerSig string) bool {
	want := strings.TrimSpace(EventSignature(timestamp, nonce, encryptKey, rawBody))
	got := strings.TrimSpace(headerSig)
	if want == "" || got == "" {
		return false
	}
	return want == got
}

// WebhookParseResult is a normalized inbound event (plain JSON only in MVP).
type WebhookParseResult struct {
	IsURLVerification bool
	Challenge         string
	SkipResponse      bool
	EventRaw          json.RawMessage
	EventType         string
	MessageID         string
	ChatID            string
	ChatType          string
	SenderOpenID      string
	SenderUserID      string
	SenderType        string
	MessageType       string
	Mentioned         bool
	Text              string
}

type genericEvent struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	Token     string          `json:"token"`
	Event     json.RawMessage `json:"event"`
	Header    json.RawMessage `json:"header"`
}

type eventHeader struct {
	EventType string `json:"event_type"`
}

type imMessageEvent struct {
	Message struct {
		MessageID   string `json:"message_id"`
		ChatID      string `json:"chat_id"`
		ChatType    string `json:"chat_type"`
		MessageType string `json:"message_type"`
		Content     string `json:"content"`
		Mentions    []struct {
			Key string `json:"key"`
		} `json:"mentions"`
	} `json:"message"`
	Sender struct {
		SenderType string `json:"sender_type"`
		SenderID   struct {
			OpenID string `json:"open_id"`
			UserID string `json:"user_id"`
		} `json:"sender_id"`
	} `json:"sender"`
}

// ParseWebhookPost parses raw POST body. If verificationToken is non-empty, url_verification token must match.
func ParseWebhookPost(raw []byte, verificationToken string) (*WebhookParseResult, error) {
	var top genericEvent
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("lark webhook: invalid json: %w", err)
	}
	if top.Type == "url_verification" {
		if verificationToken != "" && top.Token != verificationToken {
			return nil, fmt.Errorf("lark webhook: verification token mismatch")
		}
		return &WebhookParseResult{IsURLVerification: true, Challenge: top.Challenge}, nil
	}
	res := &WebhookParseResult{}
	if len(top.Header) > 0 {
		var hdr eventHeader
		if err := json.Unmarshal(top.Header, &hdr); err == nil {
			res.EventType = strings.TrimSpace(hdr.EventType)
		}
	}
	switch res.EventType {
	case "im.message.receive_v1":
		var wrap struct {
			Event imMessageEvent `json:"event"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, fmt.Errorf("lark webhook: im.message.receive_v1: %w", err)
		}
		res.MessageID = strings.TrimSpace(wrap.Event.Message.MessageID)
		res.ChatID = strings.TrimSpace(wrap.Event.Message.ChatID)
		res.ChatType = strings.TrimSpace(wrap.Event.Message.ChatType)
		if res.ChatType == "" && res.ChatID != "" {
			res.ChatType = InferChatTypeFromChatID(res.ChatID)
		}
		res.MessageType = strings.TrimSpace(wrap.Event.Message.MessageType)
		res.SenderType = strings.TrimSpace(wrap.Event.Sender.SenderType)
		res.SenderOpenID = strings.TrimSpace(wrap.Event.Sender.SenderID.OpenID)
		res.SenderUserID = strings.TrimSpace(wrap.Event.Sender.SenderID.UserID)
		res.Mentioned = len(wrap.Event.Message.Mentions) > 0
		res.Text = extractTextFromIMContent(strings.TrimSpace(wrap.Event.Message.Content))
	default:
		res.SkipResponse = true
	}
	return res, nil
}

type imTextContent struct {
	Text string `json:"text"`
}

func extractTextFromIMContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var t imTextContent
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return raw
	}
	return strings.TrimSpace(t.Text)
}

// VerifyHTTPRequest checks Feishu signature headers when encryptKey is non-empty.
func VerifyHTTPRequest(r *http.Request, encryptKey string, rawBody []byte) error {
	encryptKey = strings.TrimSpace(encryptKey)
	if encryptKey == "" || r == nil {
		return nil
	}
	ts := strings.TrimSpace(r.Header.Get("X-Lark-Request-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("X-Lark-Request-Nonce"))
	sig := strings.TrimSpace(r.Header.Get("X-Lark-Signature"))
	if ts == "" || nonce == "" || sig == "" {
		return fmt.Errorf("lark webhook: missing signature headers")
	}
	if !VerifyEventSignature(ts, nonce, encryptKey, rawBody, sig) {
		return fmt.Errorf("lark webhook: signature mismatch")
	}
	return nil
}

// ReadBodyDrain reads the body and restores r.Body for downstream use.
func ReadBodyDrain(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, fmt.Errorf("lark webhook: nil request")
	}
	raw, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, err
}

type tenantTokenJSON struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// FetchTenantAccessToken exchanges app_id and app_secret for tenant_access_token.
func FetchTenantAccessToken(ctx context.Context, httpClient *http.Client, region, appID, appSecret string) (token string, ttlSec int, err error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return "", 0, fmt.Errorf("lark: app_id and app_secret required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	u := APIBase(region) + "/open-apis/auth/v3/tenant_access_token/internal"
	body := map[string]string{"app_id": appID, "app_secret": appSecret}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	var out tenantTokenJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", 0, fmt.Errorf("lark token: bad json: %w", err)
	}
	if out.Code != 0 {
		return "", 0, fmt.Errorf("lark token: code=%d msg=%s", out.Code, out.Msg)
	}
	if strings.TrimSpace(out.TenantAccessToken) == "" {
		return "", 0, fmt.Errorf("lark token: empty tenant_access_token")
	}
	return out.TenantAccessToken, out.Expire, nil
}

type sendMsgReq struct {
	ReceiveID string `json:"receive_id"`
	MsgType   string `json:"msg_type"`
	Content   string `json:"content"`
}

// SendTextMessageOpenID posts a text message to a user (receive_id_type=open_id).
func SendTextMessageOpenID(ctx context.Context, httpClient *http.Client, region, tenantToken, openID, text string) error {
	return SendTextMessage(ctx, httpClient, region, tenantToken, openID, ReceiveIDTypeOpenID, text)
}

// SendTextMessage posts a text IM message with the given receive_id_type.
func SendTextMessage(ctx context.Context, httpClient *http.Client, region, tenantToken, receiveID, receiveIDType, text string) error {
	receiveID = strings.TrimSpace(receiveID)
	text = strings.TrimSpace(text)
	if receiveID == "" || text == "" {
		return fmt.Errorf("lark send: receive_id and text required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	switch strings.ToLower(strings.TrimSpace(receiveIDType)) {
	case ReceiveIDTypeUserID, ReceiveIDTypeChatID:
		receiveIDType = strings.ToLower(strings.TrimSpace(receiveIDType))
	default:
		receiveIDType = ReceiveIDTypeOpenID
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	reqBody := sendMsgReq{
		ReceiveID: receiveID,
		MsgType:   "text",
		Content:   string(content),
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	u := APIBase(region) + "/open-apis/im/v1/messages?receive_id_type=" + url.QueryEscape(receiveIDType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tenantToken))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("lark send: bad json: %w", err)
	}
	if out.Code != 0 {
		return fmt.Errorf("lark send: code=%d msg=%s body=%s", out.Code, out.Msg, string(raw))
	}
	return nil
}

// DefaultHTTPClient returns a client suitable for Feishu APIs.
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

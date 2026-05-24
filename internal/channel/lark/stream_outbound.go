package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/channel/preview"
)

const defaultStreamEditInterval = 2 * time.Second

// FeishuStreamTextLimit is the safe rune cap for a single stream PATCH (F-07 guard).
const FeishuStreamTextLimit = 11800

// StreamSender posts the first reply then patches in place (Feishu im.v1 message update).
type StreamSender struct {
	Region         string
	AppID          string
	AppSecret      string
	HTTP           *http.Client
	EditInterval   time.Duration
	ReceiveIDType  string

	mu         sync.Mutex
	receiveID  string
	messageID  string
	lastEdit   time.Time
	tenantTok  string
	tokenUntil time.Time
}

// ID implements channel.Identified.
func (s *StreamSender) ID() string { return "feishu" }

// PreviewMessageID returns the platform message id after the first successful send.
func (s *StreamSender) PreviewMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.messageID)
}

// Update sends or patches the preview message. force bypasses throttle (final flush).
func (s *StreamSender) Update(ctx context.Context, receiveOpenID, text string, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = preview.TruncateRunes(text, FeishuStreamTextLimit)
	receiveOpenID = strings.TrimSpace(receiveOpenID)
	if receiveOpenID == "" {
		return fmt.Errorf("feishu stream: receive_id required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageID == "" || s.receiveID != receiveOpenID {
		id, err := s.sendTextLocked(ctx, receiveOpenID, text)
		if err != nil {
			return err
		}
		s.receiveID = receiveOpenID
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
	if err := s.patchTextLocked(ctx, s.messageID, text); err != nil {
		return err
	}
	s.lastEdit = time.Now()
	return nil
}

func (s *StreamSender) effectiveReceiveIDType() string {
	switch strings.ToLower(strings.TrimSpace(s.ReceiveIDType)) {
	case ReceiveIDTypeUserID, ReceiveIDTypeChatID:
		return strings.ToLower(strings.TrimSpace(s.ReceiveIDType))
	default:
		return ReceiveIDTypeOpenID
	}
}

func (s *StreamSender) tenantTokenLocked(ctx context.Context) (string, error) {
	if s.tenantTok != "" && time.Now().Before(s.tokenUntil.Add(-30*time.Second)) {
		return s.tenantTok, nil
	}
	client := s.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region := strings.TrimSpace(strings.ToLower(s.Region))
	if region == "" {
		region = RegionFeishu
	}
	appID := strings.TrimSpace(s.AppID)
	secret := strings.TrimSpace(s.AppSecret)
	if appID == "" || secret == "" {
		return "", fmt.Errorf("feishu stream: app_id and app_secret required")
	}
	tok, expireSec, err := FetchTenantAccessToken(ctx, client, region, appID, secret)
	if err != nil {
		return "", err
	}
	s.tenantTok = tok
	if expireSec > 60 {
		s.tokenUntil = time.Now().Add(time.Duration(expireSec-60) * time.Second)
	} else {
		s.tokenUntil = time.Now().Add(5 * time.Minute)
	}
	return tok, nil
}

func (s *StreamSender) sendTextLocked(ctx context.Context, openID, text string) (string, error) {
	tok, err := s.tenantTokenLocked(ctx)
	if err != nil {
		return "", err
	}
	client := s.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region := strings.TrimSpace(strings.ToLower(s.Region))
	if region == "" {
		region = RegionFeishu
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	reqBody := sendMsgReq{
		ReceiveID: openID,
		MsgType:   "text",
		Content:   string(content),
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	u := APIBase(region) + "/open-apis/im/v1/messages?receive_id_type=" + url.QueryEscape(s.effectiveReceiveIDType())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return "", err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("feishu stream send: bad json: %w", err)
	}
	if out.Code != 0 {
		return "", fmt.Errorf("feishu stream send: code=%d msg=%s", out.Code, out.Msg)
	}
	id := strings.TrimSpace(out.Data.MessageID)
	if id == "" {
		return "", fmt.Errorf("feishu stream send: empty message_id")
	}
	return id, nil
}

func (s *StreamSender) patchTextLocked(ctx context.Context, messageID, text string) error {
	tok, err := s.tenantTokenLocked(ctx)
	if err != nil {
		return err
	}
	client := s.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region := strings.TrimSpace(strings.ToLower(s.Region))
	if region == "" {
		region = RegionFeishu
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	body, _ := json.Marshal(map[string]string{
		"msg_type": "text",
		"content":  string(content),
	})
	u := APIBase(region) + "/open-apis/im/v1/messages/" + strings.TrimSpace(messageID)
	// Text/post edits use PUT; PATCH on the same path is card-only (Feishu 230001 "NOT a card").
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Code != 0 {
		desc := strings.ToLower(out.Msg)
		if strings.Contains(desc, "not modified") || strings.Contains(desc, "same content") {
			return nil
		}
		return fmt.Errorf("feishu stream edit: code=%d msg=%s", out.Code, out.Msg)
	}
	return nil
}

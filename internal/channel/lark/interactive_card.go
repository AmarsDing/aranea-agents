package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"aranea-agents/internal/channel/port"
)

// SendInteractiveMessage posts an interactive card IM message and returns message_id.
func SendInteractiveMessage(ctx context.Context, httpClient *http.Client, region, tenantToken, receiveID, receiveIDType, cardJSON string) (string, error) {
	receiveID = strings.TrimSpace(receiveID)
	cardJSON = strings.TrimSpace(cardJSON)
	if receiveID == "" || cardJSON == "" {
		return "", errCardOrReceiveIDRequired
	}
	if httpClient == nil {
		httpClient = DefaultHTTPClient()
	}
	switch strings.ToLower(strings.TrimSpace(receiveIDType)) {
	case ReceiveIDTypeUserID, ReceiveIDTypeChatID:
		receiveIDType = strings.ToLower(strings.TrimSpace(receiveIDType))
	default:
		receiveIDType = ReceiveIDTypeOpenID
	}
	reqBody := sendMsgReq{
		ReceiveID: receiveID,
		MsgType:   "interactive",
		Content:   cardJSON,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	u := APIBase(region) + "/open-apis/im/v1/messages?receive_id_type=" + url.QueryEscape(receiveIDType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tenantToken))
	resp, err := httpClient.Do(req)
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
		return "", feishuParseError("lark interactive", err)
	}
	if out.Code != 0 {
		return "", feishuAPIError("lark interactive", out.Code, out.Msg)
	}
	id := strings.TrimSpace(out.Data.MessageID)
	if id == "" {
		return "", errEmptyMessageID
	}
	return id, nil
}

// UpdateInteractiveMessage patches an existing interactive card message.
func UpdateInteractiveMessage(ctx context.Context, httpClient *http.Client, region, tenantToken, messageID, cardJSON string) error {
	messageID = strings.TrimSpace(messageID)
	cardJSON = strings.TrimSpace(cardJSON)
	if messageID == "" || cardJSON == "" {
		return errMessageIDAndCardRequired
	}
	if httpClient == nil {
		httpClient = DefaultHTTPClient()
	}
	region = strings.TrimSpace(strings.ToLower(region))
	if region == "" {
		region = RegionFeishu
	}
	body, err := json.Marshal(map[string]string{
		"msg_type": "interactive",
		"content":  cardJSON,
	})
	if err != nil {
		return err
	}
	u := APIBase(region) + "/open-apis/im/v1/messages/" + url.PathEscape(messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(body))
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
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return feishuParseError("lark interactive update", err)
	}
	if out.Code != 0 {
		desc := strings.ToLower(out.Msg)
		if strings.Contains(desc, "not modified") || strings.Contains(desc, "same content") {
			return nil
		}
		return feishuAPIError("lark interactive update", out.Code, out.Msg)
	}
	return nil
}

// CardSender sends or updates Feishu interactive tool cards.
type CardSender struct {
	Region        string
	AppID         string
	AppSecret     string
	ReceiveIDType string
	HTTP          *http.Client
}

// UpsertToolCard creates a card when messageID is empty, otherwise PATCHes the existing card.
func (s *CardSender) UpsertToolCard(ctx context.Context, recipient, messageID, cardJSON string) (string, error) {
	recipient = strings.TrimSpace(recipient)
	cardJSON = strings.TrimSpace(cardJSON)
	if recipient == "" || cardJSON == "" {
		return "", nil
	}
	client := s.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region := strings.TrimSpace(strings.ToLower(s.Region))
	if region == "" {
		region = RegionFeishu
	}
	tok, _, err := FetchTenantAccessToken(ctx, client, region, strings.TrimSpace(s.AppID), strings.TrimSpace(s.AppSecret))
	if err != nil {
		return "", err
	}
	ridType := ReceiveIDTypeFromMeta(map[string]string{port.MetaReceiveIDType: s.ReceiveIDType})
	if id := strings.TrimSpace(messageID); id != "" {
		if err := UpdateInteractiveMessage(ctx, client, region, tok, id, cardJSON); err != nil {
			return "", err
		}
		return id, nil
	}
	return SendInteractiveMessage(ctx, client, region, tok, recipient, ridType, cardJSON)
}

// SendToolCard posts an interactive card for a completed tool call.
func (s *CardSender) SendToolCard(ctx context.Context, recipient, cardJSON string) error {
	_, err := s.UpsertToolCard(ctx, recipient, "", cardJSON)
	return err
}

package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const defaultProcessingReaction = "THUMBSUP"

// ReactionController adds/removes emoji reactions on inbound messages (F-09).
type ReactionController struct {
	Region    string
	AppID     string
	AppSecret string
	HTTP      *http.Client
	Emoji     string
}

func (r *ReactionController) emojiType() string {
	if e := strings.TrimSpace(r.Emoji); e != "" {
		return e
	}
	return defaultProcessingReaction
}

// Add posts a processing reaction and returns the platform reaction_id for Remove.
func (r *ReactionController) Add(ctx context.Context, messageID string) (reactionID string, err error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", nil
	}
	tok, client, region, err := r.auth(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{
		"reaction_type": map[string]string{"emoji_type": r.emojiType()},
	})
	u := APIBase(region) + "/open-apis/im/v1/messages/" + messageID + "/reactions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ReactionID string `json:"reaction_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", feishuParseError("feishu reaction add", err)
	}
	if out.Code != 0 {
		return "", feishuAPIError("feishu reaction add", out.Code, out.Msg)
	}
	return strings.TrimSpace(out.Data.ReactionID), nil
}

// Remove deletes a reaction previously added via Add.
func (r *ReactionController) Remove(ctx context.Context, messageID, reactionID string) error {
	messageID = strings.TrimSpace(messageID)
	reactionID = strings.TrimSpace(reactionID)
	if messageID == "" || reactionID == "" {
		return nil
	}
	tok, client, region, err := r.auth(ctx)
	if err != nil {
		return err
	}
	u := APIBase(region) + "/open-apis/im/v1/messages/" + messageID + "/reactions/" + reactionID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return feishuParseError("feishu reaction remove", err)
	}
	if out.Code != 0 {
		return feishuAPIError("feishu reaction remove", out.Code, out.Msg)
	}
	return nil
}

func (r *ReactionController) auth(ctx context.Context) (token string, client *http.Client, region string, err error) {
	client = r.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region = strings.TrimSpace(strings.ToLower(r.Region))
	if region == "" {
		region = RegionFeishu
	}
	appID := strings.TrimSpace(r.AppID)
	secret := strings.TrimSpace(r.AppSecret)
	if appID == "" || secret == "" {
		return "", client, region, errAppCredentialsRequired
	}
	tok, _, err := FetchTenantAccessToken(ctx, client, region, appID, secret)
	if err != nil {
		return "", client, region, err
	}
	return tok, client, region, nil
}

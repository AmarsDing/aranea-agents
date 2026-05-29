package wechat

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

func ReplyXML(toUser, fromUser, content string) string {
	return fmt.Sprintf(
		"<xml><ToUserName><![CDATA[%s]]></ToUserName><FromUserName><![CDATA[%s]]></FromUserName><CreateTime>%d</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[%s]]></Content></xml>",
		toUser, fromUser, time.Now().Unix(), content,
	)
}

type TextSender struct {
	AppID     string
	AppSecret string
	HTTP      *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

func (s *TextSender) ID() string { return "wechat" }

func (s *TextSender) SendText(ctx context.Context, openID, text string) error {
	openID = strings.TrimSpace(openID)
	text = strings.TrimSpace(text)
	if openID == "" || text == "" {
		return nil
	}
	token, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"touser":  openID,
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	})
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	url := "https://api.weixin.qq.com/cgi-bin/message/custom/send?access_token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("wechat outbound: parse response: %w", err)
	}
	if out.ErrCode != 0 {
		return fmt.Errorf("wechat outbound: %s", strings.TrimSpace(out.ErrMsg))
	}
	return nil
}

func (s *TextSender) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.exp) {
		return s.token, nil
	}
	appID := strings.TrimSpace(s.AppID)
	secret := strings.TrimSpace(s.AppSecret)
	if appID == "" || secret == "" {
		return "", fmt.Errorf("wechat: app_id and app_secret required")
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		appID, secret,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("wechat token: %s", strings.TrimSpace(out.ErrMsg))
	}
	s.token = out.AccessToken
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 7200 * time.Second
	}
	buffer := 5 * time.Minute
	if ttl <= buffer {
		buffer = ttl / 2
	}
	s.exp = time.Now().Add(ttl - buffer)
	return out.AccessToken, nil
}

func ActiveModeFromConfig(configJSON string) bool {
	var env struct {
		Config struct {
			ActiveMode bool `json:"active_mode"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	return env.Config.ActiveMode
}

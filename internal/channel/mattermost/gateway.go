package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/gorilla/websocket"
)

func init() {
	runtime.RegisterStarterWithLogger("mattermost", "websocket", RunWebSocket)
}

func RunWebSocket(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("Mattermost WebSocket 连接器启动",
		loggateway.StepID("channel.mattermost.ws.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	serverURL, err := lookup(ctx, creds, "server_url")
	if err != nil {
		lg.Error("Mattermost 凭据获取失败",
			loggateway.StepID("channel.mattermost.ws.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		lg.Error("Mattermost 凭据获取失败",
			loggateway.StepID("channel.mattermost.ws.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	serverURL = strings.TrimSpace(serverURL)
	botToken = strings.TrimSpace(botToken)
	if serverURL == "" || botToken == "" {
		return errServerURLAndBotTokenRequired
	}

	botUserID, err := fetchBotUserID(ctx, serverURL, botToken, lg)
	if err != nil {
		return mattermostAPIError("mattermost websocket: get user", err.Error())
	}

	wsURL := buildWSURL(serverURL)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+botToken)

	// guard 收敛读失败与 ping 失败同时触发的重复 error 发射。
	flowGuard := &runtime.ConnectFlowGuard{}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		dialErr := mattermostAPIError("mattermost websocket: dial", err.Error())
		flowGuard.EmitError(func() {
			runtime.EmitConnectError(ctx, "mattermost", ch.ID, "Mattermost WebSocket 连接失败", dialErr)
		})
		return dialErr
	}
	defer conn.Close()
	lg.Info("Mattermost WebSocket 连接成功",
		loggateway.StepID("channel.mattermost.ws.connected"),
		loggateway.Str("channel_id", ch.ID),
		loggateway.Str("server_url", serverURL))
	runtime.EmitConnectOpen(ctx, "mattermost", ch.ID, botUserID, "Mattermost WebSocket 已连接")

	chRow := ch
	readErr := make(chan error, 1)
	safego.Go(ctx, "channel.mattermost.ws.inbound", func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				lg.Warn("Mattermost WebSocket 读取失败",
					loggateway.StepID("channel.mattermost.ws.read_failed"),
					loggateway.Str("channel_id", chRow.ID),
					loggateway.Err(err))
				flowGuard.EmitError(func() {
					runtime.EmitConnectError(ctx, "mattermost", chRow.ID, "Mattermost WebSocket 读取异常", err)
				})
				readErr <- err
				return
			}
			ev, ok, parseFailed := parseWSMessageDetail(message, botUserID)
			if parseFailed {
				lg.Warn("Mattermost WebSocket 消息解析失败",
					loggateway.StepID("channel.mattermost.ws.parse_failed"),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			if !ok {
				continue
			}
			ev.PlatformType = "mattermost"
			if err := handler.ProcessInbound(ctx, chRow, ev); err != nil {
				lg.Warn("Mattermost 入站处理失败",
					loggateway.StepID("channel.mattermost.inbound_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
		}
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return mattermostAPIError("mattermost websocket: read failed", err.Error())
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"seq":0,"action":"ping"}`)); err != nil {
				pingErr := mattermostAPIError("mattermost websocket: ping failed", err.Error())
				flowGuard.EmitError(func() {
					runtime.EmitConnectError(ctx, "mattermost", ch.ID, "Mattermost WebSocket 心跳失败", pingErr)
				})
				return pingErr
			}
		}
	}
}

func fetchBotUserID(ctx context.Context, serverURL, token string, lg loggateway.Logger) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v4/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", mattermostAPIError("mattermost websocket", fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw))))
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &user); err != nil {
		return "", mattermostParseError("mattermost websocket: parse user response", err)
	}
	if strings.TrimSpace(user.ID) == "" {
		return "", mattermostAPIError("mattermost websocket", "empty bot user id")
	}
	return strings.TrimSpace(user.ID), nil
}

func buildWSURL(serverURL string) string {
	u := strings.TrimSpace(serverURL)
	u = strings.TrimRight(u, "/")
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return u + "/api/v4/websocket"
}

func parseWSMessage(raw []byte, botUserID string) (port.InboundEvent, bool) {
	ev, ok, _ := parseWSMessageDetail(raw, botUserID)
	return ev, ok
}

// parseWSMessageDetail 第三返回值 parseFailed 标记协议 JSON 损坏
// （区别于非 posted 事件、bot 消息、空文本等正常业务过滤）。
func parseWSMessageDetail(raw []byte, botUserID string) (port.InboundEvent, bool, bool) {
	var envelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return port.InboundEvent{}, false, true
	}
	if envelope.Event != "posted" {
		return port.InboundEvent{}, false, false
	}
	var data struct {
		Post       string `json:"post"`
		SenderType string `json:"sender_type"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return port.InboundEvent{}, false, true
	}
	if strings.EqualFold(data.SenderType, "bot") {
		return port.InboundEvent{}, false, false
	}
	var post struct {
		Message   string `json:"message"`
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal([]byte(data.Post), &post); err != nil {
		return port.InboundEvent{}, false, true
	}
	text := strings.TrimSpace(post.Message)
	if text == "" {
		return port.InboundEvent{}, false, false
	}
	if strings.TrimSpace(post.UserID) == strings.TrimSpace(botUserID) {
		return port.InboundEvent{}, false, false
	}
	channelID := strings.TrimSpace(post.ChannelID)
	userID := strings.TrimSpace(post.UserID)
	return port.InboundEvent{
		PeerID:         port.FirstNonEmpty(userID, channelID),
		Text:           text,
		IdempotencyKey: "mattermost:" + strings.TrimSpace(post.ID),
		OutboundMeta: map[string]string{
			port.MetaRecipient: channelID,
			port.MetaChatID:    channelID,
		},
	}, true, false
}

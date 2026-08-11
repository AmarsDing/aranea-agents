package wechatilink

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
)

func init() {
	runtime.RegisterStarterWithLogger("wechat_ilink", "polling", RunPolling)
}

type getUpdatesReq struct {
	BaseInfo      baseInfo `json:"base_info"`
	GetUpdatesBuf string   `json:"get_updates_buf"`
}

type getUpdatesResp struct {
	Ret                  int             `json:"ret"`
	Msgs                 []WeixinMessage `json:"msgs"`
	GetUpdatesBuf        string          `json:"get_updates_buf"`
	LongPollingTimeoutMs int             `json:"longpolling_timeout_ms"`
	ErrCode              int             `json:"errcode"`
	ErrMsg               string          `json:"errmsg"`
}

// RunPolling is the runtime starter for wechat_ilink channels. It long-polls
// getupdates and feeds messages into the ingress handler until ctx is cancelled.
func RunPolling(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("WeChat iLink Polling 连接器启动",
		loggateway.StepID("channel.wechat_ilink.polling.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		lg.Error("WeChat iLink 凭据获取失败",
			loggateway.StepID("channel.wechat_ilink.polling.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return fmt.Errorf("wechat_ilink: bot_token not configured")
	}
	baseURL, _ := lookup(ctx, creds, "baseurl") // 可选，默认官方域名

	state, err := readState(ch.ID)
	if err != nil {
		lg.Warn("WeChat iLink 状态文件读取失败，从空状态开始",
			loggateway.StepID("channel.wechat_ilink.polling.state_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		state = &stateFile{ContextTokens: map[string]string{}}
	}

	c := newClient(baseURL, botToken, lg)
	cfg := parseInstanceConfig(ch.ConfigJSON)
	buf := state.GetUpdatesBuf
	consecutiveErrors := 0

	lg.Info("WeChat iLink Polling 已连接",
		loggateway.StepID("channel.wechat_ilink.polling.connected"),
		loggateway.Str("channel_id", ch.ID),
	)
	runtime.EmitConnectOpen(ctx, "wechat_ilink", ch.ID, "iLink", "WeChat iLink Polling 已连接")

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req := getUpdatesReq{
			BaseInfo:      baseInfo{ChannelVersion: channelVersion},
			GetUpdatesBuf: buf,
		}
		resp, httpErr := c.post(ctx, "/ilink/bot/getupdates", req)
		if httpErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			consecutiveErrors++
			backoff := pollingBackoff(consecutiveErrors)
			lg.Warn("getupdates 请求失败",
				loggateway.StepID("channel.wechat_ilink.polling.getupdates_fail"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Int("consecutive", consecutiveErrors),
				loggateway.Err(httpErr),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		r, decodeErr := decodeJSON[getUpdatesResp](resp)
		if decodeErr != nil {
			consecutiveErrors++
			lg.Warn("getupdates 响应解码失败",
				loggateway.StepID("channel.wechat_ilink.polling.decode_fail"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Err(decodeErr),
			)
			if !sleepOrDone(ctx, 2*time.Second) {
				return ctx.Err()
			}
			continue
		}

		if isSessionExpired(r.ErrCode) {
			lg.Error("WeChat iLink 登录会话过期，需重新扫码",
				loggateway.StepID("channel.wechat_ilink.polling.session_expired"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Err(ErrSessionExpired),
			)
			runtime.EmitConnectError(ctx, "wechat_ilink", ch.ID, "微信登录已过期，请重新扫码", ErrSessionExpired)
			state.LoginStatus = "expired"
			if writeErr := writeState(ch.ID, state); writeErr != nil {
				lg.Warn("状态文件写入失败", loggateway.Err(writeErr))
			}
			return ErrSessionExpired
		}

		if r.Ret != 0 || r.ErrCode != 0 {
			consecutiveErrors++
			lg.Warn("getupdates 业务错误",
				loggateway.StepID("channel.wechat_ilink.polling.api_error"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Int("ret", r.Ret),
				loggateway.Int("errcode", r.ErrCode),
				loggateway.Str("errmsg", r.ErrMsg),
			)
			if !sleepOrDone(ctx, 2*time.Second) {
				return ctx.Err()
			}
			continue
		}

		consecutiveErrors = 0
		buf = r.GetUpdatesBuf

		for i := range r.Msgs {
			msg := &r.Msgs[i]
			if msg.MessageType != MessageTypeUser {
				continue // 只处理用户消息，跳过 bot 自己发出的回声
			}
			if msg.ContextToken != "" {
				state.ContextTokens[msg.FromUserID] = msg.ContextToken
				if msg.GroupID != "" {
					// 群聊出站 recipient = GroupID，回退查找按 recipient 进行，
					// 必须同时以 GroupID 为键缓存。
					state.ContextTokens[msg.GroupID] = msg.ContextToken
				}
			}
			if !shouldHandleGroupMessage(msg, cfg) {
				continue
			}
			ev, parseErr := parseMessage(ch.ID, msg)
			if parseErr != nil {
				lg.Warn("WeChat iLink 消息解析失败",
					loggateway.StepID("channel.wechat_ilink.polling.parse_fail"),
					loggateway.Str("channel_id", ch.ID),
					loggateway.Err(parseErr),
				)
				continue
			}
			if handleErr := handler.ProcessInbound(ctx, ch, *ev); handleErr != nil {
				lg.Warn("WeChat iLink 入站处理失败",
					loggateway.StepID("channel.wechat_ilink.inbound_failed"),
					loggateway.Str("channel_id", ch.ID),
					loggateway.Err(handleErr),
				)
			}
		}

		state.GetUpdatesBuf = buf
		state.LoginStatus = "active"
		if writeErr := writeState(ch.ID, state); writeErr != nil {
			lg.Warn("状态文件写入失败",
				loggateway.StepID("channel.wechat_ilink.polling.state_write_fail"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Err(writeErr),
			)
		}
	}
}

func pollingBackoff(consecutive int) time.Duration {
	switch {
	case consecutive <= 2:
		return 2 * time.Second
	case consecutive <= 5:
		return 5 * time.Second
	default:
		return 30 * time.Second
	}
}

// sleepOrDone returns false when ctx is done during the wait.
func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

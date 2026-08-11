package wechatilink

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/channel/port"
)

// instanceConfig mirrors the non-sensitive config_json.config section.
type instanceConfig struct {
	Config struct {
		GroupEnabled   *bool  `json:"group_enabled"`
		RequireMention *bool  `json:"require_mention"`
		BotNickname    string `json:"bot_nickname"`
	} `json:"config"`
}

func parseInstanceConfig(configJSON string) instanceConfig {
	var cfg instanceConfig
	if strings.TrimSpace(configJSON) == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(configJSON), &cfg)
	return cfg
}

func (c instanceConfig) groupEnabled() bool {
	return c.Config.GroupEnabled != nil && *c.Config.GroupEnabled
}

func (c instanceConfig) requireMention() bool {
	// 默认 true：群内只在被 @ 时响应，避免 bot 在群里过度发言
	return c.Config.RequireMention == nil || *c.Config.RequireMention
}

// parseMessage converts an inbound iLink message into a transport-neutral event.
func parseMessage(channelID string, msg *WeixinMessage) (*port.InboundEvent, error) {
	if len(msg.ItemList) == 0 {
		return nil, fmt.Errorf("wechat_ilink: empty item list")
	}
	item := msg.ItemList[0]
	var text string
	switch item.Type {
	case ItemTypeText:
		if item.TextItem != nil {
			text = item.TextItem.Text
		}
	case ItemTypeImage:
		text = "[图片]"
	case ItemTypeVoice:
		if item.VoiceItem != nil && strings.TrimSpace(item.VoiceItem.Text) != "" {
			text = item.VoiceItem.Text // 微信服务端语音识别结果
		} else {
			text = "[语音消息，未识别]"
		}
	case ItemTypeFile:
		name := ""
		if item.FileItem != nil {
			name = item.FileItem.FileName
		}
		text = fmt.Sprintf("[文件: %s]", name)
	case ItemTypeVideo:
		text = "[视频]"
	default:
		text = "[未知消息]"
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("wechat_ilink: empty message text")
	}

	peerID := msg.FromUserID
	// 回复目标：私聊回用户；群聊回群（to_user_id 传 group_id，联调实测验证）
	recipient := msg.FromUserID
	if msg.GroupID != "" {
		recipient = msg.GroupID
	}

	return &port.InboundEvent{
		PlatformType:   "wechat_ilink",
		PeerID:         peerID,
		Text:           text,
		IdempotencyKey: fmt.Sprintf("wechat_ilink:%s:%d", channelID, msg.MessageID),
		OutboundMeta: map[string]string{
			port.MetaRecipient:    recipient,
			port.MetaContextToken: msg.ContextToken,
			port.MetaSessionID:    msg.SessionID,
		},
	}, nil
}

// shouldHandleGroupMessage applies the group gating policy from config_json.
func shouldHandleGroupMessage(msg *WeixinMessage, cfg instanceConfig) bool {
	if msg.GroupID == "" {
		return true // 私聊直通
	}
	if !cfg.groupEnabled() {
		return false
	}
	if !cfg.requireMention() {
		return true
	}
	nick := strings.TrimSpace(cfg.Config.BotNickname)
	if nick == "" {
		return true // 未配置昵称时无法检测 @，放行由实测再收紧
	}
	for _, item := range msg.ItemList {
		if item.Type == ItemTypeText && item.TextItem != nil {
			return strings.Contains(item.TextItem.Text, "@"+nick)
		}
	}
	return false
}

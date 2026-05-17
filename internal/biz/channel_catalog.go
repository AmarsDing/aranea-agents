package biz

var channelCatalog = []ChannelCatalogItem{
	channelCatalogItem("qq", "QQ (NapCat)", "国内", "websocket", "QQ 个人号，NapCat OneBot11 协议", 10, true),
	channelCatalogItem("qqbot", "QQ 官方机器人", "国内", "webhook", "QQ 开放平台机器人", 20, true),
	feishuCatalogItem(),
	channelCatalogItem("dingtalk", "钉钉", "办公协作", "webhook", "钉钉机器人与事件回调", 40, true),
	channelCatalogItem("wecom", "企业微信智能机器人", "办公协作", "webhook", "群机器人或智能机器人", 50, true),
	channelCatalogItem("wecom-app", "企业微信自建应用", "办公协作", "webhook", "企业微信自建应用通道", 60, true),
	channelCatalogItem("openclaw-weixin", "微信（ClawBot）", "国内", "qrcode", "腾讯官方 WeChat ClawBot", 70, true),
	channelCatalogItem("wechat", "微信开放平台", "国内", "webhook", "公众号、小程序、企业微信", 80, true),
	channelCatalogItem("telegram", "Telegram", "海外", "webhook", "Bot API 接入", 90, true),
	channelCatalogItem("whatsapp", "WhatsApp", "海外", "webhook", "Meta Cloud API 或 BSP", 100, true),
	channelCatalogItem("facebook", "Facebook Messenger", "海外", "webhook", "Page Webhook 与 Messenger", 110, false),
	channelCatalogItem("discord", "Discord", "海外", "gateway", "Bot Gateway / Interaction", 120, true),
	channelCatalogItem("slack", "Slack", "办公协作", "event", "Slack App Events / Bot", 130, true),
	channelCatalogItem("msteams", "Microsoft Teams", "办公协作", "webhook", "Teams Bot / Graph", 140, false),
	channelCatalogItem("googlechat", "Google Chat", "办公协作", "webhook", "Google Chat App", 150, false),
	channelCatalogItem("line", "LINE", "海外", "webhook", "LINE Messaging API", 160, false),
	channelCatalogItem("matrix", "Matrix", "海外", "sync", "Matrix Bot", 170, false),
	channelCatalogItem("mattermost", "Mattermost", "办公协作", "websocket", "Mattermost Bot", 180, false),
	channelCatalogItem("signal", "Signal", "海外", "daemon", "Signal CLI/daemon", 190, false),
	channelCatalogItem("zalo", "Zalo", "海外", "webhook", "Zalo Official Account", 200, false),
	channelCatalogItem("zalouser", "Zalo User", "海外", "polling", "Zalo 用户侧通道", 210, false),
	channelCatalogItem("imessage", "iMessage", "海外", "bridge", "BlueBubbles / macOS 桥接", 220, false),
	channelCatalogItem("bluebubbles", "BlueBubbles", "海外", "bridge", "iMessage Android/Server 桥接", 230, false),
	channelCatalogItem("nextcloud-talk", "Nextcloud Talk", "办公协作", "polling", "Nextcloud Talk Bot", 240, false),
	channelCatalogItem("synology-chat", "Synology Chat", "办公协作", "webhook", "Synology Chat Bot", 250, false),
	channelCatalogItem("irc", "IRC", "海外", "socket", "IRC Bot", 260, false),
	channelCatalogItem("nostr", "Nostr", "海外", "relay", "Nostr relay 订阅", 270, false),
	channelCatalogItem("twitch", "Twitch", "海外", "eventsub", "Twitch Chat / EventSub", 280, false),
	channelCatalogItem("tlon", "Tlon", "海外", "plugin", "Tlon / Urbit 集成", 290, false),
	channelCatalogItem("voice-call", "Voice Call", "语音", "webhook", "Twilio / Telnyx / Plivo / mock", 300, false),
	channelCatalogItem("qa-channel", "QA Channel", "测试", "plugin", "QA 与回归测试通道", 310, true),
}

func channelCatalogItem(channelType, label, group, receiveMode, description string, sortOrder int, supportsTest bool) ChannelCatalogItem {
	return ChannelCatalogItem{
		Type:            channelType,
		Label:           label,
		Group:           group,
		Description:     description,
		ReceiveModes:    []string{receiveMode},
		Icon:            channelType,
		Bundled:         true,
		SupportsTest:    supportsTest,
		SupportsWebhook: receiveMode == "webhook" || receiveMode == "event" || receiveMode == "eventsub",
		ConfigSchema: map[string]any{
			"type":        "object",
			"description": "Non-sensitive channel configuration",
		},
		CredentialSchema: map[string]any{
			"type":     "object",
			"required": requiredCredentials(channelType),
		},
		UIHints: map[string]any{
			"group":        group,
			"receive_mode": receiveMode,
		},
		SortOrder: sortOrder,
	}
}

func feishuCatalogItem() ChannelCatalogItem {
	return ChannelCatalogItem{
		Type:            "feishu",
		Label:           "飞书 / Lark",
		Group:           "办公协作",
		Description:     "事件订阅、机器人消息、多账号",
		ReceiveModes:    []string{"webhook"},
		Icon:            "feishu",
		Bundled:         true,
		SupportsTest:    true,
		SupportsWebhook: true,
		ConfigSchema: map[string]any{
			"type":        "object",
			"description": "Non-sensitive Feishu / Lark configuration",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"const":       "feishu",
					"description": "platform type",
				},
				"receive_mode": map[string]any{"type": "string"},
				"webhook": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
				"routing": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"default_agent_id": map[string]any{"type": "string"},
						"default_team_id":  map[string]any{"type": "string"},
						"dm_scope": map[string]any{
							"type": "string",
							"enum": []string{"main", "per-peer", "per-channel-peer"},
						},
					},
				},
				"config": map[string]any{
					"type":     "object",
					"required": []string{"app_id"},
					"properties": map[string]any{
						"app_id": map[string]any{"type": "string", "description": "Feishu app id"},
						"region": map[string]any{
							"type":        "string",
							"description": "feishu (China) or lark (international)",
							"enum":        []string{"feishu", "lark"},
						},
					},
				},
			},
		},
		CredentialSchema: map[string]any{
			"type":     "object",
			"required": requiredCredentials("feishu"),
			"properties": map[string]any{
				"app_secret": map[string]any{"type": "string", "description": "App secret"},
				"encrypt_key": map[string]any{
					"type":        "string",
					"description": "Event subscription Encrypt Key (for signature verification)",
				},
				"verification_token": map[string]any{
					"type":        "string",
					"description": "URL verification token from Feishu console",
				},
			},
		},
		UIHints: map[string]any{
			"group":        "办公协作",
			"receive_mode": "webhook",
		},
		SortOrder: 30,
	}
}

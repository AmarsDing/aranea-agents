package biz

func channelTypeItem(channelType, label, group, receiveMode, description string, sortOrder int, bundled, supportsTest, supportsWebhook bool) ChannelTypeItem {
	modes := []string{receiveMode}
	return ChannelTypeItem{
		Type:            channelType,
		Label:           label,
		Group:           group,
		Description:     description,
		ReceiveModes:    modes,
		Icon:            channelType,
		Bundled:         bundled,
		SupportsTest:    supportsTest,
		SupportsWebhook: supportsWebhook,
		ConfigSchema: map[string]any{
			"type":        "object",
			"description": "Non-sensitive channel configuration",
		},
		CredentialSchema: nil, // built by channelTypeRegistry.Register
		UIHints: map[string]any{
			"group":        group,
			"receive_mode": receiveMode,
		},
		SortOrder: sortOrder,
	}
}

func feishuTypeItem() ChannelTypeItem {
	item := channelTypeItem("feishu", "飞书 / Lark", "办公协作", "websocket", "larkws 长连接（MuseBot 默认）；Webhook 暂未开放", 10, true, true, false)
	item.ReceiveModes = []string{"websocket"}
	item.ConfigSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"app_id":          map[string]any{"type": "string"},
					"region":          map[string]any{"type": "string", "enum": []string{"feishu", "lark"}},
					"connection_mode": map[string]any{"type": "string", "enum": []string{"websocket"}},
					"require_mention": map[string]any{"type": "boolean"},
				},
			},
		},
	}
	return item
}

func dingtalkTypeItem() ChannelTypeItem {
	item := channelTypeItem("dingtalk", "钉钉", "办公协作", "webhook", "机器人 Webhook 或 Stream 长连接（MuseBot ding.go）", 20, true, false, true)
	item.ReceiveModes = []string{"webhook", "stream"}
	return item
}

func wecomTypeItem(channelType, label, description string, sortOrder int) ChannelTypeItem {
	return channelTypeItem(channelType, label, "办公协作", "webhook", description+"；MuseBot comwechat.go", sortOrder, true, false, true)
}

func wechatTypeItem() ChannelTypeItem {
	return channelTypeItem("wechat", "微信公众号", "国内", "webhook", "被动回复或客服 API；MuseBot wechat.go", 50, true, false, true)
}

func slackTypeItem() ChannelTypeItem {
	item := channelTypeItem("slack", "Slack", "办公协作", "event", "Events API 或 Socket Mode；MuseBot slack.go", 60, true, true, true)
	item.ReceiveModes = []string{"event", "socket_mode"}
	return item
}

func telegramTypeItem() ChannelTypeItem {
	item := channelTypeItem("telegram", "Telegram", "海外", "webhook", "Webhook 或 Long Polling；MuseBot telegram.go", 70, true, true, true)
	item.ReceiveModes = []string{"webhook", "polling"}
	return item
}

func discordTypeItem() ChannelTypeItem {
	item := channelTypeItem("discord", "Discord", "海外", "gateway", "Gateway WebSocket；MuseBot discord.go", 80, true, false, false)
	item.SupportsWebhook = false
	return item
}

func qqTypeItem() ChannelTypeItem {
	return channelTypeItem("qq", "QQ 官方机器人", "国内", "webhook", "Webhook + botgo 事件；MuseBot qq.go", 90, true, true, true)
}

func personalQQTypeItem() ChannelTypeItem {
	item := channelTypeItem("personal_qq", "QQ（OneBot）", "国内", "onebot", "NapCat/LLOneBot HTTP 推送；MuseBot personalqq.go", 100, true, false, true)
	item.ReceiveModes = []string{"onebot"}
	return item
}

func lineTypeItem() ChannelTypeItem {
	item := channelTypeItem("line", "LINE", "海外", "webhook", "LINE Messaging API Webhook；line-bot-sdk-go", 75, true, true, true)
	item.ReceiveModes = []string{"webhook"}
	return item
}

func mattermostTypeItem() ChannelTypeItem {
	item := channelTypeItem("mattermost", "Mattermost", "开源协作", "websocket", "Mattermost WebSocket + REST API v4；自部署", 85, true, true, true)
	item.ReceiveModes = []string{"webhook", "websocket"}
	return item
}

func teamsTypeItem() ChannelTypeItem {
	item := channelTypeItem("teams", "Microsoft Teams", "办公协作", "webhook", "Bot Framework Webhook；OAuth2 client_credentials", 55, true, false, true)
	item.ReceiveModes = []string{"webhook"}
	return item
}

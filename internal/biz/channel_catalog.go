package biz

// channelCatalog aligns with MuseBot-supported messaging platforms (robot/*.go).
// bundled=true means adapter exists in this binary; false = spec only (see docs/需求/17 channel.md).
var channelCatalog = []ChannelCatalogItem{
	feishuCatalogItem(),
	dingtalkCatalogItem(),
	wecomCatalogItem("wecom", "企业微信智能机器人", "群机器人或智能机器人", 30),
	wecomCatalogItem("wecom-app", "企业微信自建应用", "企业微信自建应用", 40),
	wechatCatalogItem(),
	slackCatalogItem(),
	telegramCatalogItem(),
	discordCatalogItem(),
	lineCatalogItem(),
	mattermostCatalogItem(),
	teamsCatalogItem(),
	qqCatalogItem(),
	personalQQCatalogItem(),
}

func channelCatalogItem(channelType, label, group, receiveMode, description string, sortOrder int, bundled, supportsTest, supportsWebhook bool) ChannelCatalogItem {
	modes := []string{receiveMode}
	return ChannelCatalogItem{
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
		CredentialSchema: credentialSchemaFor(channelType),
		UIHints: map[string]any{
			"group":        group,
			"receive_mode": receiveMode,
		},
		SortOrder: sortOrder,
	}
}

func feishuCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("feishu", "飞书 / Lark", "办公协作", "websocket", "larkws 长连接（MuseBot 默认）；Webhook 暂未开放", 10, true, true, false)
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

func dingtalkCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("dingtalk", "钉钉", "办公协作", "webhook", "机器人 Webhook 或 Stream 长连接（MuseBot ding.go）", 20, true, false, true)
	item.ReceiveModes = []string{"webhook", "stream"}
	return item
}

func wecomCatalogItem(channelType, label, description string, sortOrder int) ChannelCatalogItem {
	return channelCatalogItem(channelType, label, "办公协作", "webhook", description+"；MuseBot comwechat.go", sortOrder, true, false, true)
}

func wechatCatalogItem() ChannelCatalogItem {
	return channelCatalogItem("wechat", "微信公众号", "国内", "webhook", "被动回复或客服 API；MuseBot wechat.go", 50, true, false, true)
}

func slackCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("slack", "Slack", "办公协作", "event", "Events API 或 Socket Mode；MuseBot slack.go", 60, true, true, true)
	item.ReceiveModes = []string{"event", "socket_mode"}
	return item
}

func telegramCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("telegram", "Telegram", "海外", "webhook", "Webhook 或 Long Polling；MuseBot telegram.go", 70, true, true, true)
	item.ReceiveModes = []string{"webhook", "polling"}
	return item
}

func discordCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("discord", "Discord", "海外", "gateway", "Gateway WebSocket；MuseBot discord.go", 80, true, false, false)
	item.SupportsWebhook = false
	return item
}

func qqCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("qq", "QQ 官方机器人", "国内", "webhook", "Webhook + botgo 事件；MuseBot qq.go", 90, true, true, true)
	item.CredentialSchema = credentialSchemaFor("qq")
	return item
}

func personalQQCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("personal_qq", "QQ（OneBot）", "国内", "onebot", "NapCat/LLOneBot HTTP 推送；MuseBot personalqq.go", 100, true, false, true)
	item.ReceiveModes = []string{"onebot"}
	return item
}

func lineCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("line", "LINE", "海外", "webhook", "LINE Messaging API Webhook；line-bot-sdk-go", 75, true, true, true)
	item.ReceiveModes = []string{"webhook"}
	return item
}

func mattermostCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("mattermost", "Mattermost", "开源协作", "websocket", "Mattermost WebSocket + REST API v4；自部署", 85, true, true, true)
	item.ReceiveModes = []string{"webhook", "websocket"}
	return item
}

func teamsCatalogItem() ChannelCatalogItem {
	item := channelCatalogItem("teams", "Microsoft Teams", "办公协作", "webhook", "Bot Framework Webhook；OAuth2 client_credentials", 55, true, false, true)
	item.ReceiveModes = []string{"webhook"}
	return item
}

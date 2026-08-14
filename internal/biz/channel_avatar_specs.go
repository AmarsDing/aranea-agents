package biz

import "strings"

// ChannelPlatformAvatarSpec describes one built-in channel platform icon in avatar_assets.
type ChannelPlatformAvatarSpec struct {
	ChannelType string
	AssetKey    string
	Name        string
	Label       string
	// Background RGB for icon tile when compositing Simple Icons glyph.
	R, G, B   uint8
	SortOrder int
}

// ChannelPlatformAvatarSpecs returns all catalog platform icons (keys channel_<type>).
func ChannelPlatformAvatarSpecs() []ChannelPlatformAvatarSpec {
	raw := []struct {
		typ, name, label string
		r, g, b          uint8
		sort             int
	}{
		{"qq", "QQ", "Q", 18, 183, 245, 10},
		{"qqbot", "QQ 机器人", "Q", 18, 183, 245, 20},
		{"feishu", "飞书 / Lark", "飞", 51, 112, 255, 30},
		{"dingtalk", "钉钉", "钉", 0, 137, 255, 40},
		{"wecom", "企业微信", "企", 38, 126, 240, 50},
		{"wecom-app", "企微应用", "应", 38, 126, 240, 60},
		{"openclaw-weixin", "微信 ClawBot", "微", 7, 193, 96, 70},
		{"wechat", "微信开放平台", "微", 7, 193, 96, 80},
		{"wechat_ilink", "微信（个人号·iLink）", "微", 7, 193, 96, 85},
		{"telegram", "Telegram", "T", 38, 165, 228, 90},
		{"whatsapp", "WhatsApp", "W", 37, 211, 102, 100},
		{"facebook", "Messenger", "M", 0, 132, 255, 110},
		{"discord", "Discord", "D", 88, 101, 242, 120},
		{"slack", "Slack", "S", 74, 21, 75, 130},
		{"msteams", "Teams", "T", 98, 100, 167, 140},
		{"googlechat", "Google Chat", "G", 66, 133, 244, 150},
		{"line", "LINE", "L", 0, 195, 0, 160},
		{"matrix", "Matrix", "X", 0, 0, 0, 170},
		{"mattermost", "Mattermost", "M", 31, 85, 139, 180},
		{"signal", "Signal", "S", 59, 130, 246, 190},
		{"zalo", "Zalo", "Z", 0, 104, 255, 200},
		{"zalouser", "Zalo User", "Z", 0, 104, 255, 210},
		{"imessage", "iMessage", "i", 52, 199, 89, 220},
		{"bluebubbles", "BlueBubbles", "B", 10, 132, 255, 230},
		{"nextcloud-talk", "Nextcloud", "N", 0, 130, 201, 240},
		{"synology-chat", "Synology", "S", 36, 164, 92, 250},
		{"irc", "IRC", "#", 85, 85, 85, 260},
		{"nostr", "Nostr", "N", 147, 51, 234, 270},
		{"twitch", "Twitch", "Tw", 145, 70, 255, 280},
		{"tlon", "Tlon", "T", 0, 0, 0, 290},
		{"voice-call", "Voice", "V", 233, 169, 32, 300},
		{"qa-channel", "QA", "QA", 107, 114, 128, 310},
	}
	out := make([]ChannelPlatformAvatarSpec, 0, len(raw))
	for _, item := range raw {
		out = append(out, ChannelPlatformAvatarSpec{
			ChannelType: item.typ,
			AssetKey:    "channel_" + channelTypeToAssetKeySuffix(item.typ),
			Name:        item.name,
			Label:       item.label,
			R:           item.r,
			G:           item.g,
			B:           item.b,
			SortOrder:   item.sort,
		})
	}
	return out
}

func channelTypeToAssetKeySuffix(channelType string) string {
	return strings.ReplaceAll(channelType, "-", "_")
}

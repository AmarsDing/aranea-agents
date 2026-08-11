// Package wechatilink implements the WeChat personal-account channel via
// Tencent's official iLink Bot API (ilinkai.weixin.qq.com).
//
// Protocol reference: https://ilinkai.weixin.qq.com (OpenClaw ClawBot plugin)
// Receive mode: long-polling getupdates; send via sendmessage with context_token.
package wechatilink

// channelVersion is sent in base_info of every request body.
const channelVersion = "2.1.1"

type baseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

// CDNMedia references encrypted media stored on the WeChat CDN.
type CDNMedia struct {
	FullURL       string `json:"full_url,omitempty"`
	Key           string `json:"key,omitempty"`
	Host          string `json:"host,omitempty"`
	EncryptionKey string `json:"encryption_key,omitempty"`
}

type TextItem struct {
	Text string `json:"text"`
}

type ImageItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
}

type VoiceItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
	Text     string   `json:"text,omitempty"` // server-side transcription, may be empty
}

type FileItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
	FileName string   `json:"file_name,omitempty"`
}

type VideoItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
}

// MessageItem type values.
const (
	ItemTypeText  = 1
	ItemTypeImage = 2
	ItemTypeVoice = 3
	ItemTypeFile  = 4
	ItemTypeVideo = 5
)

// Message type/state values.
const (
	MessageTypeUser = 1
	MessageTypeBot  = 2

	MessageStateNew        = 0
	MessageStateGenerating = 1
	MessageStateFinish     = 2
)

type MessageItem struct {
	Type      int        `json:"type"`
	TextItem  *TextItem  `json:"text_item,omitempty"`
	ImageItem *ImageItem `json:"image_item,omitempty"`
	VoiceItem *VoiceItem `json:"voice_item,omitempty"`
	FileItem  *FileItem  `json:"file_item,omitempty"`
	VideoItem *VideoItem `json:"video_item,omitempty"`
}

// WeixinMessage is an inbound message delivered by getupdates.
type WeixinMessage struct {
	Seq          int64         `json:"seq"`
	MessageID    int64         `json:"message_id"`
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id"`
	CreateTimeMs int64         `json:"create_time_ms"`
	SessionID    string        `json:"session_id"`
	GroupID      string        `json:"group_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ItemList     []MessageItem `json:"item_list"`
	ContextToken string        `json:"context_token"`
}

// WeixinSendMessage is the outbound message payload of sendmessage.
type WeixinSendMessage struct {
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id,omitempty"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token,omitempty"`
	ItemList     []MessageItem `json:"item_list"`
}

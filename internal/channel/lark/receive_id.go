package lark

import "strings"

const (
	ReceiveIDTypeOpenID = "open_id"
	ReceiveIDTypeUserID = "user_id"
	ReceiveIDTypeChatID = "chat_id"
)

// ResolveReceiveTarget picks Feishu IM receive_id and receive_id_type for outbound/stream sends.
// Prefer chat_id when present (MuseBot SendMsg uses ReceiveIdTypeChatId for both p2p and group).
func ResolveReceiveTarget(openID, userID, chatID string) (receiveID, receiveIDType string) {
	openID = strings.TrimSpace(openID)
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	if chatID != "" {
		return chatID, ReceiveIDTypeChatID
	}
	switch {
	case openID != "":
		return openID, ReceiveIDTypeOpenID
	case userID != "":
		return userID, ReceiveIDTypeUserID
	default:
		return "", ReceiveIDTypeOpenID
	}
}

// ReceiveIDTypeFromMeta reads receive_id_type from OutboundMeta (defaults to open_id).
func ReceiveIDTypeFromMeta(meta map[string]string) string {
	if meta == nil {
		return ReceiveIDTypeOpenID
	}
	switch strings.ToLower(strings.TrimSpace(meta["receive_id_type"])) {
	case ReceiveIDTypeUserID:
		return ReceiveIDTypeUserID
	case ReceiveIDTypeChatID:
		return ReceiveIDTypeChatID
	default:
		return ReceiveIDTypeOpenID
	}
}

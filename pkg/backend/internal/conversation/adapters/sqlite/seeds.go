package sqlite

import (
	"database/sql"

	"arenea/backend/internal/domain"
)

// SeedChatOptions 插入默认对话模式与模型提供方项，供聊天界面在零配置时即可渲染。
func SeedChatOptions(db *sql.DB) error {
	rows := []domain.ChatOption{
		{Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 10},
		{Type: "dialog_mode", Key: "plan", Label: "深思考", Enabled: true, SortOrder: 20},
		{Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 30},
		{Type: "model_provider", Key: "openai", Label: "OpenAI 兼容", Enabled: true, SortOrder: 10},
		{Type: "model_provider", Key: "anthropic", Label: "Anthropic", Enabled: true, SortOrder: 20},
		{Type: "model_provider", Key: "self", Label: "自托管", Enabled: true, SortOrder: 30},
	}
	for _, row := range rows {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO chat_options(type, key, label, enabled, sort_order, metadata_json) VALUES (?, ?, ?, ?, ?, ?)`,
			row.Type, row.Key, row.Label, row.Enabled, row.SortOrder, row.MetadataJSON,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

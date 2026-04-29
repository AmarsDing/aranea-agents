package sqlite

import (
	"database/sql"
	"errors"

	"arenea/backend/internal/domain"
)

// AddMessage 插入消息并递增 sessions.message_count（事务内）。
func (s *Store) AddMessage(m domain.Message) (domain.Message, error) {
	if m.ID == "" || m.SessionID == "" || m.Role == "" || m.Content == "" {
		return domain.Message{}, errors.New("missing required fields")
	}
	if m.Status == "" {
		m.Status = "ok"
	}
	if m.TurnIndex <= 0 {
		next, err := s.nextTurnIndex(m.SessionID)
		if err != nil {
			return domain.Message{}, err
		}
		m.TurnIndex = next
	}
	m.CreatedAt = nowISO()
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Message{}, err
	}
	defer tx.Rollback()
	if err = addMessageTx(tx, m); err != nil {
		return domain.Message{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Message{}, err
	}
	return m, nil
}

func (s *Store) nextTurnIndex(sessionID string) (int, error) {
	var next sql.NullInt64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(turn_index), 0) + 1 FROM messages WHERE session_id = ?`, sessionID).Scan(&next)
	if err != nil {
		return 0, err
	}
	return int(next.Int64), nil
}

// ListMessages 按 turn_index/created_at 顺序列出会话内消息。
func (s *Store) ListMessages(sessionID string) ([]domain.Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, parent_message_id, turn_index, role, content_markdown, COALESCE(model_name, ''), token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at
		 FROM messages WHERE session_id = ? ORDER BY turn_index ASC, created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Message
	for rows.Next() {
		var v domain.Message
		if err = rows.Scan(&v.ID, &v.SessionID, &v.ParentMessageID, &v.TurnIndex, &v.Role, &v.Content, &v.ModelName, &v.TokenIn, &v.TokenOut, &v.LatencyMS, &v.Status, &v.AttachmentsCount, &v.OptionsJSON, &v.ErrorMessage, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func addMessageTx(tx *sql.Tx, m domain.Message) error {
	if _, err := tx.Exec(
		`INSERT INTO messages(id, session_id, parent_message_id, turn_index, role, content_markdown, model_name, token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.ParentMessageID, m.TurnIndex, m.Role, m.Content, m.ModelName, m.TokenIn, m.TokenOut, m.LatencyMS, m.Status, m.AttachmentsCount, m.OptionsJSON, m.ErrorMessage, m.CreatedAt,
	); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE sessions SET message_count = message_count + 1, last_message_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, m.CreatedAt, m.CreatedAt, m.SessionID)
	return err
}

// ListLatestMessagesByTokens 为 L0 滑动窗口从 messages 表取时间逆序后截断的 suffix（返回时间正序）。
func (s *Store) ListLatestMessagesByTokens(sessionID string, maxTokens int, hardCap int) ([]domain.Message, error) {
	if hardCap <= 0 {
		hardCap = 200
	}
	rows, err := s.db.Query(
		`SELECT id, session_id, parent_message_id, turn_index, role, content_markdown, COALESCE(model_name, ''), token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at
		 FROM messages WHERE session_id = ? ORDER BY turn_index DESC, created_at DESC LIMIT ?`,
		sessionID, hardCap,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collected := make([]domain.Message, 0, hardCap)
	totalTokens := 0
	for rows.Next() {
		var v domain.Message
		if err = rows.Scan(&v.ID, &v.SessionID, &v.ParentMessageID, &v.TurnIndex, &v.Role, &v.Content, &v.ModelName, &v.TokenIn, &v.TokenOut, &v.LatencyMS, &v.Status, &v.AttachmentsCount, &v.OptionsJSON, &v.ErrorMessage, &v.CreatedAt); err != nil {
			return nil, err
		}
		tokens := v.TokenIn + v.TokenOut
		if tokens <= 0 {
			tokens = approxTokensFromText(v.Content)
		}
		if maxTokens > 0 && len(collected) > 0 && totalTokens+tokens > maxTokens {
			break
		}
		totalTokens += tokens
		collected = append(collected, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected, nil
}

func approxTokensFromText(text string) int {
	runes := 0
	for range text {
		runes++
	}
	if runes == 0 {
		return 0
	}
	tokens := runes / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

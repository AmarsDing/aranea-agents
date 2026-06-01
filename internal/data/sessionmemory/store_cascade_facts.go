package sessionmemory

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ReplaceNameInAgentFacts rewrites whole-word occurrences of oldName to newName for one agent scope.
// Returns updated fact JSON rows and the number of facts changed.
func (st *Store) ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error) {
	if st == nil || st.client == nil {
		return nil, 0, errors.New("session memory store not wired")
	}
	agentID = strings.TrimSpace(agentID)
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if agentID == "" || oldName == "" || newName == "" || strings.EqualFold(oldName, newName) {
		return nil, 0, nil
	}
	re, err := nameReplacePattern(oldName)
	if err != nil {
		return nil, 0, err
	}
	like := "%" + strings.ToLower(oldName) + "%"
	rows, err := st.client.QueryContext(ctx, sqlFactSelect+`
 WHERE scope_type = 'agent' AND scope_id = ? AND deleted_at = '' AND status = 'active'
 AND (LOWER(statement) LIKE ? OR LOWER(details_markdown) LIKE ?)`,
		agentID, like, like)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	type pendingUpdate struct {
		id, stmt, details, meta string
		sessionID, messageID    string
	}
	var pending []pendingUpdate
	for rows.Next() {
		raw, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, err
		}
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			st.lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
			return nil, 0, err
		}
		id, _ := row["id"].(string)
		stmt, _ := row["statement"].(string)
		details, _ := row["details_markdown"].(string)
		if id == "" {
			continue
		}
		if !re.MatchString(stmt) && !re.MatchString(details) {
			continue
		}
		newStmt := replaceNameWordBoundary(stmt, re, newName)
		newDetails := replaceNameWordBoundary(details, re, newName)
		if newStmt == stmt && newDetails == details {
			continue
		}
		pending = append(pending, pendingUpdate{
			id: id, stmt: newStmt, details: newDetails,
			meta: mergeCascadeFactMeta(jsonStrMap(row, "metadata_json"), oldName, newName, st.lg),
			sessionID: jsonStrMap(row, "source_session_id"),
			messageID: jsonStrMap(row, "source_message_id"),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var updated [][]byte
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range pending {
		if _, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET
 statement = ?, statement_normalized = ?, details_markdown = ?,
 metadata_json = ?, version = version + 1, updated_at = ?
WHERE id = ? AND deleted_at = ''`,
			item.stmt, strings.ToLower(strings.TrimSpace(item.stmt)), item.details, item.meta, now, item.id); err != nil {
			return updated, len(updated), err
		}
		st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
			Action:        "CASCADE_RENAME",
			TargetKind:    "fact",
			TargetID:      item.id,
			Reason:        oldName + " -> " + newName,
			PolicyVersion: "cascade_v1",
			SourceEventIDs: []string{
				strings.TrimSpace(item.sessionID),
				strings.TrimSpace(item.messageID),
			},
			TurnID: composeTurnRef(item.sessionID, item.messageID),
		})
		out, err := json.Marshal(map[string]any{
			"id": item.id, "scope_id": agentID, "agent_id": agentID,
			"statement": item.stmt, "details_markdown": item.details,
			"metadata_json": item.meta, "updated_at": now,
		})
		if err != nil {
			return updated, len(updated), err
		}
		updated = append(updated, out)
	}
	return updated, len(updated), nil
}

func nameReplacePattern(oldName string) (*regexp.Regexp, error) {
	return regexp.Compile(`(?i)\b` + regexp.QuoteMeta(oldName) + `\b`)
}

func replaceNameWordBoundary(text string, re *regexp.Regexp, newName string) string {
	if text == "" || re == nil {
		return text
	}
	return re.ReplaceAllString(text, newName)
}

func composeTurnRef(sessionID, messageID string) string {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	switch {
	case sessionID != "" && messageID != "":
		return sessionID + ":" + messageID
	case messageID != "":
		return messageID
	default:
		return sessionID
	}
}

func jsonStrMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mergeCascadeFactMeta(base, oldName, newName string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		m = map[string]any{}
	} else if m == nil {
		m = map[string]any{}
	}
	m["cascade_renamed_from"] = oldName
	m["cascade_renamed_to"] = newName
	m["source"] = "cascade_approve"
	b, _ := json.Marshal(m)
	return string(b)
}

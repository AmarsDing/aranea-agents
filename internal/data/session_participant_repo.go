package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	bizsess "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type sessionParticipantRepo struct {
	data *Data
}

var _ bizsess.SessionParticipantRepository = (*sessionParticipantRepo)(nil)

func NewSessionParticipantRepo(d *Data) bizsess.SessionParticipantRepository {
	return &sessionParticipantRepo{data: d}
}

func (r *sessionParticipantRepo) db() *sql.DB {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RawDB()
}

type participantAgg struct {
	participantType string
	participantID   string
	displayName     string
	roleInSession   string
	messageCount    int
	inputTokens     int
	outputTokens    int
	firstActiveAt   string
	lastActiveAt    string
}

func (r *sessionParticipantRepo) SyncFromSession(ctx context.Context, sess bizsess.Session, messages []bizsess.ChatMessage) error {
	if r.data == nil {
		return nil
	}
	sessionID := strings.TrimSpace(sess.ID)
	if sessionID == "" {
		return nil
	}
	aggs := map[string]*participantAgg{}

	ensure := func(key, pType, pID, name, role string) *participantAgg {
		if aggs[key] == nil {
			aggs[key] = &participantAgg{
				participantType: pType,
				participantID:   pID,
				displayName:     name,
				roleInSession:   role,
			}
		}
		return aggs[key]
	}

	if aid := strings.TrimSpace(sess.AgentID); aid != "" {
		ensure("agent:"+aid+":owner", "agent", aid, aid, "owner")
	}

	for _, msg := range messages {
		pType, pID, name, role := participantFromMessage(msg, sess, r.data.lg)
		if pID == "" {
			continue
		}
		key := pType + ":" + pID + ":" + role
		row := ensure(key, pType, pID, name, role)
		row.messageCount++
		row.inputTokens += msg.TokenIn
		row.outputTokens += msg.TokenOut
		at := strings.TrimSpace(msg.CreatedAt)
		if at == "" {
			continue
		}
		if row.firstActiveAt == "" || at < row.firstActiveAt {
			row.firstActiveAt = at
		}
		if at > row.lastActiveAt {
			row.lastActiveAt = at
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RawDB())
		if _, err := e.ExecContext(txCtx, `DELETE FROM session_participants WHERE session_id=?`, sessionID); err != nil {
			return err
		}
		for _, row := range aggs {
			id := uuid.NewString()
			_, err := e.ExecContext(txCtx, `
INSERT INTO session_participants (
  id, session_id, participant_type, participant_id, display_name, role_in_session, status,
  first_active_at, last_active_at, message_count, run_step_count, input_tokens, output_tokens,
  context_used_ratio, metadata_json, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, sessionID, row.participantType, row.participantID, row.displayName, row.roleInSession, "active",
				row.firstActiveAt, row.lastActiveAt, row.messageCount, 0, row.inputTokens, row.outputTokens,
				0, "{}", now, now,
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func participantFromMessage(msg bizsess.ChatMessage, sess bizsess.Session, lg loggateway.Logger) (pType, pID, name, role string) {
	if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		return "user", "user", "User", "owner"
	}
	var opts struct {
		AgentID string `json:"agent_id"`
		Name    string `json:"name"`
		Agent   struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"agent"`
		TeamMember struct {
			AgentID string `json:"agent_id"`
			Name    string `json:"name"`
			Role    string `json:"role"`
		} `json:"team_member"`
	}
	if msg.OptionsJSON != "" {
		if err := json.Unmarshal([]byte(msg.OptionsJSON), &opts); err != nil {
			lg.Warn("options json unmarshal failed", loggateway.StepID("data.session_participant"), loggateway.Err(err))
		}
	}
	if opts.TeamMember.AgentID != "" || opts.TeamMember.Name != "" {
		pID = firstNonEmpty(opts.TeamMember.AgentID, opts.Agent.ID, opts.AgentID, sess.AgentID)
		name = firstNonEmpty(opts.TeamMember.Name, opts.Agent.DisplayName, opts.Name, pID)
		role = firstNonEmpty(opts.TeamMember.Role, "executor")
		return "agent", pID, name, role
	}
	pID = firstNonEmpty(opts.Agent.ID, opts.AgentID, sess.AgentID)
	name = firstNonEmpty(opts.Agent.DisplayName, opts.Name, pID)
	if pID == "" {
		return "", "", "", ""
	}
	return "agent", pID, name, "executor"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (r *sessionParticipantRepo) ListBySession(ctx context.Context, sessionID string) ([]bizsess.SessionParticipant, error) {
	db := r.db()
	if db == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, session_id, participant_type, participant_id, display_name, role_in_session, status,
  first_active_at, last_active_at, message_count, run_step_count, input_tokens, output_tokens,
  context_used_ratio, metadata_json, created_at, updated_at
FROM session_participants WHERE session_id=? ORDER BY message_count DESC, last_active_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bizsess.SessionParticipant
	for rows.Next() {
		var p bizsess.SessionParticipant
		if err := rows.Scan(
			&p.ID, &p.SessionID, &p.ParticipantType, &p.ParticipantID, &p.DisplayName, &p.RoleInSession, &p.Status,
			&p.FirstActiveAt, &p.LastActiveAt, &p.MessageCount, &p.RunStepCount, &p.InputTokens, &p.OutputTokens,
			&p.ContextUsedRatio, &p.MetadataJSON, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

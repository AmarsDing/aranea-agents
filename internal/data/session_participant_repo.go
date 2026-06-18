package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	bizsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/sessionparticipant"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type sessionParticipantRepo struct {
	data *Data
}

var _ bizsess.SessionParticipantRepository = (*sessionParticipantRepo)(nil)

func NewSessionParticipantRepo(d *Data) bizsess.SessionParticipantRepository {
	return &sessionParticipantRepo{data: d}
}

func (r *sessionParticipantRepo) readClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Read(ctx)
}

func (r *sessionParticipantRepo) writeClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Write(ctx)
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

// entSessionParticipantToBiz converts an Ent SessionParticipant entity to a biz SessionParticipant.
func entSessionParticipantToBiz(e *ent.SessionParticipant) bizsess.SessionParticipant {
	if e == nil {
		return bizsess.SessionParticipant{}
	}
	return bizsess.SessionParticipant{
		ID:               e.ID,
		SessionID:        e.SessionID,
		ParticipantType:  e.ParticipantType,
		ParticipantID:    e.ParticipantID,
		DisplayName:      e.DisplayName,
		RoleInSession:    e.RoleInSession,
		Status:           e.Status,
		FirstActiveAt:    e.FirstActiveAt,
		LastActiveAt:     e.LastActiveAt,
		MessageCount:     e.MessageCount,
		RunStepCount:     e.RunStepCount,
		InputTokens:      e.InputTokens,
		OutputTokens:     e.OutputTokens,
		ContextUsedRatio: e.ContextUsedRatio,
		MetadataJSON:     e.MetadataJSON,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	}
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
		client := r.data.RW().Write(txCtx)
		// Delete existing participants for this session.
		_, err := client.SessionParticipant.Delete().
			Where(sessionparticipant.SessionIDEQ(sessionID)).
			Exec(txCtx)
		if err != nil {
			return entErrToBizErr(err, "SESSION_PARTICIPANT")
		}
		// Insert new participants.
		for _, row := range aggs {
			id := uuid.NewString()
			_, err := client.SessionParticipant.Create().
				SetID(id).
				SetSessionID(sessionID).
				SetParticipantType(row.participantType).
				SetParticipantID(row.participantID).
				SetDisplayName(row.displayName).
				SetRoleInSession(row.roleInSession).
				SetStatus("active").
				SetFirstActiveAt(row.firstActiveAt).
				SetLastActiveAt(row.lastActiveAt).
				SetMessageCount(row.messageCount).
				SetRunStepCount(0).
				SetInputTokens(row.inputTokens).
				SetOutputTokens(row.outputTokens).
				SetContextUsedRatio(0).
				SetMetadataJSON("{}").
				SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(txCtx)
		if err != nil {
			return entErrToBizErr(err, "SESSION_PARTICIPANT")
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
	client := r.readClient(ctx)
	if client == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session id is required")
	}
	items, err := client.SessionParticipant.Query().
		Where(sessionparticipant.SessionIDEQ(sessionID)).
		Order(ent.Desc(sessionparticipant.FieldMessageCount), ent.Desc(sessionparticipant.FieldLastActiveAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_PARTICIPANT")
	}
	out := make([]bizsess.SessionParticipant, len(items))
	for i, item := range items {
		out[i] = entSessionParticipantToBiz(item)
	}
	return out, nil
}

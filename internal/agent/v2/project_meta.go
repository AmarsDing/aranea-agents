package v2

import (
	"time"

	"aranea-agents/internal/biz"
)

// ProjectMeta carries the per-turn metadata required by ActivityProjector v2
// to project runtime callbacks into v2 events.
//
// All fields are set once at turn start and treated as read-only for the
// duration of the turn. The v1→v2 mapping is performed by
// v2ProjectMetaFromV1 in internal/agent/stream_consumer.go.
type ProjectMeta struct {
	SessionID       string // current session (spirit / team / member)
	SpiritSessionID string // root spirit session (WS filtering key)
	TaskID          string
	TurnID          string
	ParentTurnID    string
	TeamStageID     string // non-empty for team member turns
	TeamRunID       string
	TeamID          string
	MemberSessionID string
	AgentKey        string
	AgentName       string
	MemberAgentKeys map[string]struct{}
	TaskContent     string // user input text for the root task
}

// newTask constructs a biz.Task from ProjectMeta. The Task's SessionID is the
// spirit session ID (per spec §3.2.2: task.SessionID = spirit_session_id).
func (m ProjectMeta) newTask(id string, status biz.TaskStatus, content string) biz.Task {
	now := time.Now()
	return biz.Task{
		ID:          id,
		SessionID:   m.SpiritSessionID,
		UserMessage: content,
		Status:      status,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// newTurn constructs a biz.Turn from ProjectMeta.
func (m ProjectMeta) newTurn(id string, seq int64) biz.Turn {
	return biz.Turn{
		ID:              id,
		TaskID:          m.TaskID,
		SessionID:       m.SessionID,
		SpiritSessionID: m.SpiritSessionID,
		ParentTurnID:    m.ParentTurnID,
		TeamStageID:     m.TeamStageID,
		TeamID:          m.TeamID,
		AgentKey:        m.AgentKey,
		Seq:             seq,
		Status:          biz.TurnStatusRunning,
		StartedAt:       time.Now(),
		Version:         1,
	}
}

// newStep constructs a biz.Step from ProjectMeta.
func (m ProjectMeta) newStep(id string, kind biz.StepKind, seq int64) biz.Step {
	return biz.Step{
		ID:              id,
		TurnID:          m.TurnID,
		TaskID:          m.TaskID,
		SessionID:       m.SessionID,
		SpiritSessionID: m.SpiritSessionID,
		Kind:            kind,
		Seq:             seq,
		AuthorAgentKey:  m.AgentKey,
		Status:          biz.StepStatusPending,
		StartedAt:       time.Now(),
		Version:         1,
	}
}

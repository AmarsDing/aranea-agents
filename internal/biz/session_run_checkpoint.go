package biz

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const durableResumeUserContent = "[系统] 请从上次中断处继续完成用户的任务，不要重复已完成的步骤。"

// DurableRunCheckpointPayload is persisted for worker resume (CC-R-03 / CC-F-02).
type DurableRunCheckpointPayload struct {
	SessionID        string `json:"session_id"`
	TurnID           string `json:"turn_id"`
	AgentID          string `json:"agent_id"`
	RuntimeRunID     string `json:"runtime_run_id"`
	TrpcInvocationID string `json:"trpc_invocation_id,omitempty"`
	UserContent      string `json:"user_content,omitempty"`
	SessionRevision  int64  `json:"session_revision,omitempty"`
	DialogMode       string `json:"dialog_mode,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
}

// DurableCheckpointSnapshot captures interactive turn state at escalation (CC-F-02).
type DurableCheckpointSnapshot struct {
	Run              SessionRun
	AgentID          string
	UserContent      string
	SessionRevision  int64
	DialogMode       string
	Provider         string
	Model            string
	TrpcInvocationID string
}

// CreateDurableCheckpoint snapshots run state before durable worker resume.
func (u *SessionRunUsecase) CreateDurableCheckpoint(ctx context.Context, snap DurableCheckpointSnapshot) (SessionRunCheckpoint, error) {
	run := snap.Run
	if u == nil || u.cps == nil || strings.TrimSpace(run.ID) == "" {
		return SessionRunCheckpoint{}, nil
	}
	trpcInv := strings.TrimSpace(snap.TrpcInvocationID)
	if trpcInv == "" {
		trpcInv = strings.TrimSpace(run.RuntimeRunID)
	}
	payload, err := json.Marshal(DurableRunCheckpointPayload{
		SessionID:        run.SessionID,
		TurnID:           run.TurnID,
		AgentID:          strings.TrimSpace(snap.AgentID),
		RuntimeRunID:     run.RuntimeRunID,
		TrpcInvocationID: trpcInv,
		UserContent:      strings.TrimSpace(snap.UserContent),
		SessionRevision:  snap.SessionRevision,
		DialogMode:       strings.TrimSpace(snap.DialogMode),
		Provider:         strings.TrimSpace(snap.Provider),
		Model:            strings.TrimSpace(snap.Model),
	})
	if err != nil {
		return SessionRunCheckpoint{}, err
	}
	now := sessionRunNow()
	cp := SessionRunCheckpoint{
		ID:           uuid.NewString(),
		SessionRunID: run.ID,
		SessionID:    run.SessionID,
		TurnID:       run.TurnID,
		AgentID:      strings.TrimSpace(snap.AgentID),
		PayloadJSON:  string(payload),
		CreatedAt:    now,
	}
	id, err := u.cps.Create(ctx, cp)
	if err != nil {
		return SessionRunCheckpoint{}, err
	}
	cp.ID = id
	if err := u.repo.UpdateCheckpointID(ctx, run.ID, id); err != nil {
		u.lg.Warn("update checkpoint id failed", loggateway.StepID("session_run_checkpoint"), loggateway.Str("run_id", run.ID), loggateway.Str("checkpoint_id", id), loggateway.Err(err))
	}
	return cp, nil
}

func (u *SessionRunUsecase) ListDurablePending(ctx context.Context, limit int) ([]SessionRun, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListByPhase(ctx, SessionRunPhaseDurable, limit)
}

// ListByPhase exposes phase-filtered run listing for service-layer jobs
// (e.g. shutdown durable escalation listing interactive runs).
func (u *SessionRunUsecase) ListByPhase(ctx context.Context, phase string, limit int) ([]SessionRun, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListByPhase(ctx, phase, limit)
}

func (u *SessionRunUsecase) ListForJobs(ctx context.Context, q SessionRunListQuery) ([]SessionRun, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListForJobs(ctx, q)
}

func (u *SessionRunUsecase) GetCheckpoint(ctx context.Context, sessionRunID string) (SessionRunCheckpoint, error) {
	if u == nil || u.cps == nil {
		return SessionRunCheckpoint{}, nil
	}
	return u.cps.GetBySessionRunID(ctx, sessionRunID)
}

func ParseDurableCheckpointPayload(raw string) (DurableRunCheckpointPayload, error) {
	var p DurableRunCheckpointPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &p); err != nil {
		return DurableRunCheckpointPayload{}, err
	}
	return p, nil
}

func DurableResumePrompt() string { return durableResumeUserContent }

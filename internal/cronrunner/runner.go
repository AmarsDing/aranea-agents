package cronrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/legacychat"

	"github.com/google/uuid"
)

// Deps wires cron execution to Ent repos + session create + legacy chat HTTP.
type Deps struct {
	Cron    biz.CronRepo
	Session *biz.SessionUsecase
	Teams   biz.TeamRepository
	Agents  biz.AgentRepository
}

// Runner executes due cron_task rows on an interval (ported from pkg/backend CronRunner).
type Runner struct {
	deps Deps
	mu   sync.Mutex
}

// NewRunner constructs a runner.
func NewRunner(deps Deps) *Runner {
	return &Runner{deps: deps}
}

// DefaultInterval reads CRON_RUNNER_INTERVAL (default 1m); values <=0 fall back to 1m.
func DefaultInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CRON_RUNNER_INTERVAL"))
	if raw == "" {
		return time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return time.Minute
	}
	return d
}

// Start blocks until ctx cancelled; runs an initial tick then ticks every interval.
func (r *Runner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	r.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runDue(ctx)
		}
	}
}

func (r *Runner) runDue(ctx context.Context) {
	if !r.mu.TryLock() {
		return
	}
	defer r.mu.Unlock()

	tasks, err := r.deps.Cron.ListCronTasks(ctx)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if !task.Enabled || task.Status != "active" || task.DeletedAt != "" {
			continue
		}
		cfg, err := parseCronTaskConfig(task.ConfigJSON)
		if err != nil || strings.TrimSpace(cfg.Message) == "" {
			r.recordSkipped(ctx, task, now, firstNonEmptyString(errString(err), "cron message is required"))
			continue
		}
		meta := parseCronTaskMetadata(task.MetadataJSON)
		dueAt, due, err := cronTaskDueAt(task.UpdatedAt, cfg, meta, now)
		if err != nil {
			r.recordSkipped(ctx, task, now, err.Error())
			continue
		}
		if !due {
			if meta.NextRunAt == "" && !dueAt.IsZero() {
				meta.NextRunAt = dueAt.Format(time.RFC3339)
				r.persistMetadata(ctx, task, meta)
			}
			continue
		}
		r.executeTask(ctx, task, cfg, meta, now)
	}
}

func (r *Runner) executeTask(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, meta cronTaskMetadata, now time.Time) {
	runID := uuid.NewString()
	started := now.Format(time.RFC3339)
	outputPending := mustMarshalJSON(map[string]any{"trigger": "schedule"})
	if err := r.deps.Cron.InsertCronTaskRun(ctx, runID, task.ID, "pending", started, outputPending, started); err != nil {
		return
	}

	result, execErr := r.dispatchCronTask(ctx, task, cfg)
	finishedAt := time.Now().UTC()
	output := map[string]any{
		"trigger":          "schedule",
		"target_type":      cronTargetType(cfg),
		"session_id":       result.SessionID,
		"user_message_id":  result.UserMessageID,
		"agent_message_id": result.AgentMessageID,
		"run_id":           result.AgentMessageID,
	}
	outJSON := mustMarshalJSON(output)
	finished := finishedAt.Format(time.RFC3339)
	status := "success"
	errMsg := ""
	if execErr != nil {
		status = "failure"
		errMsg = execErr.Error()
	}
	_ = r.deps.Cron.UpdateCronTaskRun(ctx, runID, status, finished, outJSON, errMsg)

	meta.RunCount++
	meta.LastRunAt = finished
	meta.LastRunStatus = status
	if execErr != nil {
		meta.FailureCount++
		meta.LastError = execErr.Error()
		meta.RecentFailure = append([]cronFailureSummary{{
			StartedAt:    started,
			ErrorMessage: execErr.Error(),
		}}, meta.RecentFailure...)
		if len(meta.RecentFailure) > 5 {
			meta.RecentFailure = meta.RecentFailure[:5]
		}
	} else {
		meta.SuccessCount++
		meta.LastError = ""
	}
	if cfg.ScheduleType == "once" {
		task.Enabled = false
		task.Status = "paused"
		meta.NextRunAt = ""
	} else if next, err := nextCronRunAfter(cfg, finishedAt); err == nil {
		meta.NextRunAt = next.Format(time.RFC3339)
	}
	rawMeta, err := json.Marshal(meta)
	if err != nil {
		return
	}
	task.MetadataJSON = string(rawMeta)
	_, _ = r.deps.Cron.UpdateCronTask(ctx, task)
}

type cronDispatchResult struct {
	SessionID      string
	UserMessageID  string
	AgentMessageID string
}

func (r *Runner) dispatchCronTask(ctx context.Context, task biz.CronTask, cfg cronTaskConfig) (cronDispatchResult, error) {
	targetType := cronTargetType(cfg)
	switch targetType {
	case "team":
		teamID := strings.TrimSpace(cfg.TeamID)
		if teamID == "" {
			return cronDispatchResult{}, validationErr("team_id is required for team cron target")
		}
		if _, err := r.deps.Teams.GetTeamByID(ctx, teamID); err != nil {
			return cronDispatchResult{}, err
		}
		sess, err := r.deps.Session.Create(ctx, biz.Session{
			OwnerType:  "team",
			TeamID:     teamID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		return r.postChat(ctx, sendMessagePayload{
			SessionID: sess.ID,
			TeamID:    teamID,
			Content:   cfg.Message,
			Options:   sendMessageOptions{DialogMode: "cron"},
		})
	default:
		agent, err := r.resolveCronAgent(ctx, task)
		if err != nil {
			return cronDispatchResult{}, err
		}
		sess, err := r.deps.Session.Create(ctx, biz.Session{
			OwnerType:  "agent",
			AgentID:    agent.ID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		return r.postChat(ctx, sendMessagePayload{
			SessionID: sess.ID,
			AgentKey:  agent.AgentKey,
			Content:   cfg.Message,
			Options:   sendMessageOptions{DialogMode: "cron"},
		})
	}
}

func (r *Runner) resolveCronAgent(ctx context.Context, task biz.CronTask) (biz.Agent, error) {
	if strings.TrimSpace(task.AgentID) != "" {
		return r.deps.Agents.GetAgentByID(ctx, task.AgentID)
	}
	res, err := r.deps.Agents.SearchAgents(ctx, biz.AgentListQuery{Limit: 500, Offset: 0})
	if err != nil {
		return biz.Agent{}, err
	}
	for _, agent := range res.Items {
		if agent.IsDefault {
			return agent, nil
		}
	}
	if len(res.Items) > 0 {
		return res.Items[0], nil
	}
	return biz.Agent{}, validationErr("no agent available for cron task")
}

func (r *Runner) recordSkipped(ctx context.Context, task biz.CronTask, now time.Time, message string) {
	meta := parseCronTaskMetadata(task.MetadataJSON)
	meta.RunCount++
	meta.FailureCount++
	meta.LastRunAt = now.Format(time.RFC3339)
	meta.LastRunStatus = "failure"
	meta.LastError = message
	meta.RecentFailure = append([]cronFailureSummary{{StartedAt: meta.LastRunAt, ErrorMessage: message}}, meta.RecentFailure...)
	if len(meta.RecentFailure) > 5 {
		meta.RecentFailure = meta.RecentFailure[:5]
	}
	r.persistMetadata(ctx, task, meta)
}

func (r *Runner) persistMetadata(ctx context.Context, task biz.CronTask, meta cronTaskMetadata) {
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	task.MetadataJSON = string(raw)
	_, _ = r.deps.Cron.UpdateCronTask(ctx, task)
}

type sendMessagePayload struct {
	SessionID string             `json:"session_id"`
	AgentKey  string             `json:"agent_key,omitempty"`
	TeamID    string             `json:"team_id,omitempty"`
	Content   string             `json:"content"`
	Options   sendMessageOptions `json:"options"`
}

type sendMessageOptions struct {
	DialogMode string `json:"dialog_mode"`
}

func (r *Runner) postChat(ctx context.Context, in sendMessagePayload) (cronDispatchResult, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN")), "/")
	if base == "" {
		return cronDispatchResult{}, errors.New("LEGACY_REST_ORIGIN is not set (cron needs POST " + legacychat.MessagesPath + " on legacy backend)")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return cronDispatchResult{}, fmt.Errorf("invalid LEGACY_REST_ORIGIN")
	}
	endpoint := base + legacychat.MessagesPath
	body, err := json.Marshal(in)
	if err != nil {
		return cronDispatchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return cronDispatchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return cronDispatchResult{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return cronDispatchResult{}, fmt.Errorf("chat POST %s: %s %s", endpoint, resp.Status, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		UserMessage struct {
			ID string `json:"id"`
		} `json:"user_message"`
		AgentMessage struct {
			ID string `json:"id"`
		} `json:"agent_message"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return cronDispatchResult{}, err
	}
	return cronDispatchResult{
		SessionID:      in.SessionID,
		UserMessageID:  out.UserMessage.ID,
		AgentMessageID: out.AgentMessage.ID,
	}, nil
}

func validationErr(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

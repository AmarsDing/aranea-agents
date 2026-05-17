package cronrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// defaultRetryBackoff is the 3-step backoff schedule: 30s, 2m, 10m.
var defaultRetryBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// maxDeadFailures is the consecutive-failure threshold before a job enters the dead state.
const maxDeadFailures = 3

var (
	cronJobRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_cron_job_runs_total",
		Help: "Number of cron job executions by job_id and status.",
	}, []string{"job_id", "status"})

	cronJobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_cron_job_duration_seconds",
		Help:    "Duration of cron job executions.",
		Buckets: []float64{0.5, 1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"job_id"})

	cronJobDeadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_cron_job_dead_total",
		Help: "Number of cron jobs that reached the dead-letter state.",
	}, []string{"job_id"})
)

// Deps wires cron execution to Ent repos + session create + chat HTTP POST.
type Deps struct {
	Cron     biz.CronRepo
	Session  *biz.SessionUsecase
	Teams    biz.TeamRepository
	Agents   biz.AgentRepository
	EventBus event.Bus
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
			r.recordSkipped(ctx, task, now, strutil.FirstNonEmpty(errString(err), "cron message is required"))
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

	startedTime := time.Now()
	result, execErr := r.dispatchWithRetry(ctx, task, cfg)
	elapsed := time.Since(startedTime)

	cronJobDuration.WithLabelValues(task.ID).Observe(elapsed.Seconds())

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
	cronJobRunsTotal.WithLabelValues(task.ID, status).Inc()
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
		// Dead-letter: mark job as dead after maxDeadFailures consecutive failures.
		if meta.FailureCount >= maxDeadFailures && task.Status != "dead" {
			task.Status = "dead"
			task.Enabled = false
			cronJobDeadTotal.WithLabelValues(task.ID).Inc()
			slog.Warn("cron job entered dead state", "job_id", task.ID, "task_key", task.TaskKey, "failure_count", meta.FailureCount)
			r.publishDeadLetterEvent(ctx, task)
		}
	} else {
		meta.SuccessCount++
		meta.LastError = ""
		meta.FailureCount = 0
	}
	if cfg.ScheduleType == "once" {
		task.Enabled = false
		if task.Status != "dead" {
			task.Status = "paused"
		}
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

// dispatchWithRetry runs dispatchCronTask with exponential back-off retries (30s/2m/10m).
// It recovers from panics on each attempt via safego.RecoverFunc.
func (r *Runner) dispatchWithRetry(ctx context.Context, task biz.CronTask, cfg cronTaskConfig) (res cronDispatchResult, err error) {
	attempts := len(defaultRetryBackoff) + 1
	for attempt := 0; attempt < attempts; attempt++ {
		res, err = r.dispatchSafe(ctx, task, cfg)
		if err == nil {
			return res, nil
		}
		if attempt < len(defaultRetryBackoff) {
			delay := defaultRetryBackoff[attempt]
			slog.Warn("cron job attempt failed, retrying", "job_id", task.ID, "attempt", attempt+1, "delay", delay, "error", err)
			select {
			case <-ctx.Done():
				return cronDispatchResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return cronDispatchResult{}, err
}

// dispatchSafe wraps dispatchCronTask with panic recovery.
func (r *Runner) dispatchSafe(ctx context.Context, task biz.CronTask, cfg cronTaskConfig) (res cronDispatchResult, retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("cron panic: %v", rec)
			slog.Error("cron task panicked", "job_id", task.ID, "panic", rec)
		}
	}()
	return r.dispatchCronTask(ctx, task, cfg)
}

// publishDeadLetterEvent emits a dead-letter admin alert event via the event bus.
func (r *Runner) publishDeadLetterEvent(ctx context.Context, task biz.CronTask) {
	if r.deps.EventBus == nil {
		return
	}
	safego.Go(ctx, "cron.dead_letter.publish", func() {
		env := event.NewEnvelope("cron.dead_letter", "cron", "")
		env.Metadata = map[string]any{
			"job_id":   task.ID,
			"task_key": task.TaskKey,
			"name":     task.Name,
		}
		r.deps.EventBus.Publish(context.Background(), env)
	})
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
		res, err := r.postChat(ctx, sendMessagePayload{
			SessionID: sess.ID,
			TeamID:    teamID,
			Content:   cfg.Message,
			Options:   sendMessageOptions{DialogMode: "cron"},
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		r.publishTeamCronMaybe(ctx, teamID)
		return res, nil
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

func cronChatPOSTRoot() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("CRON_CHAT_DISPATCH_ORIGIN")), "/")
}

func (r *Runner) publishTeamCronMaybe(ctx context.Context, teamID string) {
	if r.deps.EventBus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "cron", "")
		env.TeamID = teamID
		env.Metadata = map[string]any{"hint": true}
		r.deps.EventBus.Publish(ctx, env)
	}
}

func (r *Runner) postChat(ctx context.Context, in sendMessagePayload) (cronDispatchResult, error) {
	base := cronChatPOSTRoot()
	if base == "" {
		return cronDispatchResult{}, errors.New(`cron chat: set CRON_CHAT_DISPATCH_ORIGIN (-> admin /v1/chat/messages)`)
	}
	pu, err := url.Parse(base)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return cronDispatchResult{}, fmt.Errorf("invalid cron chat dispatch URL base")
	}
	endpoint := base + "/v1/chat/messages"
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

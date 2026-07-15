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
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

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

// CronChatRunner dispatches a single cron-triggered chat turn via the in-process
// agent runner (EP-RT-07). Implemented by *service.ChatService.
type CronChatRunner interface {
	RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error)
}

// SessionRunControl is optionally implemented by CronChatRunner to respect
// active runs on the shared RunGateway before dispatch.
type SessionRunControl interface {
	HasActiveRun(sessionID string) bool
}

// Deps wires cron execution to Ent repos + session create + chat HTTP POST.
type Deps struct {
	Cron              biz.CronRepo
	Session           *biz.SessionUsecase
	Teams             biz.TeamReader
	Agents            biz.AgentRepository
	EventBus          biz.EventBus        // v2 unified bus for chat/system events
	MonitorBus        contract.MonitorBus // typed monitor bus (cron.dead_letter)
	Chat              CronChatRunner
	RegistrySyncAgent CronRegistrySyncAgent
}

type CronRegistrySyncAgent interface {
	RunSync(ctx context.Context) error
}

// Runner executes due cron_task rows on an interval (ported from pkg/backend CronRunner).
type Runner struct {
	deps   Deps
	lg     loggateway.Logger
	mu     sync.Mutex
	taskMu sync.Map
}

func NewRunner(deps Deps, lg loggateway.Logger) *Runner {
	return &Runner{deps: deps, lg: lg}
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
			badCfg, _ := parseCronTaskConfig(task.ConfigJSON)
			r.recordScheduleFailure(ctx, task, badCfg, now, strutil.FirstNonEmpty(errString(err), "cron message is required"))
			continue
		}
		meta := parseCronTaskMetadata(task.MetadataJSON, r.lg)
		dueAt, due, err := cronTaskDueAt(task.UpdatedAt, cfg, meta, now)
		if err != nil {
			r.recordScheduleFailure(ctx, task, cfg, now, err.Error())
			continue
		}
		if !due {
			if meta.NextRunAt == "" && !dueAt.IsZero() {
				meta.NextRunAt = dueAt.Format(time.RFC3339)
				r.persistMetadata(ctx, task, meta)
			}
			continue
		}
		r.executeTask(ctx, task, cfg, meta, now, "schedule")
	}
}

// TriggerTask enqueues a manual run (async). Returns the pending cron_task_run row immediately.
func (r *Runner) TriggerTask(ctx context.Context, taskID string) (biz.CronTaskRun, error) {
	if r == nil {
		return biz.CronTaskRun{}, biz.ErrCronRunnerDisabled
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return biz.CronTaskRun{}, validationErr("task id is required")
	}
	task, err := r.deps.Cron.GetCronTask(ctx, taskID)
	if err != nil {
		return biz.CronTaskRun{}, err
	}
	if task.DeletedAt != "" {
		return biz.CronTaskRun{}, biz.ErrCronTaskDeleted
	}
	cfg, err := parseCronTaskConfig(task.ConfigJSON)
	if err != nil {
		return biz.CronTaskRun{}, err
	}
	if strings.TrimSpace(cfg.Message) == "" {
		return biz.CronTaskRun{}, validationErr("cron message is required")
	}
	now := time.Now().UTC()
	unlock := r.lockTask(taskID)
	runID, started, ok := r.insertPendingRun(ctx, task.ID, "manual", now)
	unlock()
	if !ok {
		return biz.CronTaskRun{}, errors.New("cron trigger failed to create run record")
	}
	cfgSnapshot := cfg
	safego.Go(ctx, "cron.manual_trigger", func() {
		r.runManualTask(ctx, taskID, runID, started, cfgSnapshot)
	})
	return r.deps.Cron.GetCronTaskRun(ctx, runID)
}

// cronDispatchState carries session-creation state across retry attempts so
// retries reuse the same session instead of leaking orphan sessions.
//
// Background: dispatchCronTask creates a new Session before invoking Chat.RunCronTurn.
// Without state threading, dispatchWithRetry would call dispatchCronTask again on
// each retry, creating N orphan sessions for N attempts. With state threading,
// the session is created on the first attempt and reused on every retry.
type cronDispatchState struct {
	sessID string
}

// dispatchWithRetry runs dispatchCronTask with exponential back-off retries (30s/2m/10m).
// It recovers from panics on each attempt via safego.RecoverFunc.
//
// Session creation is idempotent across retries: the first attempt that creates
// a session stores its ID in cronDispatchState, and subsequent attempts reuse
// that ID instead of creating another session.
func (r *Runner) dispatchWithRetry(ctx context.Context, task biz.CronTask, cfg cronTaskConfig) (res cronDispatchResult, err error) {
	state := &cronDispatchState{}
	attempts, backoff := retryPlan(effectiveRetryMaxAttempts(cfg))
	for attempt := 0; attempt < attempts; attempt++ {
		res, err = r.dispatchSafe(ctx, task, cfg, state)
		if err == nil {
			return res, nil
		}
		if attempt < len(backoff) {
			delay := backoff[attempt]
			r.lg.Warn("定时任务重试",
				loggateway.StepID("cron.retry"),
				loggateway.Str("job_id", task.ID),
				loggateway.Int("attempt", attempt+1),
				loggateway.Str("delay", delay.String()),
				loggateway.Err(err))
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
func (r *Runner) dispatchSafe(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, state *cronDispatchState) (res cronDispatchResult, retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("cron panic: %v", rec)
			r.lg.Error("定时任务 panic",
				loggateway.StepID("cron.panic"),
				loggateway.Str("job_id", task.ID),
				loggateway.Any("panic", rec))
		}
	}()
	return r.dispatchCronTask(ctx, task, cfg, state)
}

// publishDeadLetterEvent emits a dead-letter admin alert event via the typed MonitorBus.
func (r *Runner) publishDeadLetterEvent(ctx context.Context, task biz.CronTask) {
	if r.deps.MonitorBus == nil {
		return
	}
	safego.Go(ctx, "cron.dead_letter.publish", func() {
		ev := contract.NewMonitorEvent(contract.MonitorEventTypeCronDeadLetter, "cron")
		ev.Metadata = map[string]any{
			"job_id":   task.ID,
			"task_key": task.TaskKey,
			"name":     task.Name,
		}
		r.deps.MonitorBus.Publish(context.Background(), ev)
	})
}

type cronDispatchResult struct {
	SessionID      string
	UserMessageID  string
	AgentMessageID string
}

func (r *Runner) dispatchCronTask(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, state *cronDispatchState) (cronDispatchResult, error) {
	targetType := cronTargetType(cfg)
	switch targetType {
	case "model_registry_sync":
		if r.deps.RegistrySyncAgent == nil {
			return cronDispatchResult{}, validationErr("model registry sync agent not available")
		}
		if err := r.deps.RegistrySyncAgent.RunSync(ctx); err != nil {
			return cronDispatchResult{}, err
		}
		return cronDispatchResult{}, nil
	case "team":
		teamID := strings.TrimSpace(cfg.TeamID)
		if teamID == "" {
			return cronDispatchResult{}, validationErr("team_id is required for team cron target")
		}
		if _, err := r.deps.Teams.GetTeamByID(ctx, teamID); err != nil {
			return cronDispatchResult{}, err
		}
		sessID, err := r.ensureCronSession(ctx, state, biz.Session{
			OwnerType:  "team",
			TeamID:     teamID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		var res cronDispatchResult
		if r.deps.Chat != nil {
			if err := r.sessionBusyErr(sessID); err != nil {
				return cronDispatchResult{SessionID: sessID}, err
			}
			// EP-RT-07: in-process dispatch via plugin runtime.
			uid, aid, rerr := r.deps.Chat.RunCronTurn(ctx, sessID, cfg.Message, teamID)
			if rerr != nil {
				return cronDispatchResult{}, rerr
			}
			res = cronDispatchResult{SessionID: sessID, UserMessageID: uid, AgentMessageID: aid}
		} else {
			res, err = r.postChat(ctx, sendMessagePayload{
				SessionID: sessID,
				TeamID:    teamID,
				Content:   cfg.Message,
				Options:   sendMessageOptions{DialogMode: "cron"},
			})
			if err != nil {
				return cronDispatchResult{}, err
			}
		}
		r.publishTeamCronMaybe(ctx, teamID)
		return res, nil
	default:
		agent, err := r.resolveCronAgent(ctx, task)
		if err != nil {
			return cronDispatchResult{}, err
		}
		sessID, err := r.ensureCronSession(ctx, state, biz.Session{
			OwnerType:  "agent",
			AgentID:    agent.ID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		if r.deps.Chat != nil {
			if err := r.sessionBusyErr(sessID); err != nil {
				return cronDispatchResult{SessionID: sessID}, err
			}
			// EP-RT-07: in-process dispatch via plugin runtime.
			uid, aid, rerr := r.deps.Chat.RunCronTurn(ctx, sessID, cfg.Message, "")
			if rerr != nil {
				return cronDispatchResult{}, rerr
			}
			return cronDispatchResult{SessionID: sessID, UserMessageID: uid, AgentMessageID: aid}, nil
		}
		return r.postChat(ctx, sendMessagePayload{
			SessionID: sessID,
			AgentKey:  agent.AgentKey,
			Content:   cfg.Message,
			Options:   sendMessageOptions{DialogMode: "cron"},
		})
	}
}

// ensureCronSession returns the session ID to use for this dispatch invocation.
// If state already carries a session ID (set by a previous retry attempt), it is
// reused — this prevents retries from creating N orphan sessions for N attempts.
// Otherwise a new session is created via Session.Create and its ID is stored in
// state so subsequent retries reuse it.
func (r *Runner) ensureCronSession(ctx context.Context, state *cronDispatchState, template biz.Session) (string, error) {
	if state != nil && state.sessID != "" {
		return state.sessID, nil
	}
	sess, err := r.deps.Session.Create(ctx, template)
	if err != nil {
		return "", err
	}
	if state != nil {
		state.sessID = sess.ID
	}
	return sess.ID, nil
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
		if biz.BoolVal(agent.IsDefault) {
			return agent, nil
		}
	}
	if len(res.Items) > 0 {
		return res.Items[0], nil
	}
	return biz.Agent{}, validationErr("no agent available for cron task")
}

func (r *Runner) persistMetadata(ctx context.Context, task biz.CronTask, meta cronTaskMetadata) {
	raw, err := json.Marshal(meta)
	if err != nil {
		r.lg.Warn("marshal metadata failed", loggateway.Str("task_id", task.ID), loggateway.Err(err))
		return
	}
	// Acquire per-task lock to avoid racing with finalizeRun on the same task.
	unlock := r.lockTask(task.ID)
	defer unlock()
	// Reload task to avoid overwriting concurrent updates (e.g. from TriggerTask).
	current, err := r.deps.Cron.GetCronTask(ctx, task.ID)
	if err != nil {
		r.lg.Warn("reload task for metadata persist failed", loggateway.Str("task_id", task.ID), loggateway.Err(err))
		return
	}
	current.MetadataJSON = string(raw)
	if _, err := r.deps.Cron.UpdateCronTask(ctx, current); err != nil {
		r.lg.Warn("persist cron metadata failed", loggateway.Str("task_id", task.ID), loggateway.Err(err))
	}
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
	if r.deps.EventBus == nil {
		return
	}
	// Cron "team finished" hint: published as a v2 SystemNoticeEvent so the
	// frontend WS subscribers receive it. The v1 kind=team_stage/status=completed
	// hint is flattened into noticeType + meta (no real TeamStage entity exists).
	meta := map[string]any{
		"hint":      true,
		"stage":     "finished",
		"team_id":   teamID,
		"agent_key": "cron",
	}
	r.deps.EventBus.Publish(ctx, biz.NewSystemNoticeEvent("", "cron_team_finished", "cron team finished", meta))
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
		r.lg.Warn("解析 cron dispatch 响应失败", loggateway.StepID("cron.dispatch"), loggateway.Err(err))
		return cronDispatchResult{}, err
	}
	return cronDispatchResult{
		SessionID:      in.SessionID,
		UserMessageID:  out.UserMessage.ID,
		AgentMessageID: out.AgentMessage.ID,
	}, nil
}

func (r *Runner) sessionBusyErr(sessionID string) error {
	if r == nil || r.deps.Chat == nil {
		return nil
	}
	ctrl, ok := r.deps.Chat.(SessionRunControl)
	if !ok || !ctrl.HasActiveRun(sessionID) {
		return nil
	}
	r.lg.With(loggateway.SessionID(sessionID)).Warn("定时任务跳过：会话有活跃 Run",
		loggateway.StepID("cron.dispatch_skipped"))
	return biz.ErrCronSessionBusy
}

func validationErr(format string, args ...any) error {
	return apierror.BadRequest(apierror.DomainCron, fmt.Sprintf(format, args...))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

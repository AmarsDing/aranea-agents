package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type CronRunner struct {
	repo repository.Store
	chat *ChatService
	mu   sync.Mutex
}

type cronTaskConfig struct {
	TargetType      string `json:"target_type"`
	TeamID          string `json:"team_id"`
	ScheduleType    string `json:"schedule_type"`
	CronExpression  string `json:"cron_expression"`
	IntervalSeconds int    `json:"interval_seconds"`
	RunAt           string `json:"run_at"`
	Timezone        string `json:"timezone"`
	Message         string `json:"message"`
}

type cronTaskMetadata struct {
	RunCount      int                  `json:"run_count"`
	SuccessCount  int                  `json:"success_count"`
	FailureCount  int                  `json:"failure_count"`
	LastRunAt     string               `json:"last_run_at"`
	LastRunStatus string               `json:"last_run_status"`
	LastError     string               `json:"last_error"`
	NextRunAt     string               `json:"next_run_at"`
	RecentFailure []cronFailureSummary `json:"recent_failures"`
}

type cronFailureSummary struct {
	StartedAt    string `json:"started_at"`
	ErrorMessage string `json:"error_message"`
}

func NewCronRunner(repo repository.Store, chat *ChatService) *CronRunner {
	return &CronRunner{repo: repo, chat: chat}
}

func (r *CronRunner) Start(ctx context.Context, interval time.Duration) {
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

func (r *CronRunner) runDue(ctx context.Context) {
	if !r.mu.TryLock() {
		return
	}
	defer r.mu.Unlock()

	tasks, err := r.repo.ListPlatformResources("cron-tasks")
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if !task.Enabled || task.Status != "active" {
			continue
		}
		cfg, err := parseCronTaskConfig(task.ConfigJSON)
		if err != nil || strings.TrimSpace(cfg.Message) == "" {
			r.recordSkipped(task, now, firstNonEmptyString(errorString(err), "cron message is required"))
			continue
		}
		meta := parseCronTaskMetadata(task.MetadataJSON)
		dueAt, due, err := cronTaskDueAt(task, cfg, meta, now)
		if err != nil {
			r.recordSkipped(task, now, err.Error())
			continue
		}
		if !due {
			if meta.NextRunAt == "" && !dueAt.IsZero() {
				meta.NextRunAt = dueAt.Format(time.RFC3339)
				r.updateCronTaskMetadata(task, meta)
			}
			continue
		}
		r.executeTask(ctx, task, cfg, meta, now)
	}
}

func (r *CronRunner) executeTask(ctx context.Context, task domain.PlatformResource, cfg cronTaskConfig, meta cronTaskMetadata, now time.Time) {
	run := domain.CronTaskRun{
		ID:         newID(),
		TaskID:     task.ID,
		Status:     "pending",
		StartedAt:  now.Format(time.RFC3339),
		OutputJSON: mustMarshalJSON(map[string]any{"trigger": "schedule"}),
		CreatedAt:  now.Format(time.RFC3339),
	}
	run, err := r.repo.AddCronTaskRun(run)
	if err != nil {
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
	run.FinishedAt = finishedAt.Format(time.RFC3339)
	run.OutputJSON = mustMarshalJSON(output)
	if execErr != nil {
		run.Status = "failure"
		run.ErrorMessage = execErr.Error()
	} else {
		run.Status = "success"
	}
	_, _ = r.repo.UpdateCronTaskRun(run)

	meta.RunCount++
	meta.LastRunAt = run.FinishedAt
	meta.LastRunStatus = run.Status
	if execErr != nil {
		meta.FailureCount++
		meta.LastError = execErr.Error()
		meta.RecentFailure = append([]cronFailureSummary{{
			StartedAt:    run.StartedAt,
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
	r.updateCronTaskMetadata(task, meta)
}

type cronDispatchResult struct {
	SessionID      string
	UserMessageID  string
	AgentMessageID string
}

func (r *CronRunner) dispatchCronTask(ctx context.Context, task domain.PlatformResource, cfg cronTaskConfig) (cronDispatchResult, error) {
	targetType := cronTargetType(cfg)
	switch targetType {
	case "team":
		teamID := strings.TrimSpace(cfg.TeamID)
		if teamID == "" {
			return cronDispatchResult{}, validationError("team_id is required for team cron target")
		}
		team, err := r.repo.GetTeamByID(teamID)
		if err != nil {
			return cronDispatchResult{}, err
		}
		session, err := r.repo.CreateSession(domain.Session{
			ID:         newID(),
			OwnerType:  "team",
			TeamID:     team.ID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		out, err := r.chat.Send(ctx, SendMessageInput{
			SessionID: session.ID,
			TeamID:    team.ID,
			Content:   cfg.Message,
			Options:   SendMessageOptions{DialogMode: "cron"},
		})
		return cronDispatchResult{SessionID: session.ID, UserMessageID: out.UserMessage.ID, AgentMessageID: out.AgentMessage.ID}, err
	default:
		agent, err := r.resolveCronAgent(task)
		if err != nil {
			return cronDispatchResult{}, err
		}
		session, err := r.repo.CreateSession(domain.Session{
			ID:         newID(),
			OwnerType:  "agent",
			AgentID:    agent.ID,
			Title:      task.Name,
			DialogMode: "cron",
			Status:     "active",
		})
		if err != nil {
			return cronDispatchResult{}, err
		}
		out, err := r.chat.Send(ctx, SendMessageInput{
			SessionID: session.ID,
			AgentKey:  agent.AgentKey,
			Content:   cfg.Message,
			Options:   SendMessageOptions{DialogMode: "cron"},
		})
		return cronDispatchResult{SessionID: session.ID, UserMessageID: out.UserMessage.ID, AgentMessageID: out.AgentMessage.ID}, err
	}
}

func (r *CronRunner) resolveCronAgent(task domain.PlatformResource) (domain.Agent, error) {
	if strings.TrimSpace(task.AgentID) != "" {
		return r.repo.GetAgentByID(task.AgentID)
	}
	agents, err := r.repo.ListAgents()
	if err != nil {
		return domain.Agent{}, err
	}
	for _, agent := range agents {
		if agent.IsDefault {
			return agent, nil
		}
	}
	if len(agents) > 0 {
		return agents[0], nil
	}
	return domain.Agent{}, validationError("no agent available for cron task")
}

func (r *CronRunner) recordSkipped(task domain.PlatformResource, now time.Time, message string) {
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
	r.updateCronTaskMetadata(task, meta)
}

func (r *CronRunner) updateCronTaskMetadata(task domain.PlatformResource, meta cronTaskMetadata) {
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	task.MetadataJSON = string(raw)
	_, _ = r.repo.UpdatePlatformResource(task)
}

func cronTaskDueAt(task domain.PlatformResource, cfg cronTaskConfig, meta cronTaskMetadata, now time.Time) (time.Time, bool, error) {
	if meta.NextRunAt != "" {
		next, err := time.Parse(time.RFC3339, meta.NextRunAt)
		if err == nil {
			return next, !next.After(now), nil
		}
	}
	next, err := nextCronRunAfter(cfg, now)
	if err != nil {
		return time.Time{}, false, err
	}
	if cfg.ScheduleType == "once" && !next.After(now) {
		return next, true, nil
	}
	if task.UpdatedAt != "" && cfg.ScheduleType != "once" {
		if updated, err := time.Parse(time.RFC3339, task.UpdatedAt); err == nil && updated.Before(now) {
			return next, !next.After(now), nil
		}
	}
	return next, false, nil
}

func nextCronRunAfter(cfg cronTaskConfig, after time.Time) (time.Time, error) {
	loc, _ := time.LoadLocation(firstNonEmptyString(cfg.Timezone, "UTC"))
	if loc == nil {
		loc = time.UTC
	}
	switch cfg.ScheduleType {
	case "once":
		runAt, err := parseCronRunAt(cfg.RunAt, loc)
		if err != nil {
			return time.Time{}, err
		}
		return runAt.UTC(), nil
	case "cron":
		return nextCronExpressionTime(cfg.CronExpression, after.In(loc), loc)
	default:
		seconds := cfg.IntervalSeconds
		if seconds <= 0 {
			seconds = 900
		}
		return after.Add(time.Duration(seconds) * time.Second).UTC(), nil
	}
}

func parseCronRunAt(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, validationError("run_at is required for once cron task")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, validationError("invalid run_at: %s", value)
}

func nextCronExpressionTime(expr string, after time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return time.Time{}, validationError("cron expression must have 5 fields")
	}
	start := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	for candidate := start; candidate.Before(start.AddDate(1, 0, 0)); candidate = candidate.Add(time.Minute) {
		if cronFieldMatches(parts[0], candidate.Minute(), 0, 59) &&
			cronFieldMatches(parts[1], candidate.Hour(), 0, 23) &&
			cronFieldMatches(parts[2], candidate.Day(), 1, 31) &&
			cronFieldMatches(parts[3], int(candidate.Month()), 1, 12) &&
			cronWeekdayMatches(parts[4], int(candidate.Weekday())) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, validationError("unable to find next cron time within one year")
}

func cronWeekdayMatches(field string, weekday int) bool {
	if cronFieldMatches(field, weekday, 0, 7) {
		return true
	}
	return weekday == 0 && strings.Contains(field, "7")
}

func cronFieldMatches(field string, value int, min int, max int) bool {
	field = strings.TrimSpace(field)
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(strings.TrimPrefix(part, "*/"))
			if err == nil && step > 0 && (value-min)%step == 0 {
				return true
			}
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, startErr := strconv.Atoi(bounds[0])
			end, endErr := strconv.Atoi(bounds[1])
			if startErr == nil && endErr == nil && value >= start && value <= end {
				return true
			}
			continue
		}
		number, err := strconv.Atoi(part)
		if err == nil && number >= min && number <= max && number == value {
			return true
		}
	}
	return false
}

func parseCronTaskConfig(raw string) (cronTaskConfig, error) {
	var cfg cronTaskConfig
	if err := json.Unmarshal([]byte(defaultJSON(raw)), &cfg); err != nil {
		return cfg, err
	}
	if cfg.ScheduleType == "" {
		cfg.ScheduleType = "interval"
	}
	return cfg, nil
}

func parseCronTaskMetadata(raw string) cronTaskMetadata {
	var meta cronTaskMetadata
	_ = json.Unmarshal([]byte(defaultJSON(raw)), &meta)
	return meta
}

func cronTargetType(cfg cronTaskConfig) string {
	target := strings.ToLower(strings.TrimSpace(cfg.TargetType))
	if target == "" && strings.TrimSpace(cfg.TeamID) != "" {
		target = "team"
	}
	if target == "" {
		target = "agent"
	}
	return target
}

func mustMarshalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

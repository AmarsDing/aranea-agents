package cronrunner

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/strutil"
)

type cronTaskConfig struct {
	TargetType       string `json:"target_type"`
	TeamID           string `json:"team_id"`
	ScheduleType     string `json:"schedule_type"`
	CronExpression   string `json:"cron_expression"`
	IntervalSeconds  int    `json:"interval_seconds"`
	RunAt            string `json:"run_at"`
	Timezone         string `json:"timezone"`
	Message          string `json:"message"`
	RetryMaxAttempts *int `json:"retry_max_attempts"`
}

const defaultRetryMaxAttempts = 3

// effectiveRetryMaxAttempts returns configured retries after first attempt: nil/missing → 3, 0 → disable.
func effectiveRetryMaxAttempts(cfg cronTaskConfig) int {
	if cfg.RetryMaxAttempts == nil {
		return defaultRetryMaxAttempts
	}
	return *cfg.RetryMaxAttempts
}

// retryPlan returns total attempts (initial + retries) and backoff delays.
// maxRetries is the number of retries after the first attempt; 0 disables retry.
func retryPlan(maxRetries int) (attempts int, backoff []time.Duration) {
	if maxRetries <= 0 {
		return 1, nil
	}
	backoff = defaultRetryBackoff
	if maxRetries < len(defaultRetryBackoff) {
		backoff = defaultRetryBackoff[:maxRetries]
	}
	return len(backoff) + 1, backoff
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

func defaultJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	return raw
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

func cronTaskDueAt(updatedAt string, cfg cronTaskConfig, meta cronTaskMetadata, now time.Time) (time.Time, bool, error) {
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
	if updatedAt != "" && cfg.ScheduleType != "once" {
		if updated, err := time.Parse(time.RFC3339, updatedAt); err == nil && updated.Before(now) {
			return next, !next.After(now), nil
		}
	}
	return next, false, nil
}

func nextCronRunAfter(cfg cronTaskConfig, after time.Time) (time.Time, error) {
	loc, _ := time.LoadLocation(strutil.FirstNonEmpty(cfg.Timezone, "UTC"))
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
		return time.Time{}, errRunAtRequired
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errInvalidRunAt
}

func nextCronExpressionTime(expr string, after time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return time.Time{}, errCronFields
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
	return time.Time{}, errCronNoSlot
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

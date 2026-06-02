package biz

import (
	"aranea-agents/internal/event/contract"
	"context"
	"strings"
	"time"
)

// AutoMemoryEnqueuer abstracts the auto-memory job queue so biz does not
// depend on internal/memory/trpc directly.
type AutoMemoryEnqueuer interface {
	EnqueueAutoMemory(appName, sessionID string, enqueuedAt time.Time)
}

// FeedbackMemoryEnqueuer enqueues preference extraction from user message feedback.
type FeedbackMemoryEnqueuer interface {
	EnqueueFeedbackMemory(sessionID, messageID, rating, comment string, enqueuedAt time.Time)
}

// FeedbackMemoryEnqueuerFunc is a function adapter for FeedbackMemoryEnqueuer.
type FeedbackMemoryEnqueuerFunc func(sessionID, messageID, rating, comment string, enqueuedAt time.Time)

func (f FeedbackMemoryEnqueuerFunc) EnqueueFeedbackMemory(sessionID, messageID, rating, comment string, enqueuedAt time.Time) {
	f(sessionID, messageID, rating, comment, enqueuedAt)
}

// AutoMemoryEnqueuerFunc is a function adapter for AutoMemoryEnqueuer.
type AutoMemoryEnqueuerFunc func(appName, sessionID string, enqueuedAt time.Time)

func (f AutoMemoryEnqueuerFunc) EnqueueAutoMemory(appName, sessionID string, enqueuedAt time.Time) {
	f(appName, sessionID, enqueuedAt)
}

// SessionLogWriter abstracts session-scoped log writing so biz does not
// depend on internal/event directly for logging helpers.
type SessionLogWriter interface {
	LogSessionWarn(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
	LogSessionError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
}

// SessionLogWriterFunc is a function adapter for SessionLogWriter.
// Since SessionLogWriter has two methods, use the full adapter from the event package instead.
type SessionLogWriterFunc struct {
	WarnFn  func(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
	ErrorFn func(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
}

func (a SessionLogWriterFunc) LogSessionWarn(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair) {
	a.WarnFn(ctx, sessionID, stepID, message, pairs...)
}

func (a SessionLogWriterFunc) LogSessionError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair) {
	a.ErrorFn(ctx, sessionID, stepID, message, pairs...)
}

// LogPair is a key-value pair for structured logging, replacing event.P.
type LogPair struct {
	Key   string
	Value any
}

// EnvelopeBuffer abstracts the in-process replay buffer so biz does not
// depend on internal/event.Buffer directly.
type EnvelopeBuffer interface {
	Append(env contract.Envelope)
}

// SystemLogWriter abstracts system-domain (non-session-scoped) log writing
// so biz does not depend on internal/event.SysLogWarn/Error directly.
type SystemLogWriter interface {
	LogWarn(stepID, message string, pairs ...LogPair)
	LogError(stepID, message string, pairs ...LogPair)
}

// SystemLogWriterFunc is a function adapter for SystemLogWriter.
type SystemLogWriterFunc struct {
	WarnFn  func(stepID, message string, pairs ...LogPair)
	ErrorFn func(stepID, message string, pairs ...LogPair)
}

func (a SystemLogWriterFunc) LogWarn(stepID, message string, pairs ...LogPair) {
	a.WarnFn(stepID, message, pairs...)
}

func (a SystemLogWriterFunc) LogError(stepID, message string, pairs ...LogPair) {
	a.ErrorFn(stepID, message, pairs...)
}

// TurnMemoryWorker handles post-turn feedback memory extraction (EP-MEM-01).
// Auto-memory enqueue is handled by the framework layer (runner.enqueueAutoMemoryJob
// → memory.Service.EnqueueAutoMemoryJob), so this worker only manages feedback-triggered
// preference extraction to avoid duplicate queue entries.
type TurnMemoryWorker struct {
	feedbackEnqueuer FeedbackMemoryEnqueuer
	logger           SessionLogWriter
}

// NewTurnMemoryWorker constructs a turn memory worker.
func NewTurnMemoryWorker(feedback FeedbackMemoryEnqueuer, logger SessionLogWriter) *TurnMemoryWorker {
	return &TurnMemoryWorker{feedbackEnqueuer: feedback, logger: logger}
}

// OnRunnerCompletion logs runner completion for observability.
// Auto-memory enqueue is handled by the framework layer; this method no longer
// duplicates the enqueue to prevent double-processing of the same session.
func (w *TurnMemoryWorker) OnRunnerCompletion(ctx context.Context, de DomainEvent) {
	sid := strings.TrimSpace(de.SessionID)
	if sid == "" {
		return
	}
	if w.logger != nil {
		w.logger.LogSessionWarn(ctx, sid, "system.memory_worker.runner_complete", "Runner 完成，自动记忆由框架层入队")
	}
}

// OnUserFeedback enqueues preference memory extraction from thumbs up/down events.
func (w *TurnMemoryWorker) OnUserFeedback(ctx context.Context, sessionID, messageID, rating, comment string) {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	rating = strings.TrimSpace(rating)
	if sessionID == "" || messageID == "" || rating == "" {
		return
	}
	if w.feedbackEnqueuer != nil {
		w.feedbackEnqueuer.EnqueueFeedbackMemory(sessionID, messageID, rating, comment, time.Now().UTC())
	}
	if w.logger != nil {
		w.logger.LogSessionWarn(ctx, sessionID, "system.memory_worker.feedback_enqueue", "反馈偏好记忆任务已入队",
			LogPair{Key: "message_id", Value: messageID}, LogPair{Key: "rating", Value: rating})
	}
}

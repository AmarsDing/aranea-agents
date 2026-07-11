// Package memory provides the Sleep-time Agent that asynchronously
// consolidates memories (merge duplicates, extract reflections, update core
// memory) in the background. This aligns with the Letta/MemGPT "Sleep-time
// Agent" pattern.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// defaultConsolidationLimit is the default number of recent memories to
	// read for a single consolidation pass.
	defaultConsolidationLimit = 50
	// coreMemoryTopic is the topic tag used to identify core memory entries.
	coreMemoryTopic = "core"
	// consolidationQueueDefaultSize is the default buffer size for the
	// in-memory consolidation queue.
	consolidationQueueDefaultSize = 100
)

// MemoryReaderWriter is the subset of trpcmemory.Service needed by
// SleepTimeService. Defining a narrow interface here follows the
// interface-segregation principle and makes testing easier.
type MemoryReaderWriter interface {
	ReadMemories(ctx context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error)
	AddMemory(ctx context.Context, uk trpcmemory.UserKey, memory string, topics []string, opts ...trpcmemory.AddOption) error
	UpdateMemory(ctx context.Context, mk trpcmemory.Key, memory string, topics []string, opts ...trpcmemory.UpdateOption) error
	DeleteMemory(ctx context.Context, mk trpcmemory.Key) error
}

// ConsolidationJobRequest represents a queued memory consolidation job.
type ConsolidationJobRequest struct {
	UserKey    trpcmemory.UserKey
	EnqueuedAt time.Time
}

// ConsolidationQueue is the queue interface consumed by SleepTimeService.
// Implementations must be safe for concurrent use.
type ConsolidationQueue interface {
	Enqueue(r ConsolidationJobRequest)
	Chan() <-chan ConsolidationJobRequest
}

// ConsolidationOperation represents a single operation produced by the LLM
// during consolidation. The Type field selects the operation:
//   - "merge":       update TargetID with MergedContent, then delete SourceIDs
//   - "reflect":     add Reflection as a new memory with Topics
//   - "update_core": add each Update as a core memory entry
type ConsolidationOperation struct {
	Type          string             `json:"type"`
	TargetID      string             `json:"target_id,omitempty"`
	SourceIDs     []string           `json:"source_ids,omitempty"`
	MergedContent string             `json:"merged_content,omitempty"`
	MergedTopics  []string           `json:"merged_topics,omitempty"`
	Reflection    string             `json:"reflection,omitempty"`
	Topics        []string           `json:"topics,omitempty"`
	Updates       []CoreMemoryUpdate `json:"updates,omitempty"`
}

// CoreMemoryUpdate represents a single core memory key-value update.
type CoreMemoryUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ConsolidationResult holds the LLM-produced consolidation operations.
type ConsolidationResult struct {
	Operations []ConsolidationOperation `json:"operations"`
}

// SleepTimeService asynchronously consolidates memories (merge duplicates,
// extract reflections, update core memory) in the background.
//
// It follows the Letta/MemGPT "Sleep-time Agent" pattern: after active
// conversation turns, memories are consolidated offline by an LLM that
// analyses recent entries and produces merge/reflect/update_core operations.
//
// Phase 6A-06: an optional EpisodeConsolidator can be wired via
// SetEpisodeConsolidator to additionally extract durable L3 facts from L2
// episodes during each consolidation pass. The episode phase is independent
// of the trpcmemory phase and runs best-effort (never fails the parent call).
type SleepTimeService struct {
	memory              MemoryReaderWriter
	llm                 trpcmodel.Model
	queue               ConsolidationQueue
	episodeConsolidator *EpisodeConsolidator // Phase 6A-06: optional L2→L3 fact extraction
	lg                  loggateway.Logger
}

// NewSleepTimeService creates a SleepTimeService.
//
// Parameters:
//   - memory: the memory store used to read and mutate memories.
//   - llm:    the LLM used for consolidation analysis. May be nil (no-op).
//   - queue:  the queue for async consolidation jobs. May be nil.
//   - lg:     the logger. Falls back to a no-op logger if nil.
func NewSleepTimeService(memory MemoryReaderWriter, llm trpcmodel.Model, queue ConsolidationQueue, lg loggateway.Logger) *SleepTimeService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SleepTimeService{
		memory: memory,
		llm:    llm,
		queue:  queue,
		lg:     lg,
	}
}

// SetEpisodeConsolidator wires an EpisodeConsolidator for L2→L3 fact
// extraction (Phase 6A-06). Optional: when not set, Consolidate only runs
// the trpcmemory merge/reflect/update_core phase. Must be called before
// Start/Consolidate; not safe to call concurrently with active consolidation.
func (s *SleepTimeService) SetEpisodeConsolidator(c *EpisodeConsolidator) {
	if s == nil {
		return
	}
	s.episodeConsolidator = c
}

// EnqueueConsolidationJob enqueues a consolidation job for the given user key.
// Returns nil when no queue is wired (no-op). Respects context cancellation.
func (s *SleepTimeService) EnqueueConsolidationJob(ctx context.Context, uk trpcmemory.UserKey) error {
	if s == nil || s.queue == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.queue.Enqueue(ConsolidationJobRequest{
		UserKey:    uk,
		EnqueuedAt: time.Now(),
	})
	return nil
}

// Consolidate runs both consolidation phases:
//
//  1. trpcmemory consolidation (Letta-style merge/reflect/update_core) —
//     operates on trpcmemory.Entry items. Read failures are retryable
//     (return error); mutation failures are non-retryable (return nil).
//
//  2. L2 episode → L3 fact extraction (Phase 6A-06) — operates on project
//     L2 episodes. Best-effort: never returns an error, runs independently
//     of Phase 1. Only runs if an EpisodeConsolidator is wired.
//
// Phase 2 runs even when Phase 1 has no memories to consolidate, because
// episodes are an independent data source. Phase 2 is skipped only when
// Phase 1 returns a retryable error (so JobRunner retry doesn't produce
// duplicate facts).
//
// Behaviour:
//   - Empty memories + no episodes → no-op (returns nil).
//   - LLM failure → graceful degradation (logs warn, returns nil).
//   - Memory read failure → returns the error (retryable).
//   - Memory mutation failure → returns nil (non-retryable).
func (s *SleepTimeService) Consolidate(ctx context.Context, uk trpcmemory.UserKey) error {
	if s == nil {
		return nil
	}

	// Phase 1: trpcmemory consolidation.
	if s.memory != nil {
		if err := s.consolidateMemories(ctx, uk); err != nil {
			// Retryable read failure — skip Phase 2 to avoid duplicate facts on retry.
			return err
		}
	}

	// Phase 2: L2 episode → L3 fact extraction (Phase 6A-06).
	// Best-effort: never returns an error.
	s.consolidateEpisodes(ctx, uk)

	return nil
}

// consolidateMemories runs the Letta-style trpcmemory consolidation phase.
// Returns an error only on read failure (retryable); mutation failures are
// treated as graceful degradation (return nil, non-retryable).
func (s *SleepTimeService) consolidateMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	// 1. Read recent memories.
	memories, err := s.memory.ReadMemories(ctx, uk, defaultConsolidationLimit)
	if err != nil {
		s.lg.Warn("sleep-time read memories failed",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return err
	}
	// Empty memories → no-op (but Phase 2 may still run).
	if len(memories) == 0 {
		return nil
	}

	// 2. LLM analysis: merge duplicates, extract reflections, update core memory.
	result, err := s.llmConsolidate(ctx, memories)
	if err != nil {
		// Graceful degradation: log warn, no panic, no error returned.
		s.lg.Warn("sleep-time LLM consolidation failed, skipping",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil
	}

	// 3. Execute operations.
	//
	// T3.1: Mutation failures are treated as graceful degradation (return nil)
	// rather than returning the error. This ensures JobRunner retry is safe:
	// only read failures (step 1) return an error, and reads are idempotent.
	// Mutation operations (merge/reflect/update_core) are NOT idempotent —
	// retrying them could add duplicate reflections or core memories.
	// By returning nil on mutation failure, we prevent unsafe retries while
	// still logging the failure for observability.
	if err := s.executeOperations(ctx, uk, result.Operations); err != nil {
		s.lg.Warn("sleep-time execute operations failed, skipping (non-retryable)",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil
	}

	s.lg.Info("sleep-time consolidation completed",
		loggateway.Str("app", uk.AppName),
		loggateway.Str("user", uk.UserID),
		loggateway.Int("operations", len(result.Operations)),
		loggateway.Int("memories_scanned", len(memories)))
	return nil
}

// consolidateEpisodes runs the L2→L3 fact extraction phase (Phase 6A-06).
// Best-effort: logs warnings on failure but never returns an error, so it
// never affects JobRunner retry decisions.
func (s *SleepTimeService) consolidateEpisodes(ctx context.Context, uk trpcmemory.UserKey) {
	if s == nil || s.episodeConsolidator == nil {
		return
	}
	if err := s.episodeConsolidator.ConsolidateEpisodes(ctx, uk); err != nil {
		s.lg.Warn("sleep-time episode consolidation failed",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
	}
}

// Start runs the worker loop that drains the consolidation queue and calls
// Consolidate for each job. Blocks until ctx is cancelled.
//
// Note: callers that need retry/dead-letter semantics should use QueueChan()
// to drain the queue themselves (e.g. cronrunner/jobs.MemorySleepTimeWorker
// wraps each job in a JobRunner for retry + panic recovery + dead-letter
// metrics).
func (s *SleepTimeService) Start(ctx context.Context) {
	if s == nil || s.queue == nil {
		return
	}
	ch := s.queue.Chan()
	if ch == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-ch:
			s.processJob(ctx, req)
		}
	}
}

// QueueChan returns the consolidation queue channel for external consumers.
// Returns nil when no queue is wired. Callers (e.g. MemorySleepTimeWorker)
// can drain the queue themselves to wrap each job in a JobRunner for retry
// and dead-letter metrics.
func (s *SleepTimeService) QueueChan() <-chan ConsolidationJobRequest {
	if s == nil || s.queue == nil {
		return nil
	}
	return s.queue.Chan()
}

func (s *SleepTimeService) processJob(ctx context.Context, req ConsolidationJobRequest) {
	if err := s.Consolidate(ctx, req.UserKey); err != nil {
		s.lg.Warn("sleep-time job failed",
			loggateway.Str("app", req.UserKey.AppName),
			loggateway.Str("user", req.UserKey.UserID),
			loggateway.Err(err))
	}
}

// llmConsolidate calls the LLM with the memories and parses the JSON response.
// Returns an empty result (no operations) when the LLM is not wired.
func (s *SleepTimeService) llmConsolidate(ctx context.Context, memories []*trpcmemory.Entry) (*ConsolidationResult, error) {
	if s.llm == nil {
		return &ConsolidationResult{}, nil
	}

	prompt := buildConsolidationPrompt(memories)
	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: consolidationSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respCh, err := s.llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, err
	}

	var content string
	for resp := range respCh {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("LLM API error: %s", resp.Error.Message)
		}
		for _, choice := range resp.Choices {
			content += choice.Message.Content
		}
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return &ConsolidationResult{}, nil
	}

	var result ConsolidationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse LLM consolidation response: %w", err)
	}
	return &result, nil
}

// executeOperations applies the consolidation operations to the memory store.
func (s *SleepTimeService) executeOperations(ctx context.Context, uk trpcmemory.UserKey, ops []ConsolidationOperation) error {
	for _, op := range ops {
		switch op.Type {
		case "merge":
			if err := s.executeMerge(ctx, uk, op); err != nil {
				return err
			}
		case "reflect":
			if err := s.executeReflect(ctx, uk, op); err != nil {
				return err
			}
		case "update_core":
			if err := s.executeUpdateCore(ctx, uk, op); err != nil {
				return err
			}
		default:
			// Unknown op type — log to surface prompt/response contract drift.
			// LLM may emit typos (e.g. "merg") or new types not yet supported.
			s.lg.Warn("sleep-time unknown consolidation op type skipped",
				loggateway.Str("op_type", op.Type),
				loggateway.Str("target_id", op.TargetID))
		}
	}
	return nil
}

func (s *SleepTimeService) executeMerge(ctx context.Context, uk trpcmemory.UserKey, op ConsolidationOperation) error {
	if op.TargetID == "" || strings.TrimSpace(op.MergedContent) == "" {
		return nil
	}
	mk := trpcmemory.Key{AppName: uk.AppName, UserID: uk.UserID, MemoryID: op.TargetID}
	if err := s.memory.UpdateMemory(ctx, mk, op.MergedContent, op.MergedTopics); err != nil {
		return err
	}
	for _, srcID := range op.SourceIDs {
		srcKey := trpcmemory.Key{AppName: uk.AppName, UserID: uk.UserID, MemoryID: srcID}
		if err := s.memory.DeleteMemory(ctx, srcKey); err != nil {
			// Log but continue — partial merge is acceptable.
			s.lg.Warn("sleep-time merge delete source failed",
				loggateway.Str("source_id", srcID),
				loggateway.Err(err))
		}
	}
	return nil
}

func (s *SleepTimeService) executeReflect(ctx context.Context, uk trpcmemory.UserKey, op ConsolidationOperation) error {
	reflection := strings.TrimSpace(op.Reflection)
	if reflection == "" {
		return nil
	}
	return s.memory.AddMemory(ctx, uk, reflection, op.Topics)
}

func (s *SleepTimeService) executeUpdateCore(ctx context.Context, uk trpcmemory.UserKey, op ConsolidationOperation) error {
	for _, u := range op.Updates {
		value := strings.TrimSpace(u.Value)
		if value == "" {
			continue
		}
		content := value
		if u.Key != "" {
			content = fmt.Sprintf("%s: %s", u.Key, value)
		}
		if err := s.memory.AddMemory(ctx, uk, content, []string{coreMemoryTopic}); err != nil {
			return err
		}
	}
	return nil
}

// buildConsolidationPrompt constructs the user prompt containing the memories
// to consolidate, serialised as a JSON array.
func buildConsolidationPrompt(memories []*trpcmemory.Entry) string {
	type memItem struct {
		ID     string   `json:"id"`
		Memory string   `json:"memory"`
		Topics []string `json:"topics,omitempty"`
	}
	items := make([]memItem, 0, len(memories))
	for _, m := range memories {
		if m == nil || m.Memory == nil {
			continue
		}
		items = append(items, memItem{
			ID:     m.ID,
			Memory: m.Memory.Memory,
			Topics: m.Memory.Topics,
		})
	}
	b, err := json.Marshal(items)
	if err != nil {
		// memItem is a plain struct of strings/slices — marshal failure is
		// unexpected. Return an empty prompt; the caller (llmConsolidate)
		// treats an empty user prompt as "no items" and returns no operations.
		return ""
	}
	return "Memories to consolidate:\n" + string(b)
}

const consolidationSystemPrompt = `You are a memory consolidation agent. Analyse the provided memories and produce consolidation operations as JSON.

Return a JSON object with an "operations" array. Each operation has a "type" field:
- "merge": Merge duplicate or highly similar memories. Provide "target_id" (ID of memory to update), "source_ids" (IDs to delete after merge), "merged_content" (new content), and optionally "merged_topics".
- "reflect": Extract a higher-level reflection or insight. Provide "reflection" (text) and "topics" (array of strings).
- "update_core": Update core memory (key facts). Provide "updates" (array of {"key": "...", "value": "..."} objects).

Only produce operations when clearly warranted. If no consolidation is needed, return {"operations": []}.`

// consolidationQueue is a simple in-memory implementation of ConsolidationQueue.
type consolidationQueue struct {
	ch chan ConsolidationJobRequest
}

// NewConsolidationQueue creates a new in-memory consolidation queue with the
// given buffer size. Falls back to a default size when bufferSize <= 0.
func NewConsolidationQueue(bufferSize int) ConsolidationQueue {
	if bufferSize <= 0 {
		bufferSize = consolidationQueueDefaultSize
	}
	return &consolidationQueue{
		ch: make(chan ConsolidationJobRequest, bufferSize),
	}
}

func (q *consolidationQueue) Enqueue(r ConsolidationJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	select {
	case q.ch <- r:
	default:
		// Queue full — drop the job. A dropped consolidation job is
		// acceptable: the next cron tick will re-enqueue.
	}
}

func (q *consolidationQueue) Chan() <-chan ConsolidationJobRequest {
	if q == nil {
		return nil
	}
	return q.ch
}

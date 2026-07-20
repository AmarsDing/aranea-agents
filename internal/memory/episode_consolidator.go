// Package memory provides the Episode Consolidator that extracts durable L3
// facts from L2 episodes during Sleep-Time consolidation (Phase 6A-06).
//
// This is the Letta-style "Sleep-Time Agent" extension: instead of operating
// on raw trpcmemory.Entry items (handled by SleepTimeService), the
// EpisodeConsolidator operates on project L2 Episodes — which have richer
// metadata (title, goal, outcome, importance, key_decisions) — to extract
// durable, reusable L3 semantic facts.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// episodeConsolidationLimit caps the number of recent episodes read per pass.
	episodeConsolidationLimit = 30
	// episodeConsolidationLLMTimeout caps the LLM call duration.
	episodeConsolidationLLMTimeout = 45 * time.Second
	// episodeConsolidationScopeType is the scope type for agent-scoped facts.
	episodeConsolidationScopeType = "agent"
	// defaultEpisodeFactMinImportance is the default minimum importance for an
	// extracted fact. Can be overridden via SetMinImportance (Phase 6A-06 T8).
	defaultEpisodeFactMinImportance = 0.3
)

// EpisodeConsolidator integrates L2 episodes into L3 facts using LLM analysis.
// It reads recent episodes for an agent, asks the LLM to extract durable facts,
// and persists them via L3FactWriter. All operations are best-effort with
// graceful degradation (LLM failure → no-op, write failure → warn log).
//
// Phase 6A-06 P0: episode-to-fact extraction with action_log audit trail.
// Phase 6A-06 T8: configurable min importance via SetMinImportance.
type EpisodeConsolidator struct {
	episodeReader   biz.L2RecallStore
	factWriter      biz.L3FactWriter
	actionLogWriter biz.MemoryActionLogWriter
	llm             trpcmodel.Model
	lg              loggateway.Logger
	// T8: minimum importance threshold for extracted facts. Facts with
	// importance below this value are skipped. Default: 0.3.
	minImportance float64
}

// NewEpisodeConsolidator creates an EpisodeConsolidator.
//
// Parameters:
//   - episodeReader:   the L2 episode reader for retrieving recent episodes.
//   - factWriter:      the L3 fact writer for persisting extracted facts.
//   - actionLogWriter: the action log writer for audit. May be nil (skip audit).
//   - llm:             the LLM for fact extraction. May be nil (no-op).
//   - lg:              the logger. Falls back to a no-op logger if nil.
func NewEpisodeConsolidator(
	episodeReader biz.L2RecallStore,
	factWriter biz.L3FactWriter,
	actionLogWriter biz.MemoryActionLogWriter,
	llm trpcmodel.Model,
	lg loggateway.Logger,
) *EpisodeConsolidator {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &EpisodeConsolidator{
		episodeReader:   episodeReader,
		factWriter:      factWriter,
		actionLogWriter: actionLogWriter,
		llm:             llm,
		lg:              lg.With(loggateway.Domain("episode_consolidator")),
		minImportance:   defaultEpisodeFactMinImportance,
	}
}

// SetMinImportance sets the minimum importance threshold for extracted facts
// (Phase 6A-06 T8). Facts with importance below this value are skipped.
// Default: 0.3. Values <= 0 disable the filter (all facts are kept).
func (c *EpisodeConsolidator) SetMinImportance(min float64) {
	if c == nil {
		return
	}
	c.minImportance = min
}

// ConsolidateEpisodes reads recent L2 episodes for the given agent/user,
// extracts durable L3 facts via LLM analysis, and persists them.
//
// Behaviour:
//   - nil LLM → returns nil (no-op, warn log).
//   - nil episodeReader or factWriter → returns nil (no-op, warn log).
//   - No episodes → returns nil (no-op).
//   - LLM failure → returns nil (graceful degradation, warn log).
//   - Fact write failure → logs warn and continues (best-effort).
func (c *EpisodeConsolidator) ConsolidateEpisodes(ctx context.Context, uk trpcmemory.UserKey) error {
	if c == nil {
		return nil
	}
	if c.llm == nil {
		c.lg.Warn("episode consolidation skipped: nil LLM",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID))
		return nil
	}
	if c.episodeReader == nil || c.factWriter == nil {
		c.lg.Warn("episode consolidation skipped: nil episode reader or fact writer",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID))
		return nil
	}

	// 1. Read recent L2 episodes for the agent (across all sessions).
	rows, err := c.episodeReader.ListEpisodeRowsForRecall(ctx, uk.AppName, "", episodeConsolidationLimit)
	if err != nil {
		c.lg.Warn("episode consolidation: read episodes failed",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil
	}
	if len(rows) == 0 {
		return nil
	}

	// 2. Parse episode rows.
	var episodes []episodeData
	for _, raw := range rows {
		ep, parseErr := parseEpisodeData(raw)
		if parseErr != nil {
			continue
		}
		if strings.TrimSpace(ep.Title) == "" {
			continue
		}
		episodes = append(episodes, ep)
	}
	if len(episodes) == 0 {
		return nil
	}

	// 3. LLM analysis: extract durable facts from episodes.
	result, err := c.llmExtractFacts(ctx, episodes)
	if err != nil {
		c.lg.Warn("episode consolidation: LLM extraction failed, skipping",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil
	}
	if len(result.Facts) == 0 {
		return nil
	}

	// 4. Persist extracted facts via L3FactWriter.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	extractedCount := 0
	for _, fact := range result.Facts {
		if c.minImportance > 0 && fact.Importance < c.minImportance {
			continue
		}
		upsert := biz.FactUpsert{
			ScopeType:       episodeConsolidationScopeType,
			ScopeID:         uk.AppName,
			UserID:          uk.UserID,
			AgentID:         uk.AppName,
			Statement:       strings.TrimSpace(fact.Statement),
			FactKind:        "fact",
			Confidence:      fact.Confidence,
			Importance:      fact.Importance,
			SourceKind:      "episode",
			SourceEpisodeID: fact.SourceEpisodeID,
			SourceSessionID: "",
			Version:         1,
			Status:          "active",
			CreatedAt:       now,
			UpdatedAt:       now,
			ValidFrom:       now,
		}
		if _, upErr := c.factWriter.UpsertFactRow(ctx, upsert); upErr != nil {
			c.lg.Warn("episode consolidation: fact upsert failed",
				loggateway.Str("app", uk.AppName),
				loggateway.Str("statement", truncate(fact.Statement, 80)),
				loggateway.Err(upErr))
			continue
		}
		extractedCount++

		// action_log: audit trail for each extracted fact.
		c.writeActionLog(ctx, fact, uk)
	}

	c.lg.Info("episode consolidation completed",
		loggateway.Str("app", uk.AppName),
		loggateway.Str("user", uk.UserID),
		loggateway.Int("episodes_scanned", len(episodes)),
		loggateway.Int("facts_extracted", extractedCount))
	return nil
}

// writeActionLog writes an action_log entry for an extracted fact.
// Best-effort: failures are logged as warnings.
func (c *EpisodeConsolidator) writeActionLog(ctx context.Context, fact extractedFact, uk trpcmemory.UserKey) {
	if c.actionLogWriter == nil {
		return
	}
	rec := biz.MemoryPolicyRecord{
		Action:        "episode_extract_fact",
		TargetKind:    "memory_fact",
		TargetID:      fact.SourceEpisodeID,
		Reason:        fact.Reason,
		PolicyVersion: "episode_consolidation_v1",
	}
	if err := c.actionLogWriter.WriteMemoryActionLog(ctx, rec); err != nil {
		c.lg.Warn("episode consolidation: action_log write failed",
			loggateway.Str("target", fact.SourceEpisodeID),
			loggateway.Err(err))
	}
}

// llmExtractFacts calls the LLM with the episodes and parses the JSON response.
func (c *EpisodeConsolidator) llmExtractFacts(ctx context.Context, episodes []episodeData) (*extractionResult, error) {
	prompt := buildExtractionPrompt(episodes)
	callCtx, cancel := context.WithTimeout(ctx, episodeConsolidationLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: extractionSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respCh, err := c.llm.GenerateContent(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generate content: %w", err)
	}

	content, err := consumeLLMResponse(respCh)
	if err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return &extractionResult{}, nil
	}

	var result extractionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse LLM extraction response: %w", err)
	}
	return &result, nil
}

// buildExtractionPrompt constructs the user prompt containing the episodes.
func buildExtractionPrompt(episodes []episodeData) string {
	type epItem struct {
		ID             string  `json:"id"`
		Title          string  `json:"title"`
		OutcomeSummary string  `json:"outcome_summary,omitempty"`
		Importance     float64 `json:"importance,omitempty"`
	}
	items := make([]epItem, 0, len(episodes))
	for _, ep := range episodes {
		items = append(items, epItem{
			ID:             ep.ID,
			Title:          ep.Title,
			OutcomeSummary: ep.OutcomeSummary,
			Importance:     ep.Importance,
		})
	}
	b, _ := json.Marshal(items)
	return "Episodes to analyse:\n" + string(b)
}

const extractionSystemPrompt = `You are a memory extraction agent. Given a list of L2 episodes (each with an id, title, outcome_summary, and importance), your task is to extract durable, reusable L3 semantic facts that will remain useful beyond the specific episode.

Return a JSON object with this schema:
{"facts": [{"statement": "durable fact", "importance": 0.8, "confidence": 0.9, "source_episode_id": "episode-id", "reason": "why this fact was extracted"}]}

Rules:
- Extract facts that are durable (user preferences, entity attributes, reusable knowledge) — NOT ephemeral task details.
- Each fact must reference its source_episode_id from the input.
- Importance ∈ [0, 1]: how useful this fact is for future conversations.
- Confidence ∈ [0, 1]: how certain you are that this fact is accurate.
- Only extract facts with importance >= 0.3. Skip low-value extractions.
- If no durable facts can be extracted, return {"facts": []}.
- Output ONLY the JSON object, no markdown fences or explanations.`

// episodeData is a minimal struct for parsing L2 episode JSON.
type episodeData struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	AgentID        string  `json:"agent_id"`
	EpisodeKind    string  `json:"episode_kind"`
	Title          string  `json:"title"`
	OutcomeSummary string  `json:"outcome_summary"`
	Importance     float64 `json:"importance"`
	EndedAt        string  `json:"ended_at"`
	CreatedAt      string  `json:"created_at"`
}

// parseEpisodeData parses a raw JSON byte slice into episodeData.
func parseEpisodeData(raw []byte) (episodeData, error) {
	var ep episodeData
	if err := json.Unmarshal(raw, &ep); err != nil {
		return episodeData{}, err
	}
	return ep, nil
}

// extractedFact represents a single L3 fact extracted by the LLM.
type extractedFact struct {
	Statement       string  `json:"statement"`
	Importance      float64 `json:"importance"`
	Confidence      float64 `json:"confidence"`
	SourceEpisodeID string  `json:"source_episode_id"`
	Reason          string  `json:"reason,omitempty"`
}

// extractionResult holds the LLM-produced facts.
type extractionResult struct {
	Facts []extractedFact `json:"facts"`
}

// truncate returns s truncated to maxLen characters, with "..." appended if
// truncation occurred.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

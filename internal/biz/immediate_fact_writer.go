package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ImmediateFactWriter persists parsed facts to memory_fact immediately.
// This bridges the async gap between conversation and Sleep-time consolidation.
type ImmediateFactWriter struct {
	lg         loggateway.Logger
	factWriter MemoryConsolidationWriter
}

// NewImmediateFactWriter creates a new ImmediateFactWriter.
func NewImmediateFactWriter(factWriter MemoryConsolidationWriter, lg loggateway.Logger) *ImmediateFactWriter {
	if factWriter == nil {
		return nil
	}
	return &ImmediateFactWriter{
		lg:         lg,
		factWriter: factWriter,
	}
}

// WriteFacts persists facts asynchronously (fire-and-forget).
// The goroutine uses context.WithoutCancel to avoid being cancelled by request context.
func (w *ImmediateFactWriter) WriteFacts(ctx context.Context, sessionID, agentID, userID, sourceMessageID string, facts []FactMark) {
	if w == nil || len(facts) == 0 {
		return
	}

	// Fire-and-forget goroutine
	go func() {
		bgCtx := context.WithoutCancel(ctx)
		bgCtx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
		defer cancel()

		if err := w.writeFactsSync(bgCtx, sessionID, agentID, userID, sourceMessageID, facts); err != nil {
			w.lg.Warn("即时事实写入失败",
				loggateway.StepID("memory.immediate_fact"),
				loggateway.Err(err),
				loggateway.SessionID(sessionID),
				loggateway.Int("fact_count", len(facts)))
			return
		}

		w.lg.Info("即时事实写入成功",
			loggateway.StepID("memory.immediate_fact"),
			loggateway.SessionID(sessionID),
			loggateway.Int("fact_count", len(facts)))
	}()
}

// writeFactsSync performs the actual fact persistence.
func (w *ImmediateFactWriter) writeFactsSync(ctx context.Context, sessionID, agentID, userID, sourceMessageID string, facts []FactMark) error {
	if w.factWriter == nil {
		return nil
	}

	// Convert FactMark to MemoryFactWrite
	factWrites := make([]MemoryFactWrite, 0, len(facts))
	for _, f := range facts {
		// Map fact type to fact_kind
		factKind := mapFactTypeToKind(f.Type)

		// Map confidence string to float
		confidence := mapConfidenceToFloat(f.Confidence)

		factWrites = append(factWrites, MemoryFactWrite{
			ScopeType:       "session",
			ScopeID:         sessionID,
			UserID:          userID,
			AgentID:         agentID,
			Statement:       f.Content,
			FactKind:        factKind,
			Confidence:      confidence,
			Importance:      0.8, // High importance for user-explicit facts
			SourceKind:      "immediate_extraction",
			SourceSessionID: sessionID,
			SourceMessageID: sourceMessageID,
			Status:          "active",
		})
	}

	// Use existing consolidation writer with nil episode (facts only, no episode)
	_, err := w.factWriter.UpsertFactsAndEpisodeBatch(ctx, factWrites, nil)
	return err
}

// mapFactTypeToKind maps XML fact type to memory_fact fact_kind.
func mapFactTypeToKind(factType string) string {
	switch strings.ToLower(strings.TrimSpace(factType)) {
	case "identity":
		return "user_identity"
	case "preference":
		return "user_preference"
	case "instruction":
		return "agent_instruction"
	case "domain_knowledge":
		return "domain_knowledge"
	default:
		return "general"
	}
}

// mapConfidenceToFloat maps confidence string to float value.
func mapConfidenceToFloat(confidence string) float64 {
	switch strings.ToLower(strings.TrimSpace(confidence)) {
	case "high":
		return 0.95
	case "medium":
		return 0.7
	case "low":
		return 0.4
	default:
		return 0.6
	}
}

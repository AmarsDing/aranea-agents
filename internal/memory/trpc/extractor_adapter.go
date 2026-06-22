package trpcmem

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcextractor "trpc.group/trpc-go/trpc-agent-go/memory/extractor"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ConsolidatorExtractorAdapter adapts the project's biz.MemoryConsolidator
// to the framework's extractor.MemoryExtractor interface.
//
// This enables the framework's auto-memory worker to use the project's
// consolidation logic (HeuristicConsolidator, ChainConsolidator, etc.)
// instead of the framework's built-in LLM extractor.
//
// Usage: inject via memory backend's WithExtractor() option when switching
// from the project's AutoMemoryWorker to the framework's auto-memory pipeline.
//
// TECH-DEBT(P2-7/8): 适配器已实现但未接入生产路径。项目自建 AutoMemoryWorker
// （三优先级队列 + 死信持久化）比框架 auto-memory 更完善，当前保持自建方案。
// 适配器保留作为未来切换到框架 auto-memory pipeline 的桥接点
// （alignment-plan.md §三 P2-7/8）。
type ConsolidatorExtractorAdapter struct {
	consolidator biz.MemoryConsolidator
	lg           loggateway.Logger
}

var _ trpcextractor.MemoryExtractor = (*ConsolidatorExtractorAdapter)(nil)

// NewConsolidatorExtractorAdapter creates a new adapter.
func NewConsolidatorExtractorAdapter(consolidator biz.MemoryConsolidator, lg loggateway.Logger) *ConsolidatorExtractorAdapter {
	return &ConsolidatorExtractorAdapter{consolidator: consolidator, lg: lg}
}

// Extract adapts the framework's Extract call to the project's consolidator.
// It converts framework messages to ConsolidateMessage format, calls the
// consolidator, and maps proposals back to extractor.Operation values.
func (a *ConsolidatorExtractorAdapter) Extract(ctx context.Context, messages []trpcmodel.Message, existing []*trpcmemory.Entry) ([]*trpcextractor.Operation, error) {
	if a == nil || a.consolidator == nil {
		return nil, nil
	}

	// Convert framework messages to project ConsolidateMessage format.
	consolidateMsgs := make([]biz.ConsolidateMessage, 0, len(messages))
	for _, m := range messages {
		role := string(m.Role)
		content := m.Content
		msgID := ""
		if content == "" && len(m.ToolCalls) > 0 && m.ToolCalls[0].Function.Name != "" {
			content = m.ToolCalls[0].Function.Name
			msgID = m.ToolCalls[0].ID
		}
		if role == string(trpcmodel.RoleTool) && m.ToolID != "" {
			msgID = m.ToolID
		}
		consolidateMsgs = append(consolidateMsgs, biz.ConsolidateMessage{
			Role:      role,
			Content:   content,
			MessageID: msgID,
		})
	}

	input := biz.ConsolidateInput{
		Messages: consolidateMsgs,
	}

	proposals, err := a.consolidator.Extract(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(proposals) == 0 {
		return nil, nil
	}

	// Map proposals to extractor operations.
	ops := make([]*trpcextractor.Operation, 0, len(proposals))
	for _, p := range proposals {
		kind := trpcmemory.KindFact
		if p.Layer == "L2" {
			kind = trpcmemory.KindEpisode
		}
		opType := trpcextractor.OperationAdd
		if p.Statement == "" {
			continue // skip empty proposals
		}
		ops = append(ops, &trpcextractor.Operation{
			Type:       opType,
			Memory:     p.Statement,
			Topics:     p.Topics,
			MemoryKind: kind,
		})
	}
	return ops, nil
}

// ShouldExtract delegates to the framework's default checker logic.
// The project's AutoMemoryWorker has its own trigger logic via AutoMemoryQueue,
// so this always returns true when called by the framework's auto-memory worker.
func (a *ConsolidatorExtractorAdapter) ShouldExtract(_ *trpcextractor.ExtractionContext) bool {
	return true
}

// SetPrompt is a no-op: the project's consolidators don't support dynamic prompts.
func (a *ConsolidatorExtractorAdapter) SetPrompt(string) {}

// SetModel is a no-op: the project's consolidators don't use model.Model.
func (a *ConsolidatorExtractorAdapter) SetModel(trpcmodel.Model) {}

// Metadata returns adapter metadata for telemetry.
func (a *ConsolidatorExtractorAdapter) Metadata() map[string]any {
	return map[string]any{
		"type":   "consolidator_adapter",
		"source": "aranea",
	}
}

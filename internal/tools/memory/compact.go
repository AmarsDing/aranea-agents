package memory

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// --- Context keys for dependency injection ---

type manualCompressorKey struct{}
type compactSessionIDKey struct{}

// WithManualCompressor injects ManualCompressor into context for tool execution.
func WithManualCompressor(ctx context.Context, mc biz.ManualCompressor) context.Context {
	return context.WithValue(ctx, manualCompressorKey{}, mc)
}

// WithCompactSessionID injects session_id into context for the compact tool.
func WithCompactSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, compactSessionIDKey{}, sid)
}

// ManualCompressorFromCtx extracts ManualCompressor from context.
func ManualCompressorFromCtx(ctx context.Context) biz.ManualCompressor {
	v, _ := ctx.Value(manualCompressorKey{}).(biz.ManualCompressor)
	return v
}

// CompactSessionIDFromCtx extracts session_id from context.
func CompactSessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(compactSessionIDKey{}).(string)
	return v
}

// --- compact tool ---

// CompactInput is the input for the compact tool.
type CompactInput struct {
	PreserveInstruction string `json:"preserve_instruction,omitempty" jsonschema:"description=Optional instruction describing what information to preserve during compression (e.g. 'keep all code snippets and user preferences')"`
}

// CompactOutput is the output for the compact tool.
type CompactOutput struct {
	Success          bool   `json:"success"`
	Compacted        bool   `json:"compacted"`
	BeforeTokens     int    `json:"before_tokens"`
	AfterTokens      int    `json:"after_tokens"`
	CompressionLevel string `json:"compression_level,omitempty"`
	FromTurn         int    `json:"from_turn,omitempty"`
	ToTurn           int    `json:"to_turn,omitempty"`
	Message          string `json:"message"`
}

func compactExecute(ctx context.Context, input CompactInput) (CompactOutput, error) {
	mc := ManualCompressorFromCtx(ctx)
	if mc == nil {
		return CompactOutput{}, apierror.Internal(apierror.DomainSession, "ManualCompressor not available in context")
	}
	sessionID := strings.TrimSpace(CompactSessionIDFromCtx(ctx))
	if sessionID == "" {
		return CompactOutput{}, apierror.BadRequest(apierror.DomainSession, "session_id is required")
	}
	result, err := mc.CompactSession(ctx, sessionID, strings.TrimSpace(input.PreserveInstruction))
	if err != nil {
		return CompactOutput{}, err
	}
	if result == nil || !result.Compacted {
		return CompactOutput{
			Success: true,
			Message: "Compression not triggered (conditions not met or already compressed)",
		}, nil
	}
	return CompactOutput{
		Success:          true,
		Compacted:        true,
		BeforeTokens:     result.EstimatedTokensBefore,
		AfterTokens:      result.EstimatedTokensAfter,
		CompressionLevel: result.CompressionLevel,
		FromTurn:         result.FromTurn,
		ToTurn:           result.ToTurn,
		Message:          "Context compressed successfully",
	}, nil
}

// NewCompactTool creates the compact tool that allows agents to actively
// trigger context compression on the current session. The tool compresses
// older conversation history into a summary to free up context window space.
// The compression is idempotent — calling it when already compressed is a no-op.
func NewCompactTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(compactExecute,
		trpcfunction.WithName("compact"),
		trpcfunction.WithDescription("Actively trigger context compression on the current session. Compresses older conversation history into a summary to free up context window space. Use this when you notice the conversation is getting long. The compression is idempotent — calling it when already compressed is a no-op."),
	)
}

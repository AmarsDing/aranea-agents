package custom

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type readFlowLogsInput struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID to query flow logs for,required"`
	TraceID   string `json:"trace_id" jsonschema:"description=Optional trace ID to filter logs"`
	RunID     string `json:"run_id" jsonschema:"description=Optional run ID to filter logs"`
	Severity  string `json:"severity" jsonschema:"description=Optional severity filter (debug/info/warn/error/critical)"`
	Limit     int    `json:"limit" jsonschema:"description=Maximum number of log entries to return (default 50, max 200),default=50"`
	Offset    int    `json:"offset" jsonschema:"description=Number of entries to skip for pagination,default=0"`
}

type readFlowLogsOutput struct {
	Items []flowLogEntry `json:"items"`
	Total int            `json:"total"`
}

type flowLogEntry struct {
	ID            string `json:"id"`
	StepID        string `json:"step_id"`
	Phase         string `json:"phase"`
	Severity      string `json:"severity"`
	Title         string `json:"title"`
	Message       string `json:"message"`
	AutoHealed    bool   `json:"auto_healed"`
	HealStrategy  string `json:"heal_strategy,omitempty"`
	HealAttempts  int    `json:"heal_attempts,omitempty"`
	HealSuccess   bool   `json:"heal_success,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// NewReadFlowLogsTool creates a tool that allows an Agent to read its own flow logs.
// This enables the Agent to inspect runtime errors and diagnose issues autonomously.
func NewReadFlowLogsTool(uc *biz.FlowLogUsecase) *trpcfunction.FunctionTool[readFlowLogsInput, readFlowLogsOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input readFlowLogsInput) (readFlowLogsOutput, error) {
			if input.SessionID == "" && input.TraceID == "" && input.RunID == "" {
				return readFlowLogsOutput{}, kerrors.BadRequest(
					"FLOW_LOG", "at least one of session_id, trace_id, or run_id is required")
			}

			limit := input.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 200 {
				limit = 200
			}

			result, err := uc.List(ctx, biz.FlowLogQuery{
				SessionID: input.SessionID,
				TraceID:   input.TraceID,
				RunID:     input.RunID,
				Severity:  input.Severity,
				Limit:     limit,
				Offset:    input.Offset,
			})
			if err != nil {
				return readFlowLogsOutput{}, kerrors.InternalServer("FLOW_LOG", err.Error())
			}

			items := make([]flowLogEntry, 0, len(result.Items))
			for _, r := range result.Items {
				entry := flowLogEntry{
					ID:        r.ID,
					StepID:    r.StepID,
					Phase:     r.FlowPhase,
					Severity:  r.Severity,
					Title:     r.Title,
					Message:   r.Message,
					CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				}
				// Parse auto-heal metadata from PayloadJSON
				if r.PayloadJSON != "" {
					var payload map[string]any
					if json.Unmarshal([]byte(r.PayloadJSON), &payload) == nil {
						if v, ok := payload["auto_healed"].(bool); ok {
							entry.AutoHealed = v
						}
						if v, ok := payload["heal_strategy"].(string); ok {
							entry.HealStrategy = v
						}
						if v, ok := payload["heal_attempts"].(float64); ok {
							entry.HealAttempts = int(v)
						}
						if v, ok := payload["heal_success"].(bool); ok {
							entry.HealSuccess = v
						}
					}
				}
				items = append(items, entry)
			}

			return readFlowLogsOutput{
				Items: items,
				Total: result.Total,
			}, nil
		},
		trpcfunction.WithName("read_flow_logs"),
		trpcfunction.WithDescription(
			"Read flow logs for a session, trace, or run. Use this tool to inspect runtime errors, "+
				"diagnose issues, and review execution history. Requires at least one of: session_id, trace_id, or run_id."),
	)
}

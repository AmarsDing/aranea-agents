package team

import (
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"context"
	"fmt"
	"time"
)

// validateAssistantOutput checks that the stream produced usable output,
// returning a descriptive error if not.
func validateAssistantOutput(result agent.EventStreamResult) error {
	displayMarkdown, _ := agent.DisplayMarkdownFromStream(result)
	if displayMarkdown != "" {
		return nil
	}
	fallback := "The team workflow produced no usable assistant reply. This may indicate a configuration issue."
	if result.HasError {
		fallback = fmt.Sprintf("Team AI service error: %s. Please check your configuration or try again later.", result.LastError)
	} else if result.HasContent {
		fallback = "The team workflow completed but produced no text output. This may indicate a configuration issue with the model."
	}
	return apierror.Internal("CHAT_TEAM_NATIVE", fallback)
}

// finishTeamRunWithError is a helper that sets turnStatus to error, calls finishRunErr,
// and optionally rolls back the runner session. Returns the error for convenient use in error paths.
func (r *Runner) finishTeamRunWithError(ctx context.Context, run *biz.TeamRunRecord, t0 time.Time, errMsg string, turnStatus *string, rollbackFn func()) {
	*turnStatus = biz.TeamMemberStepStatusError
	r.finishRunErr(ctx, run, t0, errMsg)
	if rollbackFn != nil {
		rollbackFn()
	}
}

// logTeamRunError logs an error to the team emitter if present.
func logTeamRunError(emitter *event.TraceEmitter, step, msg, mode string) {
	if emitter == nil {
		return
	}
	emitter.LogError(step, msg, event.P("mode", mode))
}

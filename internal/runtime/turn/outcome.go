package turn

import "aranea-agents/internal/biz"

// ClassifyNativeOutcome converts a NativeTurnResult (+ optional error) into the
// unified TurnResult used by TurnExecutor / TurnGateway entry points.
func ClassifyNativeOutcome(native biz.NativeTurnResult, err error) (biz.TurnResult, error) {
	if err != nil {
		if isQueuedErr(err) {
			return biz.TurnResult{
				Outcome:   biz.TurnOutcomeQueued,
				PendingID: native.PendingID,
			}, err
		}
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, err
	}
	switch native.Outcome {
	case biz.NativeTurnOutcomeCompleted:
		return biz.TurnResult{
			Outcome:      biz.TurnOutcomeCompleted,
			UserMsg:      native.UserMsg,
			AssistantMsg: native.AssistantMsg,
			Reply:        native.AssistantMsg.ContentMarkdown,
		}, nil
	case biz.NativeTurnOutcomeQueued:
		return biz.TurnResult{
			Outcome:   biz.TurnOutcomeQueued,
			PendingID: native.PendingID,
		}, queuedSentinel()
	case biz.NativeTurnOutcomeFailed:
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, nil
	default:
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, nil
	}
}

// QueuedSentinel is the error returned when a turn was accepted into the pending queue.
var QueuedSentinel = errTurnQueued{}

type errTurnQueued struct{}

func (errTurnQueued) Error() string { return "turn message queued" }

func queuedSentinel() error { return QueuedSentinel }

func isQueuedErr(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(errTurnQueued); ok {
		return true
	}
	return err == QueuedSentinel
}

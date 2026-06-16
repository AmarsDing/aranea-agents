package session

import (
	"strconv"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewAppendEventAuditHook returns an AppendEventHook that logs event metadata
// when events are appended to a session. This provides a unified audit trail
// for session event writes, complementing the EventBus publish path.
//
// The hook is read-only: it does not modify the event or abort the chain.
func NewAppendEventAuditHook(lg loggateway.Logger) trpcsession.AppendEventHook {
	return func(ctx *trpcsession.AppendEventContext, next func() error) error {
		if ctx.Event != nil {
			lg.Debug("session event appended",
				loggateway.StepID("session.append_event"),
				loggateway.SessionID(ctx.Key.SessionID),
				loggateway.Str("event_type", eventTypeLabel(ctx.Event)),
				loggateway.Str("event_id", ctx.Event.ID),
			)
		}
		return next()
	}
}

// NewGetSessionAuditHook returns a GetSessionHook that logs session reads.
// The hook is read-only: it observes the session after retrieval but does not
// modify it. This provides visibility into session access patterns.
func NewGetSessionAuditHook(lg loggateway.Logger) trpcsession.GetSessionHook {
	return func(ctx *trpcsession.GetSessionContext, next func() (*trpcsession.Session, error)) (*trpcsession.Session, error) {
		sess, err := next()
		if err != nil {
			return sess, err
		}
		if sess != nil {
			sess.EventMu.RLock()
			n := len(sess.Events)
			sess.EventMu.RUnlock()
			lg.Debug("session read",
				loggateway.StepID("session.get"),
				loggateway.SessionID(ctx.Key.SessionID),
				loggateway.Str("event_count", strconv.Itoa(n)),
			)
		}
		return sess, nil
	}
}

// eventTypeLabel maps a framework event to a short label for logging.
// This mirrors the classification in plugin/trpc/hook_events.go but is
// scoped to session-level audit logging only.
func eventTypeLabel(ev *trpcevent.Event) string {
	if ev == nil {
		return "unknown"
	}
	if ev.IsRunnerCompletion() {
		return "runner_completion"
	}
	if ev.Response == nil {
		return "event"
	}
	switch ev.Response.Object {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		return "chat.completion.chunk"
	case trpcmodel.ObjectTypeChatCompletion:
		return "chat.completion"
	case trpcmodel.ObjectTypeToolResponse:
		return "tool.response"
	case trpcmodel.ObjectTypeError:
		return "error"
	case trpcmodel.ObjectTypeTransfer:
		return "agent.transfer"
	case trpcmodel.ObjectTypeStateUpdate:
		return "state.update"
	default:
		return "model_response"
	}
}

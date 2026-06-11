package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func enqueueRejectMessage(reason string) string {
	switch reason {
	case biz.ChatEnqueueRejectQueueFull:
		return "pending queue is full for this session"
	case biz.ChatEnqueueRejectNoActiveRun:
		return "agent run has ended; send your message again to start a new turn"
	default:
		return "message could not be queued"
	}
}

func enqueueRejectError(reason string) error {
	switch reason {
	case biz.ChatEnqueueRejectQueueFull:
		return apierror.BadRequest("CHAT_QUEUE_FULL", enqueueRejectMessage(reason))
	case biz.ChatEnqueueRejectNoActiveRun:
		return apierror.Conflict("CHAT_RUN_ENDED", enqueueRejectMessage(reason))
	default:
		return apierror.BadRequest("CHAT_ENQUEUE_REJECTED", enqueueRejectMessage(reason))
	}
}

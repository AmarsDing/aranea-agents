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
		return apierror.BadRequest(apierror.DomainChatQueueFull, enqueueRejectMessage(reason))
	case biz.ChatEnqueueRejectNoActiveRun:
		return apierror.Conflict(apierror.DomainChatRunEnded, enqueueRejectMessage(reason))
	default:
		return apierror.BadRequest(apierror.DomainChatEnqueueRejected, enqueueRejectMessage(reason))
	}
}

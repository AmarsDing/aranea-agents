package service

import (
	"context"
	"errors"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
)

func turnCodeFromFailure(fail provider.Failure) TurnErrorCode {
	switch fail.Kind {
	case provider.FailureBilling:
		return TurnErrProviderBilling
	case provider.FailureAuth:
		return TurnErrProviderAuth
	case provider.FailureStall:
		return TurnErrFirstByteTimeout
	default:
		return TurnErrLLMCallFailed
	}
}

func wrapLLMFailure(err error, firstByteTimeout time.Duration) error {
	if err == nil {
		return nil
	}
	if TurnErrorCodeFromErr(err) != "" {
		return err
	}
	if errors.Is(err, chatagent.ErrFirstByteTimeout) {
		detail := ""
		if firstByteTimeout > 0 {
			detail = firstByteTimeout.String()
		}
		return TurnError(TurnErrFirstByteTimeout, detail)
	}
	fail := provider.ClassifyFailure("", err)
	code := turnCodeFromFailure(fail)
	if code == TurnErrLLMCallFailed {
		return TurnError(TurnErrLLMCallFailed, err.Error())
	}
	return TurnError(code, err.Error())
}

func (o *ChatOrchestrator) publishLLMFailureNotice(ctx context.Context, sessionID string, fail provider.Failure, detail string) {
	if o == nil {
		return
	}
	rt.PublishLLMFailure(o.td().Pipeline.EventBus, o.lg(), ctx, strings.TrimSpace(sessionID), fail, detail)
}

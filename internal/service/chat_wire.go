package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
)

// ProvideChatService constructs ChatService with a noop AfterTurn hook (real hook attached by ProvideEvaluationRunner).
func ProvideChatService(deps ChatServiceDeps) *ChatService {
	deps.AfterTurn = biz.NoopNativeTurnAfter{}
	return NewChatService(deps)
}

// ProvideEvaluationRunner builds the evaluation runner and attaches the AfterTurn hook to chat.
func ProvideEvaluationRunner(
	chat *ChatService,
	turns EvalTurnGateway,
	evalUC *biz.EvalUsecase,
	catalog *biz.LlmProviderModelUsecase,
	sys biz.SystemSettingRepo,
) *evaluation.Runner {
	if chat == nil || turns == nil || evalUC == nil || catalog == nil || sys == nil {
		return nil
	}
	runner := NewEvaluationRunner(evalUC, turns, catalog, sys)
	chat.AttachNativeTurnAfterHook(NewEvaluationAfterTurnTrigger(evalUC, runner))
	return runner
}

// AttachNativeTurnAfterHook sets the post-turn hook after evaluation runner is constructed.
func (s *ChatService) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if s == nil || hook == nil {
		return
	}
	s.orch.AttachNativeTurnAfterHook(hook)
}

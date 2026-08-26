package service

import (
	"context"
	"sort"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/serviceawaitreply"
)

// VoiceConfirmResolver 实现 voice.ConfirmResolver（M74 V2-T5 语音确认拦截）：
// 找到会话内最早发起的待决议工具确认 step，复用 ConfirmActivity 的全量校验
// （归属/状态机/授权）与恢复路径（await channel / 重启后语义化 resume）。
type VoiceConfirmResolver struct {
	chat *ChatService
}

func NewVoiceConfirmResolver(chat *ChatService) *VoiceConfirmResolver {
	return &VoiceConfirmResolver{chat: chat}
}

// ResolvePendingConfirm 实现 voice.ConfirmResolver。
// 无待决议确认时返回 (false, nil)，调用方据此将语句按普通 Chat Turn 处理。
func (r *VoiceConfirmResolver) ResolvePendingConfirm(ctx context.Context, sessionID string, approved bool) (bool, error) {
	if r == nil || r.chat == nil || r.chat.orch == nil {
		return false, nil
	}
	stepReader := r.chat.orch.stepReader()
	if stepReader == nil {
		return false, nil
	}
	step, found, err := oldestPendingConfirm(ctx, stepReader, sessionID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	reply := serviceawaitreply.ReplyApprove
	if !approved {
		reply = serviceawaitreply.ReplyDeny
	}
	if _, err := r.chat.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  step.SessionID,
		ActivityId: step.ID,
		Reply:      reply,
		Approved:   &approved,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// oldestPendingConfirm 在 spirit 树与精确 session 两路收集中取最早 StartedAt 的
// kind=confirm + tool_blocked step。两路口径与前端 useCompanionConfirms 一致：
// confirm step 依挂载点可能只出现在其中一路。
func oldestPendingConfirm(ctx context.Context, reader biz.StepV2Reader, sessionID string) (biz.Step, bool, error) {
	seen := map[string]struct{}{}
	var pending []biz.Step
	collect := func(steps []biz.Step) {
		for _, st := range steps {
			if st.Kind != biz.StepKindConfirm || st.Status != biz.StepStatusToolBlocked {
				continue
			}
			if _, dup := seen[st.ID]; dup {
				continue
			}
			seen[st.ID] = struct{}{}
			pending = append(pending, st)
		}
	}
	var firstErr error
	if steps, err := reader.ListStepsBySpiritSession(ctx, sessionID); err != nil {
		firstErr = err
	} else {
		collect(steps)
	}
	if steps, err := reader.ListStepsBySession(ctx, sessionID); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	} else {
		collect(steps)
	}
	if len(pending) == 0 {
		if firstErr != nil {
			return biz.Step{}, false, firstErr
		}
		return biz.Step{}, false, nil
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].StartedAt.Before(pending[j].StartedAt) })
	return pending[0], true, nil
}

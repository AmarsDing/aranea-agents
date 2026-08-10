package service

import (
	"context"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// VoiceTurnPrewarmer 实现 voice.TurnPrewarmer（C1，2026-08-10）。
//
// voice.start 成功后后台预热 Agent 构建缓存：首个语音 Turn 的
// BuildTRPCAgentCached 实测 cache miss 2.6-2.7s，预热把这段从用户感知
// 关键路径移到「开麦→开口说话」的空窗期。只读且幂等：命中即返回，
// 未命中经 singleflight 与真实 Turn 合并构建；失败仅 Warn（K3），
// 不影响语音链路。
//
// 缓存 key 一致性：dialogMode/provider/model 的解析与
// runNativeAgentTurnBody 完全同源（语音 Turn 的 input.Options 为空），
// 保证预热产物被真实 Turn 命中。
type VoiceTurnPrewarmer struct {
	orch *ChatOrchestrator
	lg   loggateway.Logger
}

// NewVoiceTurnPrewarmer 构造预热器；chatService/orchestrator 为 nil 时返回
// nil（接线方视为关闭预热）。
func NewVoiceTurnPrewarmer(chatService *ChatService) *VoiceTurnPrewarmer {
	if chatService == nil || chatService.orch == nil {
		return nil
	}
	lg := chatService.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VoiceTurnPrewarmer{orch: chatService.orch, lg: lg.With(loggateway.Domain("voice"))}
}

// PrewarmTurn 预热指定会话的 Agent 构建缓存。非阻断容错：任何失败仅记日志。
func (p *VoiceTurnPrewarmer) PrewarmTurn(ctx context.Context, sessionID string) {
	start := time.Now()
	lg := p.lg.With(loggateway.SessionID(sessionID))

	sess, err := p.orch.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		lg.Warn("voice prewarm: session fetch failed",
			loggateway.StepID("chat.voice_prewarm"), loggateway.Err(err))
		return
	}
	// 团队会话走 executeTeamTurnViaHooks 独立构建路径，不在本预热覆盖范围。
	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return
	}
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return
	}
	ag, err := p.orch.hydratedAgent(ctx, agentID)
	if err != nil {
		lg.Warn("voice prewarm: agent hydrate failed",
			loggateway.StepID("chat.voice_prewarm"), loggateway.Err(err))
		return
	}

	// 与 runNativeAgentTurnBody 同源解析（语音 Turn 的 input.Options 为空）。
	dialogMode := strutil.FirstNonEmpty("", sess.DialogMode, "default")
	prov := strutil.FirstNonEmpty("", sess.DefaultProvider, ag.Provider)
	mod := strutil.FirstNonEmpty("", sess.DefaultModel, ag.Model)
	prov, mod = p.orch.resolveProviderModelFallback(ctx, prov, mod)

	deps, err := p.orch.agentBuild.BuildTRPCDeps(ctx, AgentBuildParams{
		Session: sess, Agent: ag, RunID: uuid.NewString(),
		DialogMode: dialogMode, Provider: prov, Model: mod,
	})
	if err != nil {
		lg.Warn("voice prewarm: build deps failed",
			loggateway.StepID("chat.voice_prewarm"), loggateway.Err(err))
		return
	}
	if _, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, lg); err != nil {
		lg.Warn("voice prewarm: agent build failed",
			loggateway.StepID("chat.voice_prewarm"), loggateway.Err(err))
		return
	}
	lg.Info("voice prewarm done",
		loggateway.StepID("chat.voice_prewarm"),
		loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
		loggateway.Any("agent_key", ag.AgentKey))
}

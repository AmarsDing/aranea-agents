package service

import (
	"context"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// embedPrewarmer 是 embedding 冷启动预热的窄接口（生产绑定
// *knowledge.MultiProviderEmbedder；测试注入计数桩）。Prewarm 自身已做
// 60s 成功去重 / 失败重试 / 未配置跳过。
type embedPrewarmer interface {
	Prewarm(ctx context.Context) error
}

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
	orch     *ChatOrchestrator
	lg       loggateway.Logger
	embedder embedPrewarmer
}

// NewVoiceTurnPrewarmer 构造预热器；chatService/orchestrator 为 nil 时返回
// nil（接线方视为关闭预热）。embedder 为 nil 时跳过 embedding 预热。
func NewVoiceTurnPrewarmer(chatService *ChatService, embedder embedPrewarmer) *VoiceTurnPrewarmer {
	if chatService == nil || chatService.orch == nil {
		return nil
	}
	lg := chatService.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VoiceTurnPrewarmer{orch: chatService.orch, lg: lg.With(loggateway.Domain("voice")), embedder: embedder}
}

// PrewarmTurn 预热指定会话的 Agent 构建缓存。非阻断容错：任何失败仅记日志。
func (p *VoiceTurnPrewarmer) PrewarmTurn(ctx context.Context, sessionID string) {
	start := time.Now()
	lg := p.lg.With(loggateway.SessionID(sessionID))

	// C3：embedding 冷启动预热（最小 ping）与 agent 构建预热并列——不依赖
	// 会话/agent 解析结果，voice.start 即触发；内部 60s 去重、失败仅 Warn。
	if p.embedder != nil {
		_ = p.embedder.Prewarm(ctx)
	}

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

// PrewarmSpiritAgent 启动期预构建 __spirit__ agent 构建缓存（修复 C，2026-08-11）。
//
// 进程首个语音/聊天 Turn 的 BuildTRPCAgentCached cache miss 实测 2.6-8s
// （模型解析 + 提示文件 + skill 依赖 + MCP 工具装配）。voice.start 的
// VoiceTurnPrewarmer 只能在「开麦→开口」空窗期预热，进程首轮仍付冷构建；
// 启动预构建在 readiness 门控后后台执行，把冷构建移出全部用户路径。
//
// 缓存 key 一致性（与 runNativeAgentTurnBody 同源）：
//   - dialogMode 固定 "default" → cacheKeyDialogMode 归一化为 ""（spirit 无显式
//     PlannerKind 时仅 "plan" 影响 key；sess.DialogMode="plan" 的会话不覆盖，
//     首个 plan 模式 Turn 仍付一次冷构建，属可接受少数路径）。
//   - provider/model 不在 cache key（per-request 经 RunOption 覆盖）。
//   - 合成 session 仅提供 ID（RoundTripForSession 标签 / AwaitHook 绑定）；
//     AwaitHook 运行时才从 ctx 解析 reply func（ReplyFuncFromContext），
//     构建期合成 session 安全。
//
// 非阻断容错：任何失败仅 Warn（K3），不阻塞启动（Always-Ready）。
func (s *ChatService) PrewarmSpiritAgent(ctx context.Context) {
	if s == nil || s.orch == nil {
		return
	}
	agentsUC := s.orch.td().ReadDeps.AgentsUC
	if agentsUC == nil {
		return
	}
	// ReadDeps.AgentsUC 是窄接口 TeamAgentLookup（无 GetByAgentKey）；生产绑定
	// 的 *biz.AgentUsecase 满足带 GetByAgentKey 的完整接口，断言失败时跳过预热
	// （测试桩场景），不影响启动。
	resolver, ok := agentsUC.(interface {
		GetByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
	})
	if !ok {
		return
	}
	start := time.Now()
	lg := s.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("startup"))

	ag, err := resolver.GetByAgentKey(ctx, biz.SpiritAgentKey)
	if err != nil {
		lg.Warn("startup prewarm: spirit agent fetch failed",
			loggateway.StepID("chat.startup_prewarm"), loggateway.Err(err))
		return
	}
	prov, mod := s.orch.resolveProviderModelFallback(ctx, ag.Provider, ag.Model)
	deps, err := s.orch.agentBuild.BuildTRPCDeps(ctx, AgentBuildParams{
		Session: biz.Session{ID: "startup-prewarm"}, Agent: ag, RunID: uuid.NewString(),
		DialogMode: "default", Provider: prov, Model: mod,
	})
	if err != nil {
		lg.Warn("startup prewarm: build deps failed",
			loggateway.StepID("chat.startup_prewarm"), loggateway.Err(err))
		return
	}
	if _, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, lg); err != nil {
		lg.Warn("startup prewarm: agent build failed",
			loggateway.StepID("chat.startup_prewarm"), loggateway.Err(err))
		return
	}
	lg.Info("startup prewarm: spirit agent built",
		loggateway.StepID("chat.startup_prewarm"),
		loggateway.Int64("elapsed_ms", time.Since(start).Milliseconds()))
}

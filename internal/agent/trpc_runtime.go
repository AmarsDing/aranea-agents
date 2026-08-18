package agent

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/trpcscope"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const TRPCDefaultAppName = trpcscope.DefaultAppName

// midRunMemoryIntervalSteps 是框架 v1.11 mid-run memory 的触发步长：每处理
// N 个 agent 事件增量提取一次记忆（走既有 EnqueueAutoMemoryJob 管线，
// AutoMemoryWorker 幂等性已被 turn-end 路径验证）。面向 24h 长编排场景，
// 避免 turn-end 一次性处理超大 session。
//
// 注意框架按原始 agent 事件计数（含流式 LLM chunk，runner.go
// maybeEnqueueMidRunMemory），流式 turn 每秒约 15~40 事件。实质提取频率由
// MemoryJobQueue 的 30s 会话级防抖兜底；但过小的步长会让防抖拦截（每次
// 拦截写一条 dead-letter upsert）变得频繁。取 100：长 turn 中每 2.5~7s
// 一次入队尝试，防抖后提取频率不变，dead-letter 写放大降一个量级。
const midRunMemoryIntervalSteps = 100

type TRPCRunnerDeps struct {
	AppName               string
	SessionService        trpcsession.Service
	MemoryService         trpcmemory.Service
	EvolutionService      trpcevolution.Service
	ArtifactService       trpcartifact.Service
	Ingestor              trpcsession.Ingestor
	AwaitUserReplyRouting bool
	RalphLoop             *trpcrunner.RalphLoopConfig
	LG                    loggateway.Logger
	Plugins               []trpcplugin.Plugin
}

func NewTRPCRunner(root trpcagent.Agent, deps TRPCRunnerDeps, opts ...trpcrunner.Option) (trpcrunner.ManagedRunner, error) {
	if root == nil {
		return nil, errors.New("trpc runtime: root agent is nil")
	}
	appName := strings.TrimSpace(deps.AppName)
	if appName == "" {
		appName = TRPCDefaultAppName
	}
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg.Info("TRPC Runner 创建", loggateway.StepID("agent.runner_create"), loggateway.Str("app_name", appName))
	if deps.SessionService != nil {
		opts = append([]trpcrunner.Option{trpcrunner.WithSessionService(deps.SessionService)}, opts...)
	}
	if deps.MemoryService != nil {
		opts = append([]trpcrunner.Option{trpcrunner.WithMemoryService(deps.MemoryService)}, opts...)
		opts = append(opts, trpcrunner.WithMidRunMemoryInterval(midRunMemoryIntervalSteps))
	}
	if deps.EvolutionService != nil {
		opts = append(opts, trpcrunner.WithEvolutionService(deps.EvolutionService))
	}
	if len(deps.Plugins) > 0 {
		opts = append(opts, trpcrunner.WithPlugins(deps.Plugins...))
	}
	if deps.ArtifactService != nil {
		opts = append(opts, trpcrunner.WithArtifactService(deps.ArtifactService))
	}
	if deps.Ingestor != nil {
		opts = append(opts, trpcrunner.WithSessionIngestor(deps.Ingestor))
	}
	if deps.AwaitUserReplyRouting {
		opts = append(opts, trpcrunner.WithAwaitUserReplyRouting(true))
	}
	if deps.RalphLoop != nil {
		opts = append(opts, trpcrunner.WithRalphLoop(*deps.RalphLoop))
	}
	r := trpcrunner.NewRunner(appName, root, opts...)
	mr, ok := r.(trpcrunner.ManagedRunner)
	if !ok {
		r.Close()
		lg.Error("TRPC Runner 创建失败：ManagedRunner 接口未实现", loggateway.StepID("agent.runner_create_fail"))
		return nil, errors.New("trpc runtime: runner does not implement ManagedRunner")
	}
	return mr, nil
}

// RunTRPCUserTurnMsg runs a turn with a pre-built user message (multimodal attachments).
func RunTRPCUserTurnMsg(ctx context.Context, r trpcrunner.Runner, userID, sessionID string, msg trpcmodel.Message, opts ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	if r == nil {
		return nil, errors.New("trpc runtime: runner is nil")
	}
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	if userID == "" {
		return nil, errors.New("trpc runtime: user id is required")
	}
	if sessionID == "" {
		return nil, errors.New("trpc runtime: session id is required")
	}
	if msg.Role == "" {
		msg.Role = trpcmodel.RoleUser
	}
	return r.Run(ctx, userID, sessionID, msg, opts...)
}

func TRPCRunStatus(r trpcrunner.Runner, requestID string) (trpcrunner.RunStatus, bool) {
	mr, ok := r.(trpcrunner.ManagedRunner)
	if !ok {
		return trpcrunner.RunStatus{}, false
	}
	return mr.RunStatus(requestID)
}

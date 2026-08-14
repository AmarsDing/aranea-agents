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
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const TRPCDefaultAppName = trpcscope.DefaultAppName

type TRPCRunnerDeps struct {
	AppName               string
	SessionService        trpcsession.Service
	MemoryService         trpcmemory.Service
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


package service

import (
	"context"
	"net/http"
	"strings"
	"sync"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcopenai "trpc.group/trpc-go/trpc-agent-go/server/openai"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

type OpenAICompatService struct {
	chat   *ChatService
	conf   *conf.OpenAI
	mu     sync.RWMutex
	server *trpcopenai.Server
	closer func()
}

func NewOpenAICompatService(chat *ChatService, c *conf.Server) *OpenAICompatService {
	oc := &OpenAICompatService{chat: chat}
	if c != nil && c.Openai != nil && c.Openai.Enable {
		oc.conf = c.Openai
	}
	return oc
}

func (s *OpenAICompatService) Enabled() bool {
	return s.conf != nil && s.conf.Enable
}

func (s *OpenAICompatService) Handler(ctx context.Context) (http.Handler, error) {
	if err := s.ensureServer(ctx); err != nil {
		return nil, err
	}
	return s.server.Handler(), nil
}

func (s *OpenAICompatService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
	return nil
}

func (s *OpenAICompatService) ensureServer(ctx context.Context) error {
	s.mu.RLock()
	if s.server != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}

	agentKey := s.defaultAgentKey()
	runner, closeRunner, err := s.chat.BuildOpenAIRunner(ctx, agentKey)
	if err != nil {
		return err
	}

	basePath := "/v1"
	if s.conf.BasePath != "" {
		basePath = s.conf.BasePath
	}
	modelName := "gpt-3.5-turbo"
	if s.conf.ModelName != "" {
		modelName = s.conf.ModelName
	}

	srv, err := trpcopenai.New(
		trpcopenai.WithRunner(runner),
		trpcopenai.WithBasePath(basePath),
		trpcopenai.WithModelName(modelName),
	)
	if err != nil {
		closeRunner()
		return err
	}

	s.server = srv
	s.closer = closeRunner
	return nil
}

func (s *OpenAICompatService) defaultAgentKey() string {
	if s.conf.DefaultAgentKey != "" {
		return s.conf.DefaultAgentKey
	}
	return "default"
}

func (s *ChatService) BuildOpenAIRunner(ctx context.Context, agentKey string) (trpcrunner.Runner, func(), error) {
	if s == nil || s.orch == nil {
		return nil, nil, biz.ErrNotFound
	}
	ag, err := s.orch.td.Catalog.Agents.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		return nil, nil, err
	}
	biz.HydrateAgentKind(&ag)
	if biz.IsA2AProxyAgent(ag) {
		return nil, nil, biz.ErrNotFound
	}

	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	deps := chatagent.TRPCBuilderDeps{
		Catalog:               s.orch.td.Catalog.LLM,
		AgentUC:               s.orch.td.Catalog.AgentsUC,
		Agents:                s.orch.td.Catalog.Agents,
		RT:                    s.orch.td.RoundTrip(),
		SkillUC:               s.orch.td.Catalog.SkillUC,
		MCPTooling:            s.orch.td.Persist.AgentMCP,
		ToolUC:                s.orch.td.Catalog.ToolUC,
		Sessions:              s.orch.td.Sessions,
		Sys:                   s.orch.td.Catalog.Settings,
		Provider:              prov,
		Model:                 mod,
		SkillDBRepo:           s.orch.rt.SkillDBRepo,
		HasMemory:             s.orch.td.Persist.Memory.Available(),
		PluginManager:         s.orch.rt.PluginManager,
		MemoryAdmin:           s.orch.td.Persist.Memory.Admin,
		MemoryL2Recall:        s.orch.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        s.orch.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: s.orch.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever:    s.orch.rt.KnowledgeRetriever,
		CodeExecFactory:       s.orch.rt.CodeExecFactory,
		KanbanBridge:          s.orch.rt.KanbanBridge,
	}
	var plugins []trpcplugin.Plugin
	if s.orch.rt.PluginManager != nil {
		plugins = s.orch.rt.PluginManager.RunnerPluginsForAgent(ag.ID)
	} else if s.orch.rt.PluginRT != nil {
		plugins = s.orch.rt.PluginRT.PluginsForAgent(ag.ID)
	}
	deps.Plugins = plugins

	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, s.orch.lg)
	if err != nil {
		return nil, nil, err
	}
	lookup := map[string]trpcagent.Agent{}
	if key := strings.TrimSpace(ag.AgentKey); key != "" {
		lookup[key] = root
	}
	rl := chatagent.ResolveRalphLoopTurn(ag.Settings)
	if rl.SkipErr != nil {
		loggateway.Global().Warn("Ralph Loop 配置无效，已跳过",
			loggateway.StepID("openai.runner.ralph_loop"),
			loggateway.Str("agent_id", ag.ID), loggateway.Str("error", rl.SkipErr.Error()))
	}
	runner, err := s.orch.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:          plugins,
		BuilderDeps:      deps,
		AgentFactoryKeys: []string{ag.AgentKey},
		LookupAgents:     lookup,
		RalphLoop:        rl.Config,
	})
	if err != nil {
		return nil, nil, err
	}
	return runner, func() { runner.Close() }, nil
}

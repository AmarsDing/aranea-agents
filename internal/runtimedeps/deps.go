package runtimedeps

import (
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/provider"
)

type Runtime struct {
	SessionMemory *sessionmemory.Store
	AgentMCP      *biz.AgentMCPTooling
}

func NewRuntime(store *sessionmemory.Store, mcp *biz.AgentMCPTooling) *Runtime {
	return &Runtime{SessionMemory: store, AgentMCP: mcp}
}

type TurnDeps struct {
	Agents       biz.AgentRepository
	AgentsUC     *biz.AgentUsecase
	ToolsCatalog biz.ToolRepo
	LLMCatalog   *biz.LlmProviderModelUsecase
	SkillUC      *biz.SkillUsecase
	Sys          biz.SystemSettingRepo
	RT           *Runtime
	LLMHTTP      *http.Client
	Sessions     *biz.SessionUsecase
	Compress     biz.NativeTurnCompressor
	MonitorLogs  *biz.MonitorLogBroker
	TeamSSE      *biz.TeamRunEventBroker
}

func (d TurnDeps) RoundTrip() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: d.LLMHTTP}
}

func (d TurnDeps) SQLiteSessionMemory() bool {
	return d.RT != nil && d.RT.SessionMemory != nil
}

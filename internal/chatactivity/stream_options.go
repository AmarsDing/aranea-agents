package chatactivity

import (
	"context"
	"strings"
	"sync"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type catalogActivityMetaResolver struct {
	tools  biz.TeamToolLookup
	agents biz.AgentRepository
	cache  sync.Map
}

func newCatalogActivityMetaResolver(tools biz.TeamToolLookup, agents biz.AgentRepository) chatagent.ActivityMetaResolver {
	return &catalogActivityMetaResolver{tools: tools, agents: agents}
}

func (r *catalogActivityMetaResolver) ResolveDisplayLabel(ctx context.Context, toolName string) string {
	if r == nil || r.tools == nil {
		return ""
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ""
	}
	if v, ok := r.cache.Load("tool:" + toolName); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	for _, key := range chatagent.CatalogLookupKeysForRuntimeName(toolName) {
		tool, err := r.tools.GetTool(ctx, key)
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(tool.DisplayName); name != "" {
			r.cache.Store("tool:"+toolName, name)
			return name
		}
	}
	return ""
}

func (r *catalogActivityMetaResolver) ResolveAgentDisplayName(ctx context.Context, agentKey string) string {
	if r == nil || r.agents == nil {
		return ""
	}
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return ""
	}
	if v, ok := r.cache.Load("agent-name:" + agentKey); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	ag, err := r.agents.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(ag.DisplayName)
	if name == "" {
		name = strings.TrimSpace(ag.AgentKey)
	}
	if name != "" {
		r.cache.Store("agent-name:"+agentKey, name)
	}
	return name
}

func (r *catalogActivityMetaResolver) ResolveAgentID(ctx context.Context, agentKey string) string {
	if r == nil || r.agents == nil {
		return ""
	}
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return ""
	}
	if v, ok := r.cache.Load("agent-id:" + agentKey); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	ag, err := r.agents.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(ag.ID)
	if id != "" {
		r.cache.Store("agent-id:"+agentKey, id)
	}
	return id
}

// NewStreamConsumeOptions wires catalog lookup and the v2 projector for a chat turn.
//
// v2 phase: the v2 projector is the sole projection path. The v1
// ActivityProjector has been removed. When v2Projector is nil (test
// scenarios), events are not projected. The v2 projector is a singleton
// shared across turns (per-turn state is reset in OnTurnStart).
func NewStreamConsumeOptions(tools biz.TeamToolLookup, toolRegistry biz.ToolRegistryReader, agents biz.AgentRepository, activityWriter biz.ActivityUpserter, activityBus biz.ActivityEventBus, v2Projector *v2.ActivityProjector, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	var resolver chatagent.ActivityMetaResolver
	if tools != nil || agents != nil {
		resolver = newCatalogActivityMetaResolver(tools, agents)
	}
	opts := &chatagent.StreamConsumeOptions{
		MetaResolver: resolver,
	}
	if activityBus != nil {
		opts.ActivityBus = activityBus
	}
	opts.V2Projector = v2Projector
	return opts
}

// StreamOptsFactoryAdapter implements team.StreamOptsFactory by closing over
// the catalog dependencies needed to construct StreamConsumeOptions.
// Inject this into the team Runner via SetStreamOptsFactory to eliminate
// the team→chatactivity direct import.
type StreamOptsFactoryAdapter struct {
	Tools            biz.TeamToolLookup
	ToolRegistry     biz.ToolRegistryReader
	Agents           biz.AgentRepository
	ActivityUpserter biz.ActivityUpserter
	ActivityBus      biz.ActivityEventBus
	// V2Projector is the singleton v2 projector. When non-nil, every chat
	// turn triggers the v2 dual-path (additive to v1). Wired via Wire DI.
	V2Projector *v2.ActivityProjector
	Logger      loggateway.Logger
}

func (a *StreamOptsFactoryAdapter) NewStreamConsumeOptions() *chatagent.StreamConsumeOptions {
	return NewStreamConsumeOptions(a.Tools, a.ToolRegistry, a.Agents, a.ActivityUpserter, a.ActivityBus, a.V2Projector, a.Logger)
}

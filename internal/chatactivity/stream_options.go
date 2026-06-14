package chatactivity

import (
	"context"
	"fmt"
	"strings"
	"sync"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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

type sessionActivityPersister struct {
	sessions biz.SessionTurnExtrasPort
}

func newSessionActivityPersister(sessions biz.SessionTurnExtrasPort) chatagent.ActivityPersister {
	return &sessionActivityPersister{sessions: sessions}
}

func (p *sessionActivityPersister) UpsertActivity(ctx context.Context, meta chatagent.ProjectMeta, tc event.EnvelopeToolCall) error {
	if p == nil || isNilInterface(p.sessions) {
		return nil
	}
	sessionID := strings.TrimSpace(meta.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session id required")
	}
	if strings.TrimSpace(tc.ID) == "" {
		return fmt.Errorf("tool_call id required")
	}
	msg, err := chatagent.ChatMessageFromToolActivity(meta, tc)
	if err != nil {
		return err
	}
	return p.sessions.UpsertChatActivityMessage(ctx, sessionID, msg)
}

// NewStreamConsumeOptions wires catalog lookup and activity persistence for a chat turn.
func NewStreamConsumeOptions(tools biz.TeamToolLookup, agents biz.AgentRepository, sessions biz.SessionTurnExtrasPort, activityWriter biz.ActivityWriter, eventBus event.Bus, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	var resolver chatagent.ActivityMetaResolver
	var persister chatagent.ActivityPersister
	if tools != nil || agents != nil {
		resolver = newCatalogActivityMetaResolver(tools, agents)
	}
	if sessions != nil {
		persister = newSessionActivityPersister(sessions)
	}
	opts := &chatagent.StreamConsumeOptions{
		MetaResolver:      resolver,
		ActivityPersister: persister,
	}
	// AF phase: create ActivityProjector for dual-emission.
	// When ActivityProjector is active, stream_consumer.go uses hasAF
	// (opts.ActivityProjector != nil) to skip WS publishing of
	// EventProjector envelopes. The frontend AF path consumes Activity
	// events exclusively.
	if activityWriter != nil && eventBus != nil {
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		opts.ActivityProjector = chatagent.NewActivityProjector(eventBus, activityWriter, lg)
	}
	return opts
}

// StreamOptsFactoryAdapter implements team.StreamOptsFactory by closing over
// the catalog dependencies needed to construct StreamConsumeOptions.
// Inject this into the team Runner via SetStreamOptsFactory to eliminate
// the team→chatactivity direct import.
type StreamOptsFactoryAdapter struct {
	Tools          biz.TeamToolLookup
	Agents         biz.AgentRepository
	Sessions       biz.SessionTurnExtrasPort
	ActivityWriter biz.ActivityWriter
	EventBus       event.Bus
	Logger         loggateway.Logger
}

func (a *StreamOptsFactoryAdapter) NewStreamConsumeOptions() *chatagent.StreamConsumeOptions {
	return NewStreamConsumeOptions(a.Tools, a.Agents, a.Sessions, a.ActivityWriter, a.EventBus, a.Logger)
}

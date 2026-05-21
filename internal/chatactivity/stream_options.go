package chatactivity

import (
	"context"
	"fmt"
	"strings"
	"sync"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

type catalogActivityMetaResolver struct {
	tools  *biz.ToolUsecase
	agents biz.AgentRepository
	cache  sync.Map
}

func newCatalogActivityMetaResolver(tools *biz.ToolUsecase, agents biz.AgentRepository) chatagent.ActivityMetaResolver {
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
	sessions *biz.SessionUsecase
}

func newSessionActivityPersister(sessions *biz.SessionUsecase) chatagent.ActivityPersister {
	return &sessionActivityPersister{sessions: sessions}
}

func (p *sessionActivityPersister) UpsertActivity(ctx context.Context, meta chatagent.ProjectMeta, tc event.EnvelopeToolCall) error {
	if p == nil || p.sessions == nil {
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
func NewStreamConsumeOptions(tools *biz.ToolUsecase, agents biz.AgentRepository, sessions *biz.SessionUsecase) *chatagent.StreamConsumeOptions {
	var resolver chatagent.ActivityMetaResolver
	var persister chatagent.ActivityPersister
	if tools != nil || agents != nil {
		resolver = newCatalogActivityMetaResolver(tools, agents)
	}
	if sessions != nil {
		persister = newSessionActivityPersister(sessions)
	}
	return &chatagent.StreamConsumeOptions{
		MetaResolver:      resolver,
		ActivityPersister: persister,
	}
}

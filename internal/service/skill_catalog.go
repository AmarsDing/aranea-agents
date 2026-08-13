package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// skillCatalogPushTimeout bounds the best-effort catalog resolution + publish
// triggered from the WS connect path. Derived from the caller's (connection)
// context so a dropped connection aborts the push.
const skillCatalogPushTimeout = 5 * time.Second

// PushSkillCatalog resolves the session's agent-visible skills and publishes a
// skill.catalog v2 event (design 69 Phase 3). Triggered once per chat WS
// connection setup so the frontend can render the skill entry strip.
//
// Best-effort: every failure is logged and swallowed — a catalog failure must
// never break WS connection setup. When the session has no agent, or the agent
// has no visible skills, no event is published.
//
// Implements server.SkillCatalogPusher.
func (s *ChatService) PushSkillCatalog(ctx context.Context, sessionID string) {
	if s == nil || s.orch == nil {
		return
	}
	lg := s.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || sessionID == "*" {
		return
	}
	td := s.orch.td()
	if td.Sessions == nil || td.ReadDeps.SkillUC == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pushCtx, cancel := context.WithTimeout(ctx, skillCatalogPushTimeout)
	defer cancel()

	sess, err := td.Sessions.Get(pushCtx, sessionID)
	if err != nil {
		lg.Warn("skill catalog: session lookup failed",
			loggateway.StepID("chat.skill_catalog"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return
	}
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return
	}
	ag, err := s.orch.hydratedAgent(pushCtx, agentID)
	if err != nil {
		lg.Warn("skill catalog: agent lookup failed",
			loggateway.StepID("chat.skill_catalog"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return
	}
	candidates, err := td.ReadDeps.SkillUC.ListEnabledPublishedSkillCandidates(pushCtx)
	if err != nil {
		lg.Warn("skill catalog: list candidates failed",
			loggateway.StepID("chat.skill_catalog"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return
	}
	if len(candidates) == 0 {
		return
	}

	// Layer A-only visibility filter (allowed/denied slugs) — same source as
	// the runtime skill overview (agent.buildSkillDeps). Avoid typed-nil
	// interface: nil *AgentRuntimeSettings must stay a nil interface.
	var runtimeIface skillruntime.RuntimeSettings
	if ag.Settings != nil {
		runtimeIface = ag.Settings
	}
	filter := skillruntime.NewAgentVisibilityFilter(runtimeIface)

	entries := make([]biz.SkillCatalogEntry, 0, len(candidates))
	for _, c := range candidates {
		if filter != nil && !filter(pushCtx, trpcskill.Summary{Name: c.Slug, Description: c.Description}) {
			continue
		}
		entries = append(entries, biz.SkillCatalogEntry{
			Slug:        c.Slug,
			Name:        c.Name,
			Description: c.Description,
			Tags:        plainSkillTagNames(c.Tags),
		})
	}
	if len(entries) == 0 {
		return
	}

	ev := biz.NewSkillCatalogEvent(sessionID, entries)
	if seq := s.orch.v2Seq; seq != nil {
		seq.Publish(pushCtx, ev)
	} else if bus := td.Pipeline.EventBus; bus != nil {
		bus.Publish(pushCtx, ev)
	} else {
		return
	}
	lg.Info("skill catalog: pushed",
		loggateway.StepID("chat.skill_catalog"),
		loggateway.SessionID(sessionID),
		loggateway.Str("skill_count", strconv.Itoa(len(entries))))
}

// plainSkillTagNames converts skill tags to plain display names, stripping the
// dimension prefix ("domain:sales" → "sales") for compact catalog cards.
func plainSkillTagNames(tags []biz.SkillTag) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		name := strings.TrimSpace(t.Name)
		if idx := strings.Index(name, ":"); idx >= 0 {
			name = strings.TrimSpace(name[idx+1:])
		}
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubSkillLookup 仅实现 biz.TeamSkillLookup 中 catalog 推送用到的方法，
// 其余方法返回零值（测试不会触达）。
type stubSkillLookup struct {
	biz.TeamSkillLookup // 嵌入接口：未实现的方法 panic（测试中不应被调用）
	candidates          []biz.SkillRuntimeCandidate
	err                 error
}

func (s stubSkillLookup) ListEnabledPublishedSkillCandidates(context.Context) ([]biz.SkillRuntimeCandidate, error) {
	return s.candidates, s.err
}

// stubAgentLookup 返回固定 agent（含 Layer A 可见性策略）。
type stubAgentLookup struct {
	biz.TeamAgentLookup
	agent biz.Agent
	err   error
}

func (s stubAgentLookup) Get(context.Context, string) (biz.Agent, error) {
	return s.agent, s.err
}

// capturePublisher 捕获 rt.EventPublisher 收到的事件。
type capturePublisher struct {
	events []biz.Event
}

func (c *capturePublisher) Publish(_ context.Context, e biz.Event) {
	c.events = append(c.events, e)
}

func newSkillCatalogTestSvc(
	sessions biz.SessionTurnManager,
	agents biz.TeamAgentLookup,
	skills biz.TeamSkillLookup,
	pub *capturePublisher,
) *ChatService {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = sessions
	orch.core.TD.ReadDeps.AgentsUC = agents
	orch.core.TD.ReadDeps.SkillUC = skills
	orch.v2Seq = pub
	return &ChatService{orch: orch, lg: loggateway.NewNoop()}
}

func skillCatalogCandidates() []biz.SkillRuntimeCandidate {
	return []biz.SkillRuntimeCandidate{
		{
			Slug:        "code-review",
			Name:        "Code Review",
			Description: "Review code changes",
			Tags:        []biz.SkillTag{{Name: "domain:engineering", Source: "user"}, {Name: "review", Source: "user"}},
		},
		{
			Slug:        "doc-writer",
			Name:        "Doc Writer",
			Description: "Write docs",
			Tags:        []biz.SkillTag{{Name: "writing", Source: "user"}},
		},
	}
}

// 主路径：会话 → agent → Layer A 过滤 → 发布 skill.catalog 事件。
func TestPushSkillCatalog_PublishesFilteredCatalog(t *testing.T) {
	sessions := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}
	agents := stubAgentLookup{agent: biz.Agent{
		ID:       "agent-1",
		Settings: &biz.AgentRuntimeSettings{SkillRuntimeJSON: `{"denied_slugs":["doc-writer"]}`},
	}}
	pub := &capturePublisher{}
	svc := newSkillCatalogTestSvc(sessions, agents, stubSkillLookup{candidates: skillCatalogCandidates()}, pub)

	svc.PushSkillCatalog(context.Background(), "sess-1")

	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	ev, ok := pub.events[0].(*biz.SkillCatalogEvent)
	if !ok {
		t.Fatalf("expected *biz.SkillCatalogEvent, got %T", pub.events[0])
	}
	if ev.EventKind() != biz.EventKindSkillCatalog {
		t.Fatalf("kind=%s", ev.EventKind())
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("session=%s", ev.SpiritSessionID())
	}
	if len(ev.Skills) != 1 {
		t.Fatalf("expected 1 visible skill, got %+v", ev.Skills)
	}
	entry := ev.Skills[0]
	if entry.Slug != "code-review" || entry.Name != "Code Review" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	// 维度前缀必须剥离（domain:engineering → engineering）。
	if len(entry.Tags) != 2 || entry.Tags[0] != "engineering" || entry.Tags[1] != "review" {
		t.Fatalf("tags not stripped: %+v", entry.Tags)
	}
}

// Layer A allowed_slugs 非空时仅放行白名单。
func TestPushSkillCatalog_AllowedSlugsWhitelist(t *testing.T) {
	sessions := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}
	agents := stubAgentLookup{agent: biz.Agent{
		ID:       "agent-1",
		Settings: &biz.AgentRuntimeSettings{SkillRuntimeJSON: `{"allowed_slugs":["doc-writer"]}`},
	}}
	pub := &capturePublisher{}
	svc := newSkillCatalogTestSvc(sessions, agents, stubSkillLookup{candidates: skillCatalogCandidates()}, pub)

	svc.PushSkillCatalog(context.Background(), "sess-1")

	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	ev := pub.events[0].(*biz.SkillCatalogEvent)
	if len(ev.Skills) != 1 || ev.Skills[0].Slug != "doc-writer" {
		t.Fatalf("expected only doc-writer, got %+v", ev.Skills)
	}
}

// 无 Settings（nil）→ 过滤放行全部，且不出现 typed-nil panic。
func TestPushSkillCatalog_NilSettingsAllowsAll(t *testing.T) {
	sessions := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}
	agents := stubAgentLookup{agent: biz.Agent{ID: "agent-1"}}
	pub := &capturePublisher{}
	svc := newSkillCatalogTestSvc(sessions, agents, stubSkillLookup{candidates: skillCatalogCandidates()}, pub)

	svc.PushSkillCatalog(context.Background(), "sess-1")

	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	ev := pub.events[0].(*biz.SkillCatalogEvent)
	if len(ev.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %+v", ev.Skills)
	}
}

// 全部候选被 Layer A 拒绝 → 不发事件。
func TestPushSkillCatalog_NoVisibleSkillsNoEvent(t *testing.T) {
	sessions := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}
	agents := stubAgentLookup{agent: biz.Agent{
		ID:       "agent-1",
		Settings: &biz.AgentRuntimeSettings{SkillRuntimeJSON: `{"allowed_slugs":["nonexistent"]}`},
	}}
	pub := &capturePublisher{}
	svc := newSkillCatalogTestSvc(sessions, agents, stubSkillLookup{candidates: skillCatalogCandidates()}, pub)

	svc.PushSkillCatalog(context.Background(), "sess-1")

	if len(pub.events) != 0 {
		t.Fatalf("expected no event, got %+v", pub.events)
	}
}

// 容错路径均不发布事件、不 panic、快速返回。
func TestPushSkillCatalog_BestEffortGuards(t *testing.T) {
	notFoundSession := stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found")
	}}
	noAgentSession := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id}, nil
	}}
	agentErr := stubAgentLookup{err: apierror.NotFound(apierror.DomainAgent, "agent not found")}
	skillErr := stubSkillLookup{err: apierror.Internal(apierror.DomainSkill, "db down")}
	okSession := stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}
	okAgent := stubAgentLookup{agent: biz.Agent{ID: "agent-1"}}

	cases := map[string]*ChatService{
		"nil service":        nil,
		"nil orchestrator":   {},
		"session not found":  newSkillCatalogTestSvc(notFoundSession, okAgent, stubSkillLookup{candidates: skillCatalogCandidates()}, &capturePublisher{}),
		"session no agent":   newSkillCatalogTestSvc(noAgentSession, okAgent, stubSkillLookup{candidates: skillCatalogCandidates()}, &capturePublisher{}),
		"agent lookup error": newSkillCatalogTestSvc(okSession, agentErr, stubSkillLookup{candidates: skillCatalogCandidates()}, &capturePublisher{}),
		"skill list error":   newSkillCatalogTestSvc(okSession, okAgent, skillErr, &capturePublisher{}),
		"empty candidates":   newSkillCatalogTestSvc(okSession, okAgent, stubSkillLookup{}, &capturePublisher{}),
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				tc.PushSkillCatalog(context.Background(), "sess-x")
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("PushSkillCatalog must return promptly on guard paths")
			}
		})
	}
}

// global 通配会话（session_id="*"）不推送。
func TestPushSkillCatalog_SkipsGlobalSession(t *testing.T) {
	pub := &capturePublisher{}
	svc := newSkillCatalogTestSvc(stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, AgentID: "agent-1"}, nil
	}}, stubAgentLookup{agent: biz.Agent{ID: "agent-1"}}, stubSkillLookup{candidates: skillCatalogCandidates()}, pub)

	svc.PushSkillCatalog(context.Background(), "*")

	if len(pub.events) != 0 {
		t.Fatalf("expected no event for global session, got %+v", pub.events)
	}
}

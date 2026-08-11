package agent

// Prompt prefix stabilization tests (P1 TTFT optimization).
//
// DeepSeek prompt caching matches tokens from position 0: any per-turn change
// inside the system-message prefix invalidates the whole cached block. All
// BeforeModel inject hooks must therefore append their system message AFTER
// the existing system block (insertAfterLastSystem), never prepend to
// position 0. These tests pin that contract for every injection site:
// the base system prompt must remain at index 0 and the injected cue must
// land immediately after it (before the first user message).

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const prefixTestBaseSystem = "BASE-SYSTEM-PROMPT"

// runBeforeModelHook executes a BeforeModel hook against a request that
// already carries the base system prompt + one user message, returning the
// mutated message list.
func runBeforeModelHook(t *testing.T, cb callbacks.Callback, ctx context.Context) []trpcmodel.Message {
	t.Helper()
	if cb == nil {
		t.Fatal("hook constructor returned nil; test setup must satisfy its guards")
	}
	h, ok := cb.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatalf("hook %T does not implement callbacks.BeforeModelHook", cb)
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage(prefixTestBaseSystem),
		trpcmodel.NewUserMessage("你好"),
	}}}
	if _, err := h.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("HandleBeforeModel: %v", err)
	}
	return args.Request.Messages
}

// assertCueAfterBase pins the prefix-stabilization contract: base system
// prompt stays at index 0, injected cue is a system message at index 1
// containing marker, and the user message follows.
func assertCueAfterBase(t *testing.T, msgs []trpcmodel.Message, marker string) {
	t.Helper()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (base + cue + user), got %d", len(msgs))
	}
	if msgs[0].Role != trpcmodel.RoleSystem || msgs[0].Content != prefixTestBaseSystem {
		t.Fatalf("base system prompt must remain at index 0, got role=%s content=%.40q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != trpcmodel.RoleSystem {
		t.Fatalf("injected cue must be a system message at index 1, got role=%s", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, marker) {
		t.Fatalf("injected cue at index 1 must contain %q, got %.60q", marker, msgs[1].Content)
	}
	if msgs[2].Role != trpcmodel.RoleUser {
		t.Fatalf("user message must follow the system block, got role=%s at index 2", msgs[2].Role)
	}
}

// ── fakes ────────────────────────────────────────────────────────────────

type fakeTeamAgentLookup struct{ eff biz.AgentEffectiveTools }

func (f fakeTeamAgentLookup) Get(context.Context, string) (biz.Agent, error) { return biz.Agent{}, nil }
func (f fakeTeamAgentLookup) GetEffectiveTools(context.Context, string) (biz.AgentEffectiveTools, error) {
	return f.eff, nil
}
func (f fakeTeamAgentLookup) BatchHydrateForBuild(_ context.Context, agents []biz.Agent) ([]biz.Agent, error) {
	return agents, nil
}

type fakeSkillLookup struct {
	candidates []biz.SkillRuntimeCandidate
	guidance   []biz.SkillGuidanceEntry
}

func (f fakeSkillLookup) ListEnabledPublishedSkillKeys(context.Context) ([]string, error) {
	return nil, nil
}
func (f fakeSkillLookup) ListEnabledPublishedSkillRefs(context.Context) ([]biz.SkillEnabledRef, error) {
	return nil, nil
}
func (f fakeSkillLookup) ListEnabledPublishedSkillCandidates(context.Context) ([]biz.SkillRuntimeCandidate, error) {
	return f.candidates, nil
}
func (f fakeSkillLookup) ScoreByEmbedding(context.Context, string, []biz.SkillRuntimeCandidate) (map[string]float64, error) {
	return nil, nil
}
func (f fakeSkillLookup) BatchGetSkillGuidance(_ context.Context, slugs []string) ([]biz.SkillGuidanceEntry, error) {
	return f.guidance, nil
}
func (f fakeSkillLookup) GetBySlug(context.Context, string) (biz.Skill, error) {
	return biz.Skill{}, nil
}
func (f fakeSkillLookup) RecordInvocation(context.Context, biz.SkillInvocationWrite) error {
	return nil
}

// fakeProfileCardReader is reused from composite_prompt_test.go.

// fakeL1AdminStore satisfies biz.SessionAdminStore via a nil embedded
// interface; only the two L1 row readers exercised by L1MemoryCue are
// overridden.
type fakeL1AdminStore struct {
	biz.SessionAdminStore
	taskRows [][]byte
}

func (f fakeL1AdminStore) ListL1TaskRows(context.Context, string, string, string, string) ([][]byte, error) {
	return f.taskRows, nil
}
func (f fakeL1AdminStore) ListL1FieldRows(context.Context, string, bool, ...string) ([][]byte, error) {
	return nil, nil
}

// fakeKnowledgeRepos satisfies the collection/document/chunk repo triple via
// nil embedded interfaces; only ListCollections is exercised by
// buildKnowledgeCue (Usecase.requireRepo demands all three non-nil).
type fakeKnowledgeRepos struct {
	biz.KnowledgeCollectionRepo
	biz.KnowledgeDocumentRepo
	biz.KnowledgeChunkRepo
	collections []biz.KnowledgeCollection
}

func (f fakeKnowledgeRepos) ListCollections(context.Context, string, int, int) ([]biz.KnowledgeCollection, int, error) {
	return f.collections, len(f.collections), nil
}

// ── injection position tests ─────────────────────────────────────────────

func TestStaticRuntimeCueHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	hook := newStaticRuntimeCueBeforeHook(ag, TRPCBuilderDeps{})
	msgs := runBeforeModelHook(t, hook, context.Background())
	assertCueAfterBase(t, msgs, "Runtime capability policy")
}

func TestDynamicRuntimeCueHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	deps := TRPCBuilderDeps{TRPCModelCatalogDeps: TRPCModelCatalogDeps{
		AgentUC: fakeTeamAgentLookup{eff: biz.AgentEffectiveTools{ToolsEnabled: true, Profile: "full"}},
	}}
	hook := newDynamicRuntimeCueBeforeHook(ag, deps)
	msgs := runBeforeModelHook(t, hook, context.Background())
	assertCueAfterBase(t, msgs, "Effective tool keys")
}

func TestSkillGuidanceFullProfileHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{SkillLoadMode: "turn"},
	}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{
		SkillUC: fakeSkillLookup{
			candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
			guidance:   []biz.SkillGuidanceEntry{{Slug: "demo-skill", Guidance: "use the demo skill"}},
		},
	}}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	msgs := runBeforeModelHook(t, hook, context.Background())
	assertCueAfterBase(t, msgs, "Available Skills")
}

func TestProgressiveSkillGuidanceHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{SkillLoadMode: "progressive"},
	}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{
		SkillUC: fakeSkillLookup{
			candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
		},
	}}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	msgs := runBeforeModelHook(t, hook, context.Background())
	assertCueAfterBase(t, msgs, "Routed Skills")
}

func TestMemoryInjectHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID: "ag-1",
		Settings: &biz.AgentRuntimeSettings{
			MemoryEnabled: true,
			L3Enabled:     true,
			L0InjectL3:    true,
		},
	}
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
		MemoryProfileCardReader: fakeProfileCardReader{card: &biz.ProfileCard{Content: "偏好：中餐"}},
	}}
	hook := newMemoryInjectBeforeHook(ag, deps)
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := runBeforeModelHook(t, hook, ctx)
	assertCueAfterBase(t, msgs, "用户档案")
}

func TestRebuildMemoryInjectForCompaction_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID: "ag-1",
		Settings: &biz.AgentRuntimeSettings{
			MemoryEnabled: true,
			L1Enabled:     true,
			L0InjectL1:    true,
		},
	}
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
		MemoryAdmin: fakeL1AdminStore{taskRows: [][]byte{[]byte(`{"id":"t1","task_title":"测试任务"}`)}},
	}}
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	req := &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage(prefixTestBaseSystem),
		trpcmodel.NewUserMessage("你好"),
	}}
	RebuildMemoryInjectForCompaction(ctx, deps, ag, req)
	assertCueAfterBase(t, req.Messages, "L1 working memory")
}

func TestKnowledgeCueHook_InsertsAfterExistingSystem(t *testing.T) {
	ag := biz.Agent{
		ID:       "ag-1",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	repo := fakeKnowledgeRepos{
		collections: []biz.KnowledgeCollection{{ID: "c1", Name: "产品手册"}},
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{KnowledgeUsecase: uc}}
	hook := newKnowledgeCueBeforeHook(ag, deps)
	msgs := runBeforeModelHook(t, hook, context.Background())
	assertCueAfterBase(t, msgs, "Available Knowledge Bases")
}

func TestReplyReminderHook_InsertsAfterExistingSystem(t *testing.T) {
	hook := newReplyReminderBeforeHook()
	inv := &trpcagent.Invocation{}
	inv.SetState(replyReminderStateKey, true)
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := runBeforeModelHook(t, hook, ctx)
	assertCueAfterBase(t, msgs, "系统提醒")
}

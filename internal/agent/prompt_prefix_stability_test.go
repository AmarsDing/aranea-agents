package agent

// Prefix byte-level stability regression tests (G1-C, 29-token.design.md §9.5).
//
// DeepSeek-style prompt caching matches tokens from position 0: any per-turn
// change inside the cached prefix invalidates the whole block. These tests run
// the REAL product callback chain (productCallbackChain) offline — no LLM, no
// DB — and pin two invariants across two adjacent turns of the same session:
//
//  1. Static zone byte-identical: [base system + static capability cue] must
//     be byte-for-byte equal between turn N and turn N+1, even when the
//     dynamic inputs (user query, intent context) change every turn.
//  2. Dynamic zone tail-only: memory cue / knowledge cue / reply reminder /
//     intent context messages must all live in the trailing append region as
//     user-role sentinels — never inserted between the static zone and the
//     history, and never as extra system messages (DeepSeek prefixes all
//     role=system).
//
// A third test proves the comparator itself is sensitive (not a tautology).

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const prefixStabilityBaseSystem = "PREFIX-STABILITY-BASE-SYSTEM"

// prefixStabilityFixture builds the fixed agent + stub deps used by every
// turn. All DB/LLM-facing collaborators are package-local fakes shared with
// prompt_prefix_position_test.go.
func prefixStabilityFixture() (biz.Agent, TRPCBuilderDeps, func(context.Context) context.Context) {
	ag := biz.Agent{
		ID:               "ag-prefix",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:         true,
			MemoryEnabled:        true,
			L3Enabled:            true,
			L0InjectL3:           true,
			ReplyReminderEnabled: true,
		},
	}
	repo := cueSearchRepo{
		fakeKnowledgeRepos: fakeKnowledgeRepos{
			collections: []biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 1}},
		},
		chunks: []biz.KnowledgeChunk{{ID: "k1", DocID: "d1", CollectionID: "c1", Content: "天气晴，25 度。", Score: 0.9}},
	}
	ret := knowledge.NewRetriever(cueEmbedder{}, repo, nil, loggateway.NewNoop())
	deps := TRPCBuilderDeps{
		TRPCModelCatalogDeps: TRPCModelCatalogDeps{
			AgentUC: fakeTeamAgentLookup{eff: biz.AgentEffectiveTools{ToolsEnabled: true, Profile: "full"}},
		},
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			MemoryProfileCardReader: fakeProfileCardReader{card: &biz.ProfileCard{Content: "偏好：中餐"}},
			KnowledgeUsecase:        biz.NewKnowledgeUsecase(repo, repo, repo),
		},
	}
	return ag, deps, func(ctx context.Context) context.Context {
		ctx = knowledgetool.WithRetriever(ctx, ret)
		return knowledgetool.WithKnowledgeCollections(ctx, []string{"c1"})
	}
}

// prefixStabilityInvocation returns a fresh invocation context for one turn of
// the same session, with the reply-reminder state armed (as the AfterTool hook
// would have left it after a tool call). attach 接入预检索 Retriever，使
// 知识 cue 在空目录不再广告时仍有命中块可钉尾部位置。
func prefixStabilityInvocation(attach func(context.Context) context.Context) context.Context {
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	inv.SetState(replyReminderStateKey, true)
	var ctx context.Context = trpcagent.NewInvocationContext(context.Background(), inv)
	if attach != nil {
		ctx = attach(ctx)
	}
	return ctx
}

// runProductBeforeModelChain executes every BeforeModel hook of the real
// product chain, in chain order, against one request — mirroring how the
// framework drives model callbacks.
func runProductBeforeModelChain(t *testing.T, chain *callbacks.Chain, ctx context.Context, msgs []trpcmodel.Message) []trpcmodel.Message {
	t.Helper()
	if chain == nil {
		t.Fatal("productCallbackChain returned nil; fixture must satisfy hook guards")
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	ran := 0
	for _, cb := range chain.Entries() {
		h, ok := cb.(callbacks.BeforeModelHook)
		if !ok {
			continue
		}
		if _, err := h.HandleBeforeModel(ctx, args); err != nil {
			t.Fatalf("HandleBeforeModel %T: %v", cb, err)
		}
		ran++
	}
	if ran == 0 {
		t.Fatal("chain has no BeforeModel hooks; test would assert nothing")
	}
	return args.Request.Messages
}

// buildTurnMessages simulates framework request assembly: base system prompt,
// then the per-turn intent context injected right after the system block
// (framework content-processor behavior), then history, then the current user
// message. The intent reorder hook is responsible for moving the intent
// context to the tail.
func buildTurnMessages(t *testing.T, refinedGoal string, history []trpcmodel.Message, userQuery string) []trpcmodel.Message {
	t.Helper()
	intentMsg := intent.SystemContextMessage(&intent.Artifact{RefinedGoal: refinedGoal})
	if intentMsg.Role == "" {
		t.Fatal("intent SystemContextMessage returned empty message")
	}
	msgs := []trpcmodel.Message{trpcmodel.NewSystemMessage(prefixStabilityBaseSystem), intentMsg}
	msgs = append(msgs, history...)
	msgs = append(msgs, trpcmodel.NewUserMessage(userQuery))
	return msgs
}

// staticPrefixLen locates the static zone boundary: index 0 is the base system
// prompt, index 1 must be the static runtime capability cue. N=2 per
// 29-token.design.md §9.5 (system 主体 + 静态 cue 边界).
func staticPrefixLen(t *testing.T, msgs []trpcmodel.Message) int {
	t.Helper()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages (base + static cue), got %d", len(msgs))
	}
	if msgs[0].Role != trpcmodel.RoleSystem || msgs[0].Content != prefixStabilityBaseSystem {
		t.Fatalf("base system prompt must remain at index 0, got role=%s content=%.40q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != trpcmodel.RoleSystem || !strings.Contains(msgs[1].Content, "Runtime capability policy") {
		t.Fatalf("static capability cue must be a system message at index 1, got role=%s content=%.60q", msgs[1].Role, msgs[1].Content)
	}
	return 2
}

// firstPrefixDiff returns the index of the first message in [0, n) whose Role
// or Content differs byte-for-byte between a and b, or -1 when identical.
func firstPrefixDiff(a, b []trpcmodel.Message, n int) int {
	if len(a) < n || len(b) < n {
		return 0 // shorter input cannot contain the full static zone
	}
	for i := 0; i < n; i++ {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return i
		}
	}
	return -1
}

func first100Runes(s string) string {
	r := []rune(s)
	if len(r) > 100 {
		return string(r[:100])
	}
	return s
}

// buildTwoAdjacentTurns runs the full chain for turn 1 (no history) and turn 2
// (one assistant+user exchange of history, different intent + user query).
func buildTwoAdjacentTurns(t *testing.T) (turn1, turn2 []trpcmodel.Message) {
	t.Helper()
	ag, deps, attach := prefixStabilityFixture()
	chain := productCallbackChain(context.Background(), ag, deps, nil)

	turn1 = runProductBeforeModelChain(t, chain, prefixStabilityInvocation(attach),
		buildTurnMessages(t, "查天气", nil, "今天天气怎么样"))

	history := []trpcmodel.Message{
		trpcmodel.NewUserMessage("今天天气怎么样"),
		trpcmodel.NewAssistantMessage("今天晴，25 度。"),
	}
	turn2 = runProductBeforeModelChain(t, chain, prefixStabilityInvocation(attach),
		buildTurnMessages(t, "订机票", history, "帮我订明天去北京的机票"))
	return turn1, turn2
}

// TestPromptPrefixStability_StaticZoneByteIdentical pins invariant 1: across
// two adjacent turns of the same session — with different user queries and
// different intent contexts — the static zone prefix is byte-identical.
func TestPromptPrefixStability_StaticZoneByteIdentical(t *testing.T) {
	turn1, turn2 := buildTwoAdjacentTurns(t)
	n := staticPrefixLen(t, turn1)
	staticPrefixLen(t, turn2)
	if diff := firstPrefixDiff(turn1, turn2, n); diff >= 0 {
		t.Fatalf("static zone diverged at message index %d:\n turn1: %.100q\n turn2: %.100q",
			diff, first100Runes(turn1[diff].Content), first100Runes(turn2[diff].Content))
	}
}

// TestPromptPrefixStability_DynamicCuesOnlyInTail pins invariant 2: every
// dynamic system message (reply reminder / memory cue / knowledge cue /
// intent context) lives in the trailing append region; the zone between the
// head system block and the last user message carries no system message.
func TestPromptPrefixStability_DynamicCuesOnlyInTail(t *testing.T) {
	_, turn2 := buildTwoAdjacentTurns(t)

	// Head system block: contiguous system messages from index 0.
	headEnd := 0
	for headEnd < len(turn2) && turn2[headEnd].Role == trpcmodel.RoleSystem {
		headEnd++
	}
	if headEnd < 2 {
		t.Fatalf("head system block too small (%d messages); static zone missing", headEnd)
	}

	// After the head, no extra system messages may appear — DeepSeek treats
	// every role=system as prefix. Dynamic cues are user-role sentinels in
	// one contiguous trailing block.
	for i := headEnd; i < len(turn2); i++ {
		if turn2[i].Role == trpcmodel.RoleSystem {
			t.Fatalf("system message at index %d after the static head; dynamic cues must not be role=system", i)
		}
	}
	tailStart := -1
	for i := headEnd; i < len(turn2); i++ {
		if isDynamicCueMessage(turn2[i]) {
			tailStart = i
			break
		}
	}
	if tailStart < 0 {
		t.Fatal("no dynamic cue found in the tail; fixture must produce memory/knowledge/reminder/intent cues")
	}
	for i := tailStart; i < len(turn2); i++ {
		if !isDynamicCueMessage(turn2[i]) {
			t.Fatalf("non-cue message (role=%s tool=%q) at index %d inside the dynamic tail", turn2[i].Role, turn2[i].ToolName, i)
		}
	}

	// Every expected dynamic cue class must be present in the tail.
	tail := turn2[tailStart:]
	contains := func(match func(string) bool) bool {
		for _, m := range tail {
			if match(m.Content) {
				return true
			}
		}
		return false
	}
	if !contains(func(c string) bool { return strings.Contains(c, "系统提醒") }) {
		t.Fatal("reply reminder missing from dynamic tail")
	}
	if !contains(func(c string) bool { return strings.Contains(c, "用户档案") }) {
		t.Fatal("memory cue missing from dynamic tail")
	}
	if !contains(func(c string) bool { return strings.Contains(c, "Retrieved Knowledge") }) {
		t.Fatal("knowledge cue missing from dynamic tail")
	}
	if !contains(intent.IsIntentContextContent) {
		t.Fatal("intent context missing from dynamic tail")
	}
	// The intent context must be the very last message (reorder hook contract).
	if last := turn2[len(turn2)-1]; !intent.IsIntentContextContent(last.Content) {
		t.Fatalf("intent context must be the final message, got %.60q", last.Content)
	}
}

// TestPromptPrefixStability_ComparatorDetectsDrift proves the byte-level
// comparison is not a tautology: flipping one byte inside the static zone must
// be detected at the exact message index.
func TestPromptPrefixStability_ComparatorDetectsDrift(t *testing.T) {
	turn1, turn2 := buildTwoAdjacentTurns(t)
	n := staticPrefixLen(t, turn1)
	if firstPrefixDiff(turn1, turn2, n) != -1 {
		t.Fatal("baseline must be stable before injecting drift")
	}
	// Corrupt a copy of turn2's static cue by one byte.
	corrupted := make([]trpcmodel.Message, len(turn2))
	copy(corrupted, turn2)
	corrupted[1].Content = corrupted[1].Content + " "
	diff := firstPrefixDiff(turn1, corrupted, n)
	if diff != 1 {
		t.Fatalf("comparator must detect the injected drift at index 1, got %d", diff)
	}
}

package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// l4EntityStoreMock emulates the production SQL semantics of ListEntityRows:
// keyword filters via `name_normalized LIKE '%' || keyword || '%'` (the whole
// keyword must be a substring of the entity name). This is the behavior that
// makes a full user sentence match nothing.
type l4EntityStoreMock struct {
	rows            [][]byte
	capturedKeyword string
	capturedLimit   int32
}

func (m *l4EntityStoreMock) ListEntityRows(_ context.Context, _, _ string, _, _, _, status, keyword string, limit, _ int32) ([][]byte, int32, error) {
	m.capturedKeyword = keyword
	m.capturedLimit = limit
	if keyword == "" {
		return m.rows, int32(len(m.rows)), nil
	}
	kw := strings.ToLower(keyword)
	var out [][]byte
	for _, raw := range m.rows {
		name := entityNameFromRow(raw)
		if strings.Contains(strings.ToLower(name), kw) {
			out = append(out, raw)
		}
	}
	return out, int32(len(out)), nil
}

func (m *l4EntityStoreMock) NeighborhoodJSON(_ context.Context, _ string, _, _ int32, _ string) ([]byte, error) {
	return nil, nil
}

func (m *l4EntityStoreMock) AgentIdentityJSON(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (m *l4EntityStoreMock) AgentStrategyJSON(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (m *l4EntityStoreMock) DeleteSessionEventEntities(_ context.Context, _ string) error {
	return nil
}

func entityNameFromRow(raw []byte) string {
	name, _ := strings.CutPrefix(string(raw), `{"name":"`)
	name, _, _ = strings.Cut(name, `"`)
	return name
}

func l4TestPolicy() biz.MemoryRuntimePolicy {
	return biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L4Enabled:     true,
		L0InjectL4:    true,
		L0L4MaxPaths:  3,
	})
}

// RED: a normal multi-clause Chinese user sentence can never be a substring of
// a short entity name, so the SQL LIKE filter starves the cue. The cue must
// still inject the agent's L4 entities (mention-ranked, then recency).
func TestL4MemoryCue_LongSentenceKeywordStillInjects(t *testing.T) {
	store := &l4EntityStoreMock{rows: [][]byte{
		[]byte(`{"id":"e1","name":"测试用户张三","entity_type":"person","confidence":0.9}`),
		[]byte(`{"id":"e2","name":"喝咖啡","entity_type":"preference","confidence":0.8}`),
	}}
	ag := biz.Agent{ID: "ag1"}
	keyword := "请直接回答：我叫什么名字？我喜欢喝什么？我的猫叫什么？"
	got, ids := L4MemoryCue(context.Background(), store, ag, l4TestPolicy(), keyword, nil)
	if got == "" {
		t.Fatalf("expected non-empty L4 cue for long-sentence keyword, got empty")
	}
	if !containsAll(got, "L4 knowledge graph") {
		t.Fatalf("cue missing graph header: %q", got)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 recalled entity IDs, got %v", ids)
	}
}

// Mention ranking: when the user message literally contains an entity name,
// that entity must be selected even with maxPaths=1.
func TestL4MemoryCue_MentionedEntityRankedFirst(t *testing.T) {
	store := &l4EntityStoreMock{rows: [][]byte{
		[]byte(`{"id":"e1","name":"测试用户张三","entity_type":"person","confidence":0.9}`),
		[]byte(`{"id":"e2","name":"喝咖啡","entity_type":"preference","confidence":0.8}`),
	}}
	ag := biz.Agent{ID: "ag1"}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L4Enabled:     true,
		L0InjectL4:    true,
		L0L4MaxPaths:  1,
	})
	got, ids := L4MemoryCue(context.Background(), store, ag, policy, "我还喜欢喝咖啡吗", nil)
	if got == "" {
		t.Fatalf("expected non-empty L4 cue, got empty")
	}
	if !strings.Contains(got, "喝咖啡") {
		t.Fatalf("mentioned entity 喝咖啡 must be selected with maxPaths=1, got %q", got)
	}
	if strings.Contains(got, "测试用户张三") {
		t.Fatalf("unmentioned entity must be trimmed by maxPaths=1, got %q", got)
	}
	if len(ids) != 1 || ids[0] != "e2" {
		t.Fatalf("expected recalled IDs [e2] (mentioned entity only), got %v", ids)
	}
}

// Empty KG → empty cue (no regression).
func TestL4MemoryCue_EmptyGraph(t *testing.T) {
	store := &l4EntityStoreMock{}
	ag := biz.Agent{ID: "ag1"}
	got, ids := L4MemoryCue(context.Background(), store, ag, l4TestPolicy(), "随便聊聊", nil)
	if got != "" {
		t.Fatalf("expected empty cue for empty graph, got %q", got)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no recalled IDs for empty graph, got %v", ids)
	}
}

// Low-confidence entities are still filtered by the 0.3 gate after the fix.
func TestL4MemoryCue_ConfidenceGateStillApplies(t *testing.T) {
	store := &l4EntityStoreMock{rows: [][]byte{
		[]byte(`{"id":"e1","name":"弱线索","entity_type":"concept","confidence":0.1}`),
	}}
	ag := biz.Agent{ID: "ag1"}
	got, ids := L4MemoryCue(context.Background(), store, ag, l4TestPolicy(), "弱线索是什么", nil)
	if got != "" {
		t.Fatalf("expected empty cue when all entities below confidence gate, got %q", got)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no recalled IDs below confidence gate, got %v", ids)
	}
}

func TestL4MemoryCue_NullIdentityJSONNotInjected(t *testing.T) {
	store := &l4NullIdentityStore{}
	ag := biz.Agent{ID: "ag1"}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled:    true,
		L4Enabled:        true,
		L0InjectL4:       true,
		L4IdentityInject: true,
	})
	got, _ := L4MemoryCue(context.Background(), store, ag, policy, "介绍你自己", nil)
	if strings.Contains(got, "L4 agent identity") || strings.Contains(got, `"identity":null`) {
		t.Fatalf("empty identity JSON must not be injected, got %q", got)
	}
}

func TestFormatL4JSONBlock_SkipsVacuousIdentity(t *testing.T) {
	if block := formatL4JSONBlock("L4 agent identity", []byte(`{"agent_id":"ag1","identity":null}`), 2000); block != "" {
		t.Fatalf("null identity must be empty, got %q", block)
	}
	if block := formatL4JSONBlock("L4 agent identity", []byte(`{"identity":{"persona":"ops"}}`), 2000); block == "" {
		t.Fatal("populated identity must inject")
	}
}

type l4NullIdentityStore struct{ l4EntityStoreMock }

func (m *l4NullIdentityStore) AgentIdentityJSON(_ context.Context, agentID string) ([]byte, error) {
	return []byte(`{"agent_id":"` + agentID + `","identity":null}`), nil
}

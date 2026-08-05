package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// fakeProfileFactLister implements biz.MemoryPreferenceLister for testing.
type fakeProfileFactLister struct {
	rows [][]byte
	err  error
}

func (f *fakeProfileFactLister) ListActivePreferenceFacts(_ context.Context, _, _ string, _ []string, _ int32) ([][]byte, error) {
	return f.rows, f.err
}

// fakeProfileCardWriter implements biz.MemoryProfileCardWriter for testing.
type fakeProfileCardWriter struct {
	upserted []biz.ProfileCard
	deleted  [][2]string
	err      error
}

func (f *fakeProfileCardWriter) UpsertProfileCard(_ context.Context, card biz.ProfileCard) error {
	if f.err != nil {
		return f.err
	}
	f.upserted = append(f.upserted, card)
	return nil
}

func (f *fakeProfileCardWriter) DeleteProfileCard(_ context.Context, agentID, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, [2]string{agentID, userID})
	return nil
}

func profileCardTestUK() trpcmemory.UserKey {
	return trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
}

func TestProfileCardDistill_UpsertsDistilledCard(t *testing.T) {
	lister := &fakeProfileFactLister{rows: [][]byte{
		[]byte(`{"fact_kind":"preference","statement":"喜欢咖啡"}`),
		[]byte(`{"fact_kind":"profile","statement":"名叫张三"}`),
		[]byte(`{malformed`),           // skipped by parseProfileCardFacts
		[]byte(`{"fact_kind":"goal"}`), // empty statement, skipped
	}}
	writer := &fakeProfileCardWriter{}
	d := NewProfileCardDistiller(lister, writer,
		&fakeModel{response: buildLLMResponse("- 名叫张三\n- 喜欢咖啡")}, loggateway.NewNoop())

	d.Distill(context.Background(), profileCardTestUK())

	if len(writer.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(writer.upserted))
	}
	card := writer.upserted[0]
	if card.AgentID != "agent-1" || card.UserID != "user-1" {
		t.Fatalf("wrong card target: %+v", card)
	}
	if card.FactCount != 2 {
		t.Fatalf("expected FactCount=2 (malformed/empty rows skipped), got %d", card.FactCount)
	}
	if !strings.Contains(card.Content, "喜欢咖啡") {
		t.Fatalf("unexpected card content: %q", card.Content)
	}
	if len(writer.deleted) != 0 {
		t.Fatalf("expected no delete, got %v", writer.deleted)
	}
}

func TestProfileCardDistill_ZeroFactsDeletesStaleCard(t *testing.T) {
	lister := &fakeProfileFactLister{rows: nil}
	writer := &fakeProfileCardWriter{}
	d := NewProfileCardDistiller(lister, writer,
		&fakeModel{response: buildLLMResponse("unused")}, loggateway.NewNoop())

	d.Distill(context.Background(), profileCardTestUK())

	if len(writer.deleted) != 1 || writer.deleted[0] != [2]string{"agent-1", "user-1"} {
		t.Fatalf("expected stale card delete for (agent-1,user-1), got %v", writer.deleted)
	}
	if len(writer.upserted) != 0 {
		t.Fatalf("expected no upsert, got %v", writer.upserted)
	}
}

func TestProfileCardDistill_LLMFailureKeepsOldCard(t *testing.T) {
	lister := &fakeProfileFactLister{rows: [][]byte{[]byte(`{"fact_kind":"preference","statement":"喜欢咖啡"}`)}}
	writer := &fakeProfileCardWriter{}
	d := NewProfileCardDistiller(lister, writer,
		&fakeModel{err: errors.New("llm unavailable")}, loggateway.NewNoop())

	d.Distill(context.Background(), profileCardTestUK())

	if len(writer.upserted) != 0 || len(writer.deleted) != 0 {
		t.Fatalf("expected no writes on LLM failure, got upserted=%v deleted=%v", writer.upserted, writer.deleted)
	}
}

func TestProfileCardDistill_NilLLMKeepsOldCard(t *testing.T) {
	lister := &fakeProfileFactLister{rows: [][]byte{[]byte(`{"fact_kind":"preference","statement":"喜欢咖啡"}`)}}
	writer := &fakeProfileCardWriter{}
	d := NewProfileCardDistiller(lister, writer, nil, loggateway.NewNoop())
	d.SetLLMResolver(&fakeLLMResolver{model: nil})

	d.Distill(context.Background(), profileCardTestUK())

	if len(writer.upserted) != 0 || len(writer.deleted) != 0 {
		t.Fatalf("expected no writes with nil LLM, got upserted=%v deleted=%v", writer.upserted, writer.deleted)
	}
}

func TestProfileCardDistill_ListerErrorKeepsOldCard(t *testing.T) {
	lister := &fakeProfileFactLister{err: errors.New("db down")}
	writer := &fakeProfileCardWriter{}
	d := NewProfileCardDistiller(lister, writer,
		&fakeModel{response: buildLLMResponse("unused")}, loggateway.NewNoop())

	d.Distill(context.Background(), profileCardTestUK())

	if len(writer.upserted) != 0 || len(writer.deleted) != 0 {
		t.Fatalf("expected no writes on lister error, got upserted=%v deleted=%v", writer.upserted, writer.deleted)
	}
}

func TestProfileCardDistill_ResolverTakesPrecedence(t *testing.T) {
	lister := &fakeProfileFactLister{rows: [][]byte{[]byte(`{"fact_kind":"preference","statement":"喜欢咖啡"}`)}}
	writer := &fakeProfileCardWriter{}
	// Static LLM errors; the resolver's LLM succeeds — a distilled card proves
	// the resolver path won.
	resolver := &fakeLLMResolver{model: &fakeModel{response: buildLLMResponse("- 喜欢咖啡")}}
	d := NewProfileCardDistiller(lister, writer,
		&fakeModel{err: errors.New("static llm must not be used")}, loggateway.NewNoop())
	d.SetLLMResolver(resolver)

	d.Distill(context.Background(), profileCardTestUK())

	if resolver.calls != 1 {
		t.Fatalf("expected resolver called once, got %d", resolver.calls)
	}
	if len(writer.upserted) != 1 {
		t.Fatalf("expected resolver LLM to produce card, got %v", writer.upserted)
	}
}

func TestSanitizeProfileCardContent(t *testing.T) {
	// Code fence + markdown headers stripped, content kept.
	got := sanitizeProfileCardContent("```markdown\n# 用户档案\n- 喜欢咖啡\n## 偏好\n- 浅色模式\n```")
	if strings.Contains(got, "```") || strings.Contains(got, "#") {
		t.Fatalf("fence/headers not stripped: %q", got)
	}
	if !strings.Contains(got, "- 喜欢咖啡") || !strings.Contains(got, "- 浅色模式") {
		t.Fatalf("content lost: %q", got)
	}

	// Rune budget enforced with ellipsis.
	long := strings.Repeat("档", profileCardMaxRunes+100)
	got = sanitizeProfileCardContent(long)
	if runes := []rune(got); len(runes) != profileCardMaxRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected %d runes + ellipsis, got %d runes", profileCardMaxRunes, len([]rune(got)))
	}

	// Blank input stays blank.
	if got := sanitizeProfileCardContent("  \n "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

var _ trpcmodel.Model = (*fakeModel)(nil) // compile guard: fakeModel satisfies the distiller's LLM contract

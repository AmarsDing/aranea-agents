package biz_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── Test doubles ────────────────────────────────────────────────────────────

type draftStoreRow struct {
	sug  biz.UnifiedEvolutionSuggestion
	meta map[string]string
}

type draftStore struct {
	biz.UnifiedEvolutionStore
	rows []*draftStoreRow
}

func (s *draftStore) add(typ, payload string) *draftStoreRow {
	meta := map[string]string{
		biz.EvoMetaLegacyType: typ,
		biz.EvoMetaTitle:      "指标通知",
	}
	row := &draftStoreRow{
		sug: biz.UnifiedEvolutionSuggestion{
			ID:            fmt.Sprintf("sug-%d", len(s.rows)+1),
			TargetType:    biz.EvolutionTargetAgent,
			TargetID:      "agent-1",
			ActionType:    biz.EvolutionActionEvolve,
			TriggerSource: "agent_config",
			Status:        "pending",
			DraftBody:     "近30d负反馈 10 次（阈值 2）。建议审阅相关配置。",
			CreatedAt:     time.Now(),
		},
		meta: meta,
	}
	if payload != "" {
		meta[biz.EvoMetaApplyPayload] = payload
	}
	row.syncMeta()
	s.rows = append(s.rows, row)
	return row
}

func (r *draftStoreRow) syncMeta() {
	raw, _ := json.Marshal(r.meta)
	r.sug.Metadata = raw
}

func (s *draftStore) ListByTargetAndAction(_ context.Context, targetType, targetID, actionType, _ string, status string, _, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	var out []biz.UnifiedEvolutionSuggestion
	for _, r := range s.rows {
		if r.sug.Status == status {
			out = append(out, r.sug)
		}
	}
	return out, nil
}

func (s *draftStore) UpdateMetadataKey(_ context.Context, id, key, value string) error {
	for _, r := range s.rows {
		if r.sug.ID == id {
			r.meta[key] = value
			r.syncMeta()
			return nil
		}
	}
	return errors.New("not found")
}

func (s *draftStore) metaOf(id, key string) string {
	for _, r := range s.rows {
		if r.sug.ID == id {
			return r.meta[key]
		}
	}
	return ""
}

type draftAgentReader struct {
	biz.AgentRepository
	agent biz.Agent
	files []biz.AgentPromptFile
}

func (r *draftAgentReader) GetAgentByID(context.Context, string) (biz.Agent, error) {
	return r.agent, nil
}
func (r *draftAgentReader) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return r.files, nil
}

type draftLLM struct {
	resp  string
	err   error
	calls int
	last  biz.LLMCallRequest
}

func (l *draftLLM) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	l.calls++
	l.last = req
	return l.resp, 100, l.err
}

type draftSettings struct{ maxPersona int }

func (s draftSettings) GetAgentRuntimeSettings(context.Context, string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{EvoPersonaMaxChars: s.maxPersona}, nil
}

func newDrafter(store *draftStore, agents *draftAgentReader, llm *draftLLM, maxPersona int) *biz.EvolutionDrafter {
	return biz.NewEvolutionDrafter(store, agents, draftSettings{maxPersona: maxPersona}, llm, nil, loggateway.NewNoop())
}

// ── Tests ───────────────────────────────────────────────────────────────────

func TestEvolutionDrafter_PersonaNotificationGetsDraft(t *testing.T) {
	store := &draftStore{}
	row := store.add("persona", "")
	agents := &draftAgentReader{
		agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"},
		files: []biz.AgentPromptFile{{AgentID: "agent-1", Name: "IDENTITY.md", Body: "# IDENTITY\n\n## Persona\n\n原有语气。", SortOrder: 30}},
	}
	llm := &draftLLM{resp: "语气更沉稳，先安抚再给方案。"}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if llm.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", llm.calls)
	}
	payload := store.metaOf(row.sug.ID, biz.EvoMetaApplyPayload)
	if payload != "语气更沉稳，先安抚再给方案。" {
		t.Errorf("payload = %q", payload)
	}
	diff := store.metaOf(row.sug.ID, biz.EvoMetaDiffPreview)
	if diff == "" || !strings.Contains(diff, "原有语气") {
		t.Errorf("diff_preview should contain old persona, got %q", diff)
	}
	// applicable 语义：payload + persona 类型 → 可应用
	view := biz.EvolutionSuggestion{Type: "persona", ApplyPayload: payload}
	if !view.Applicable() {
		t.Error("suggestion should become applicable after drafting")
	}
	// LLM 输入应包含当前 persona 与通知内容
	if !strings.Contains(llm.last.User, "原有语气") || !strings.Contains(llm.last.User, "负反馈") {
		t.Errorf("LLM user prompt missing context: %q", llm.last.User)
	}
	if llm.last.Provider != "openai" || llm.last.Model != "gpt-x" {
		t.Errorf("should use agent's own model, got %s/%s", llm.last.Provider, llm.last.Model)
	}
}

func TestEvolutionDrafter_PromptNotificationGetsFullFileDraft(t *testing.T) {
	store := &draftStore{}
	row := store.add("prompt", "")
	original := "# AGENTS\n\n## 工具策略\n\n优先使用只读工具。\n\n## 其他\n\n保持不变。"
	agents := &draftAgentReader{
		agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"},
		files: []biz.AgentPromptFile{{AgentID: "agent-1", Name: "AGENTS_CORE.md", Body: original, SortOrder: 10}},
	}
	revised := "# AGENTS\n\n## 工具策略\n\n优先使用只读工具；失败时降级并说明原因。\n\n## 其他\n\n保持不变。"
	llm := &draftLLM{resp: revised}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if got := store.metaOf(row.sug.ID, biz.EvoMetaApplyPayload); got != revised {
		t.Errorf("payload = %q", got)
	}
	if diff := store.metaOf(row.sug.ID, biz.EvoMetaDiffPreview); !strings.Contains(diff, "降级") {
		t.Errorf("diff should highlight change, got %q", diff)
	}
}

func TestEvolutionDrafter_PromptDraftTooShortDiscarded(t *testing.T) {
	store := &draftStore{}
	row := store.add("prompt", "")
	original := strings.Repeat("很长的系统提示内容。", 50) // 500 chars
	agents := &draftAgentReader{
		agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"},
		files: []biz.AgentPromptFile{{AgentID: "agent-1", Name: "AGENTS_CORE.md", Body: original, SortOrder: 10}},
	}
	llm := &draftLLM{resp: "太短"}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if got := store.metaOf(row.sug.ID, biz.EvoMetaApplyPayload); got != "" {
		t.Errorf("truncated draft must be discarded, got %q", got)
	}
}

func TestEvolutionDrafter_LLMFailureKeepsNotificationAndMarksAttempt(t *testing.T) {
	store := &draftStore{}
	row := store.add("persona", "")
	agents := &draftAgentReader{
		agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"},
		files: []biz.AgentPromptFile{{AgentID: "agent-1", Name: "IDENTITY.md", Body: "## Persona\n\n旧。", SortOrder: 30}},
	}
	llm := &draftLLM{err: errors.New("boom")}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("LLM failure must degrade silently, got %v", err)
	}
	if got := store.metaOf(row.sug.ID, biz.EvoMetaApplyPayload); got != "" {
		t.Errorf("payload must stay empty on failure, got %q", got)
	}
	if got := store.metaOf(row.sug.ID, biz.EvoMetaDraftAttemptAt); got == "" {
		t.Error("draft_attempt_at should be recorded after failed attempt")
	}
}

func TestEvolutionDrafter_ThrottleSkipsRecentAttempt(t *testing.T) {
	store := &draftStore{}
	row := store.add("persona", "")
	row.meta[biz.EvoMetaDraftAttemptAt] = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	row.syncMeta()
	agents := &draftAgentReader{agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"}}
	llm := &draftLLM{resp: "x"}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("attempt within 1h must be throttled, llm.calls=%d", llm.calls)
	}
}

func TestEvolutionDrafter_SkipsNonDraftableRows(t *testing.T) {
	store := &draftStore{}
	store.add("skill", "")                      // skill 类型不生成
	store.add("orchestration_optimization", "") // 编排通知不生成
	store.add("persona", "已有 payload 不再生成")     // 已就绪
	agents := &draftAgentReader{agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"}}
	llm := &draftLLM{resp: "x"}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("no draftable row, llm.calls=%d", llm.calls)
	}
}

func TestEvolutionDrafter_NoModelSkips(t *testing.T) {
	store := &draftStore{}
	store.add("persona", "")
	agents := &draftAgentReader{agent: biz.Agent{ID: "agent-1"}} // 无 provider/model
	llm := &draftLLM{resp: "x"}
	d := newDrafter(store, agents, llm, 0)

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("no model resolved, llm.calls=%d", llm.calls)
	}
}

func TestEvolutionDrafter_PersonaMaxCharsEnforced(t *testing.T) {
	store := &draftStore{}
	row := store.add("persona", "")
	agents := &draftAgentReader{
		agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"},
		files: []biz.AgentPromptFile{{AgentID: "agent-1", Name: "IDENTITY.md", Body: "## Persona\n\n旧。", SortOrder: 30}},
	}
	llm := &draftLLM{resp: strings.Repeat("长", 100)}
	d := newDrafter(store, agents, llm, 50) // max 50 chars

	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DraftPending: %v", err)
	}
	payload := store.metaOf(row.sug.ID, biz.EvoMetaApplyPayload)
	if len([]rune(payload)) > 50 {
		t.Errorf("payload exceeds EvoPersonaMaxChars: %d runes", len([]rune(payload)))
	}
}

func TestEvolutionDrafter_NilLLMNoop(t *testing.T) {
	store := &draftStore{}
	store.add("persona", "")
	agents := &draftAgentReader{agent: biz.Agent{ID: "agent-1", Provider: "openai", Model: "gpt-x"}}
	d := biz.NewEvolutionDrafter(store, agents, nil, nil, nil, loggateway.NewNoop())
	if err := d.DraftPending(context.Background(), "agent-1"); err != nil {
		t.Fatalf("nil LLM must be a no-op, got %v", err)
	}
}

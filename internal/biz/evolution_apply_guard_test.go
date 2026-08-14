package biz_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Test doubles ────────────────────────────────────────────────────────────

type applyGuardAgentRepo struct {
	biz.AgentRepository
	files       []biz.AgentPromptFile
	replaceCall int
}

func (r *applyGuardAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return r.files, nil
}

func (r *applyGuardAgentRepo) ReplaceAgentPromptFiles(_ context.Context, _ string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	r.replaceCall++
	r.files = files
	return files, nil
}

type applyGuardStore struct {
	biz.UnifiedEvolutionStore
	row          *biz.UnifiedEvolutionSuggestion
	statusUpdate string
	statusActor  string
	statusReason string
	// casForceMiss simulates a concurrent transition winning the race between
	// the usecase's read and the atomic UPDATE: UpdateStatusCAS returns
	// ok=false so tests can assert the Conflict mapping (B-1).
	casForceMiss bool
}

func (s *applyGuardStore) Create(_ context.Context, suggestion biz.UnifiedEvolutionSuggestion) error {
	row := suggestion
	s.row = &row
	return nil
}

func (s *applyGuardStore) GetByID(context.Context, string) (*biz.UnifiedEvolutionSuggestion, error) {
	return s.row, nil
}

func (s *applyGuardStore) UpdateStatus(_ context.Context, _ string, status string, actor string, reason string) error {
	s.statusUpdate = status
	s.statusActor = actor
	s.statusReason = reason
	s.row.Status = status
	return nil
}

// UpdateStatusCAS mirrors the repo's atomic precondition: ok=false when the
// row's current status is not in `from`, or when casForceMiss is set.
func (s *applyGuardStore) UpdateStatusCAS(_ context.Context, _ string, from []string, status string, actor string, reason string) (bool, error) {
	if s.casForceMiss {
		return false, nil
	}
	if len(from) > 0 {
		match := false
		for _, f := range from {
			if s.row.Status == f {
				match = true
				break
			}
		}
		if !match {
			return false, nil
		}
	}
	s.statusUpdate = status
	s.statusActor = actor
	s.statusReason = reason
	s.row.Status = status
	return true, nil
}

func (s *applyGuardStore) UpdateMetadataKey(_ context.Context, _ string, key string, value string) error {
	m := s.row.MetadataMap()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m[key] = raw
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.row.Metadata = data
	return nil
}

func applyGuardRow(legacyType, title, content, applyPayload string) *biz.UnifiedEvolutionSuggestion {
	meta := map[string]string{
		biz.EvoMetaLegacyType: legacyType,
		biz.EvoMetaTitle:      title,
	}
	if applyPayload != "" {
		meta[biz.EvoMetaApplyPayload] = applyPayload
	}
	metadata, _ := json.Marshal(meta)
	return &biz.UnifiedEvolutionSuggestion{
		ID:              "sug-1",
		TargetType:      biz.EvolutionTargetAgent,
		TargetID:        "agent-1",
		ActionType:      biz.EvolutionActionEvolve,
		TriggerSource:   "agent_config",
		TriggerReason:   title,
		Status:          biz.EvolutionStatusPending,
		DraftBody:       content,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       time.Now().UTC(),
	}
}

func newApplyGuardUsecase(store *applyGuardStore, agents *applyGuardAgentRepo) *biz.EvolutionUsecase {
	return biz.NewEvolutionUsecase(nil, store, agents, loggateway.NewNoop())
}

// ── Notification-only suggestions must never touch prompt files ─────────────

func TestApplySuggestion_NotificationPersonaRejected(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("persona", "负反馈累积",
		"近30d负反馈 10 次（阈值 2）。建议审阅 IDENTITY.md ## Persona 语气与工具策略。", "")}
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "IDENTITY.md", Body: "# IDENTITY\n\n## Persona\n\n原有语气。", SortOrder: 30},
	}}
	uc := newApplyGuardUsecase(store, agents)

	_, err := uc.ApplySuggestion(context.Background(), "agent-1", "sug-1")
	if err == nil {
		t.Fatal("notification-only persona suggestion must be rejected")
	}
	if !strings.Contains(err.Error(), "通知") && !strings.Contains(err.Error(), "payload") {
		t.Errorf("error should explain notification-only semantics, got: %v", err)
	}
	if agents.replaceCall != 0 {
		t.Errorf("prompt files must not be modified, replaceCall=%d", agents.replaceCall)
	}
	if store.statusUpdate != "" {
		t.Errorf("status must not change on rejection, got %q", store.statusUpdate)
	}
	// File body untouched.
	if !strings.Contains(agents.files[0].Body, "原有语气") {
		t.Errorf("IDENTITY.md corrupted: %q", agents.files[0].Body)
	}
}

func TestApplySuggestion_NotificationPromptRejected(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("prompt", "工具成功率偏低",
		"近30d工具成功率 45.0%（阈值 75%）。建议检查工具 allow/deny 与 Skill 挂载策略。", "")}
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "AGENTS_CORE.md", Body: "# Core\n\n原始系统提示。", SortOrder: 10},
	}}
	uc := newApplyGuardUsecase(store, agents)

	_, err := uc.ApplySuggestion(context.Background(), "agent-1", "sug-1")
	if err == nil {
		t.Fatal("notification-only prompt suggestion must be rejected")
	}
	if agents.replaceCall != 0 {
		t.Errorf("prompt files must not be modified, replaceCall=%d", agents.replaceCall)
	}
	if !strings.Contains(agents.files[0].Body, "原始系统提示") {
		t.Errorf("AGENTS_CORE.md corrupted: %q", agents.files[0].Body)
	}
}

// ── Suggestions carrying an explicit apply payload remain applicable ────────

func TestApplySuggestion_PersonaPayloadApplied(t *testing.T) {
	payload := "回复先给结论，再给推理过程；避免客套话。"
	store := &applyGuardStore{row: applyGuardRow("persona", "优化沟通风格",
		"基于负反馈生成的 persona 调整。", payload)}
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "IDENTITY.md", Body: "# IDENTITY\n\n## Persona\n\n旧 persona。", SortOrder: 30},
	}}
	uc := newApplyGuardUsecase(store, agents)

	got, err := uc.ApplySuggestion(context.Background(), "agent-1", "sug-1")
	if err != nil {
		t.Fatalf("apply with payload should succeed: %v", err)
	}
	if got.Status != biz.EvolutionStatusApplied {
		t.Errorf("status = %q, want applied", got.Status)
	}
	if agents.replaceCall != 1 {
		t.Fatalf("replaceCall = %d, want 1", agents.replaceCall)
	}
	body := agents.files[0].Body
	if !strings.Contains(body, payload) {
		t.Errorf("persona payload not written: %q", body)
	}
	if strings.Contains(body, "旧 persona") {
		t.Errorf("old persona should be replaced: %q", body)
	}
}

func TestApplySuggestion_PromptPayloadApplied(t *testing.T) {
	payload := "# Core\n\n你是严谨的代码审查助手。"
	store := &applyGuardStore{row: applyGuardRow("prompt", "重写系统提示",
		"基于工具成功率生成的 prompt 草稿。", payload)}
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "AGENTS_CORE.md", Body: "# Core\n\n旧提示。", SortOrder: 10},
	}}
	uc := newApplyGuardUsecase(store, agents)

	got, err := uc.ApplySuggestion(context.Background(), "agent-1", "sug-1")
	if err != nil {
		t.Fatalf("apply with payload should succeed: %v", err)
	}
	if got.Status != biz.EvolutionStatusApplied {
		t.Errorf("status = %q, want applied", got.Status)
	}
	if agents.files[0].Body != payload {
		t.Errorf("AGENTS_CORE.md = %q, want payload %q", agents.files[0].Body, payload)
	}
}

// ── Reject: 原因必须透传到 store（持久化 metadata.rejection_reason）─────────

func TestRejectSuggestion_PersistsReason(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("prompt", "工具成功率偏低",
		"近30d工具成功率 45.0%。建议检查工具策略。", "")}
	agents := &applyGuardAgentRepo{}
	uc := newApplyGuardUsecase(store, agents)

	got, err := uc.RejectSuggestion(context.Background(), "agent-1", "sug-1", "当前场景不适用")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got.Status != biz.EvolutionStatusRejected {
		t.Errorf("status = %q, want rejected", got.Status)
	}
	if store.statusReason != "当前场景不适用" {
		t.Errorf("reason not persisted: got %q", store.statusReason)
	}
	if agents.replaceCall != 0 {
		t.Errorf("reject must not touch prompt files, replaceCall=%d", agents.replaceCall)
	}
}

func TestRejectSuggestion_EmptyReasonAllowed(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("persona", "负反馈累积", "建议调整语气。", "")}
	agents := &applyGuardAgentRepo{}
	uc := newApplyGuardUsecase(store, agents)

	if _, err := uc.RejectSuggestion(context.Background(), "agent-1", "sug-1", ""); err != nil {
		t.Fatalf("empty reason must be allowed: %v", err)
	}
	if store.statusUpdate != biz.EvolutionStatusRejected {
		t.Errorf("status = %q, want rejected", store.statusUpdate)
	}
}

func TestRejectSuggestion_NonPendingRejected(t *testing.T) {
	row := applyGuardRow("prompt", "t", "c", "")
	row.Status = biz.EvolutionStatusApplied
	store := &applyGuardStore{row: row}
	uc := newApplyGuardUsecase(store, &applyGuardAgentRepo{})

	if _, err := uc.RejectSuggestion(context.Background(), "agent-1", "sug-1", "r"); err == nil {
		t.Fatal("rejecting an applied suggestion must fail")
	}
}

// ── B-1: CAS miss（GetByID 与原子 UPDATE 之间并发转换抢先）必须映射为 ──────
// ── Conflict，禁止静默覆盖对方状态 ─────────────────────────────────────────

func TestApplySuggestion_CASConflict(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("persona", "优化沟通风格", "内容。", "payload")}
	store.casForceMiss = true
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "IDENTITY.md", Body: "# IDENTITY\n\n## Persona\n\n旧 persona。", SortOrder: 30},
	}}
	uc := newApplyGuardUsecase(store, agents)

	_, err := uc.ApplySuggestion(context.Background(), "agent-1", "sug-1")
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
	if store.row.Status != biz.EvolutionStatusPending {
		t.Errorf("row status must remain untouched on CAS miss, got %s", store.row.Status)
	}
}

func TestRejectSuggestion_CASConflict(t *testing.T) {
	store := &applyGuardStore{row: applyGuardRow("prompt", "工具成功率偏低", "内容。", "")}
	store.casForceMiss = true
	uc := newApplyGuardUsecase(store, &applyGuardAgentRepo{})

	_, err := uc.RejectSuggestion(context.Background(), "agent-1", "sug-1", "不适用")
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
	if store.row.Status != biz.EvolutionStatusPending {
		t.Errorf("row status must remain untouched on CAS miss, got %s", store.row.Status)
	}
}

func TestRollbackSuggestion_CASConflict(t *testing.T) {
	row := applyGuardRow("persona", "t", "c", "payload")
	row.Status = biz.EvolutionStatusApplied
	meta := map[string]string{}
	if err := json.Unmarshal(row.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal row metadata: %v", err)
	}
	snap, _ := json.Marshal(map[string]string{"IDENTITY.md": "# IDENTITY\n\n## Persona\n\n旧 persona。"})
	meta[biz.EvoMetaPreApplySnapshot] = string(snap)
	row.Metadata, _ = json.Marshal(meta)

	store := &applyGuardStore{row: row, casForceMiss: true}
	agents := &applyGuardAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "IDENTITY.md", Body: "# IDENTITY\n\n## Persona\n\n新 persona。", SortOrder: 30},
	}}
	uc := newApplyGuardUsecase(store, agents)

	_, err := uc.RollbackSuggestion(context.Background(), "agent-1", "sug-1")
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
	if store.row.Status != biz.EvolutionStatusApplied {
		t.Errorf("row status must remain untouched on CAS miss, got %s", store.row.Status)
	}
}

// ── Round-trip: view conversion must preserve the payload ───────────────────

func TestEvolutionSuggestionViewRoundTrip_PreservesApplyPayload(t *testing.T) {
	store := &applyGuardStore{}
	agents := &applyGuardAgentRepo{}
	uc := newApplyGuardUsecase(store, agents)

	if _, err := uc.CreateSuggestion(context.Background(), biz.EvolutionSuggestion{
		AgentID:      "agent-1",
		Type:         "persona",
		Title:        "t",
		Content:      "通知描述",
		ApplyPayload: "真正的 persona 内容",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Read back through the same path GetSuggestionByID uses.
	got, err := uc.GetSuggestionByID(context.Background(), store.row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ApplyPayload != "真正的 persona 内容" {
		t.Errorf("ApplyPayload lost in round-trip: %q", got.ApplyPayload)
	}
	if got.Content != "通知描述" {
		t.Errorf("Content changed in round-trip: %q", got.Content)
	}
}

// ── Applicable: 前端按此显隐应用按钮，语义必须与 ApplySuggestion 守卫一致 ──

func TestEvolutionSuggestionApplicable(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		payload string
		want    bool
	}{
		{"persona with payload", "persona", "新 persona 内容", true},
		{"prompt with payload", "prompt", "新系统提示", true},
		{"persona notification only", "persona", "", false},
		{"prompt whitespace payload", "prompt", "   ", false},
		{"skill with payload", "skill", "some draft", false},
		{"orchestration notice", "orchestration_optimization", "", false},
		{"empty type", "", "x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := biz.EvolutionSuggestion{Type: tc.typ, ApplyPayload: tc.payload}
			if got := s.Applicable(); got != tc.want {
				t.Errorf("Applicable() = %v, want %v", got, tc.want)
			}
		})
	}
}

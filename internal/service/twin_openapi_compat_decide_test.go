package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// R3 3.3 C7 桥回写端点测试：POST /api/v1/memory/fact-pending/{id}/decision。

// mfpFakeStore 最小 MemoryFactPendingStore（map 实现）。
type mfpFakeStore struct {
	rows map[string]biz.MemoryFactPendingRecord
}

func (s *mfpFakeStore) InsertPending(_ context.Context, rec biz.MemoryFactPendingRecord) error {
	s.rows[rec.ID] = rec
	return nil
}
func (s *mfpFakeStore) GetPending(_ context.Context, id string) (biz.MemoryFactPendingRecord, bool, error) {
	rec, ok := s.rows[id]
	return rec, ok, nil
}
func (s *mfpFakeStore) ListPending(context.Context, string, string, int) ([]biz.MemoryFactPendingRecord, error) {
	return nil, nil
}
func (s *mfpFakeStore) MarkDecided(_ context.Context, id, status, approver string, decidedAt int64) (bool, error) {
	rec, ok := s.rows[id]
	if !ok || rec.Status != biz.MemoryFactPendingStatusPending {
		return false, nil
	}
	rec.Status, rec.Approver, rec.DecidedAt = status, approver, decidedAt
	s.rows[id] = rec
	return true, nil
}

// mfpFakeWriter 最小 L3FactWriter：记录回放调用。
type mfpFakeWriter struct {
	upserts     []biz.FactUpsert
	invalidated []string
	txUpdates   [][2]string // oldFactID → statement
}

func (w *mfpFakeWriter) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	w.upserts = append(w.upserts, in)
	return nil, nil
}
func (w *mfpFakeWriter) DeleteFactRow(context.Context, string) error                { return nil }
func (w *mfpFakeWriter) DeleteFactRowsByIDs(context.Context, []string) (int, error) { return 0, nil }
func (w *mfpFakeWriter) ClearFactsByScope(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (w *mfpFakeWriter) InvalidateFact(_ context.Context, factID string) ([]byte, error) {
	w.invalidated = append(w.invalidated, factID)
	return nil, nil
}
func (w *mfpFakeWriter) InvalidateAndUpsertFactTx(_ context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	w.txUpdates = append(w.txUpdates, [2]string{oldFactID, in.Statement})
	return nil, nil
}

// newDecideFacadeForTest 构造带真实 Decider（fake 存储/写入器）的门面。
func newDecideFacadeForTest(store biz.MemoryFactPendingStore, writer biz.L3FactWriter) *TwinOpenAPICompatService {
	return newDecideFacadeWithGrants(store, writer, nil, nil)
}

// newDecideFacadeWithGrants 同上新，但注入 E4 免审授权依赖（四档测试）。
func newDecideFacadeWithGrants(store biz.MemoryFactPendingStore, writer biz.L3FactWriter, rules biz.MemoryFactAllowRuleStore, grants *biz.MemoryFactSessionGrants) *TwinOpenAPICompatService {
	s := newTwinFacadeForTest("test-machine-token")
	s.mfpDecider = biz.NewMemoryFactPendingDecider(store, writer, nil, rules, grants, twinNoopLogger{})
	return s
}

func doDecideRequest(t *testing.T, s *TwinOpenAPICompatService, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memory/fact-pending/"+id+"/decision", strings.NewReader(body))
	req.SetPathValue("id", id)
	req.Header.Set("Authorization", "Bearer test-machine-token")
	rec := httptest.NewRecorder()
	s.guard(s.handleMemoryFactPendingDecision)(rec, req)
	return rec
}

// approve UPDATE：置态 + 回放 InvalidateAndUpsertFactTx（原 bi-temporal 写）。
func TestTwinMemoryFactPendingDecisionApproveUpdate(t *testing.T) {
	store := &mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}
	candidate := biz.FactWriteCandidate{Statement: "SRV-DB-03 主库已迁移", AgentID: "agent-1"}
	payload := biz.MarshalFactWriteDecisionSnapshot(biz.FactWriteDecision{Candidate: candidate, TargetFactID: "fact-9"})
	store.rows["mfp-1"] = biz.MemoryFactPendingRecord{
		ID: "mfp-1", AgentID: "agent-1", FactKey: "fact-9",
		Verdict: biz.MemoryFactPendingVerdictUpdate, ProposedBody: candidate.Statement,
		PayloadJSON: payload, Status: biz.MemoryFactPendingStatusPending,
	}
	writer := &mfpFakeWriter{}
	s := newDecideFacadeForTest(store, writer)

	rec := doDecideRequest(t, s, "mfp-1", `{"approved":true,"comment":"核实无误","approver_id":42}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	row := store.rows["mfp-1"]
	if row.Status != biz.MemoryFactPendingStatusApproved || row.Approver != "twinmonitor:42" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if len(writer.txUpdates) != 1 || writer.txUpdates[0][0] != "fact-9" || writer.txUpdates[0][1] != candidate.Statement {
		t.Fatalf("expect tx replay fact-9 → candidate, got %+v", writer.txUpdates)
	}
}

// approve CONTESTED：回放 plain add（UpsertFactRow）。
func TestTwinMemoryFactPendingDecisionApproveContested(t *testing.T) {
	store := &mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}
	candidate := biz.FactWriteCandidate{Statement: "新事实", AgentID: "agent-1"}
	store.rows["mfp-2"] = biz.MemoryFactPendingRecord{
		ID: "mfp-2", AgentID: "agent-1",
		Verdict:      biz.MemoryFactPendingVerdictContested,
		ProposedBody: candidate.Statement,
		PayloadJSON:  biz.MarshalFactWriteDecisionSnapshot(biz.FactWriteDecision{Candidate: candidate}),
		Status:       biz.MemoryFactPendingStatusPending,
	}
	writer := &mfpFakeWriter{}
	s := newDecideFacadeForTest(store, writer)

	rec := doDecideRequest(t, s, "mfp-2", `{"approved":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d", rec.Code)
	}
	if len(writer.upserts) != 1 || writer.upserts[0].Statement != "新事实" {
		t.Fatalf("expect plain add replay, got %+v", writer.upserts)
	}
}

// reject：仅置态，不回放。
func TestTwinMemoryFactPendingDecisionReject(t *testing.T) {
	store := &mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}
	store.rows["mfp-3"] = biz.MemoryFactPendingRecord{
		ID: "mfp-3", Verdict: biz.MemoryFactPendingVerdictDelete, FactKey: "fact-7",
		Status: biz.MemoryFactPendingStatusPending,
	}
	writer := &mfpFakeWriter{}
	s := newDecideFacadeForTest(store, writer)

	rec := doDecideRequest(t, s, "mfp-3", `{"approved":false,"comment":"错误记忆"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d", rec.Code)
	}
	if store.rows["mfp-3"].Status != biz.MemoryFactPendingStatusRejected {
		t.Fatalf("expect rejected, got %+v", store.rows["mfp-3"])
	}
	if len(writer.invalidated) != 0 || len(writer.upserts) != 0 || len(writer.txUpdates) != 0 {
		t.Fatal("reject must not replay any write")
	}
}

// 未知 id / 已决议 → 404（与 twinmonitor 审批状态机对齐）。
func TestTwinMemoryFactPendingDecisionNotFoundOrDecided(t *testing.T) {
	store := &mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}
	store.rows["mfp-4"] = biz.MemoryFactPendingRecord{
		ID: "mfp-4", Verdict: biz.MemoryFactPendingVerdictDelete,
		Status: biz.MemoryFactPendingStatusApproved, // 已决议
	}
	s := newDecideFacadeForTest(store, &mfpFakeWriter{})

	if rec := doDecideRequest(t, s, "mfp-unknown", `{"approved":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: expect 404, got %d", rec.Code)
	}
	if rec := doDecideRequest(t, s, "mfp-4", `{"approved":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("already decided: expect 404, got %d", rec.Code)
	}
}

// decider 未装配 → 503。
func TestTwinMemoryFactPendingDecisionNoDecider(t *testing.T) {
	s := newTwinFacadeForTest("test-machine-token")
	rec := doDecideRequest(t, s, "mfp-1", `{"approved":true}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expect 503, got %d", rec.Code)
	}
}

// 鉴权门：无 token → 401。
func TestTwinMemoryFactPendingDecisionAuth(t *testing.T) {
	s := newDecideFacadeForTest(&mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}, &mfpFakeWriter{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memory/fact-pending/mfp-1/decision", strings.NewReader(`{"approved":true}`))
	req.SetPathValue("id", "mfp-1")
	rec := httptest.NewRecorder()
	s.guard(s.handleMemoryFactPendingDecision)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401, got %d", rec.Code)
	}
}

// 响应体含 replay_error 字段时 JSON 可解析（决议不回滚语义透出）。
func TestTwinMemoryFactPendingDecisionResponseShape(t *testing.T) {
	store := &mfpFakeStore{rows: map[string]biz.MemoryFactPendingRecord{}}
	store.rows["mfp-6"] = biz.MemoryFactPendingRecord{
		ID: "mfp-6", Verdict: biz.MemoryFactPendingVerdictContested,
		ProposedBody: "x", Status: biz.MemoryFactPendingStatusPending,
	}
	s := newDecideFacadeForTest(store, &mfpFakeWriter{})
	rec := doDecideRequest(t, s, "mfp-6", `{"approved":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not json: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("expect ok=true, got %v", out)
	}
}

package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── fakes ─────────────────────────────────────────────────────────────

type copyFactDeps struct {
	MemoryAdminDeps // nil-embedded; only overridden methods are callable

	srcRows [][]byte
	srcErr  error

	listFactCalls int
}

func (f *copyFactDeps) GetFactRowsByIDs(_ context.Context, _ []string) ([][]byte, error) {
	return f.srcRows, f.srcErr
}

func (f *copyFactDeps) ListFactRows(_ context.Context, _, _, _, _, _, _ string, _, _ int32) ([][]byte, int32, int32, int32, error) {
	f.listFactCalls++
	return nil, 0, 0, 0, nil
}

type copyFactWriter struct {
	got   FactUpsert
	raw   []byte
	err   error
	calls int
}

func (w *copyFactWriter) UpsertFactRow(_ context.Context, in FactUpsert) ([]byte, error) {
	w.calls++
	w.got = in
	return w.raw, w.err
}

func (w *copyFactWriter) DeleteFactRow(context.Context, string) error         { return nil }
func (w *copyFactWriter) DeleteFactRowsByIDs(context.Context, []string) (int, error) {
	return 0, nil
}
func (w *copyFactWriter) ClearFactsByScope(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (w *copyFactWriter) InvalidateFact(context.Context, string) ([]byte, error) { return nil, nil }
func (w *copyFactWriter) InvalidateAndUpsertFactTx(_ context.Context, _ string, in FactUpsert) ([]byte, error) {
	w.calls++
	w.got = in
	return w.raw, w.err
}

type copyAuditRecorder struct {
	entries []AuditLog
	err     error
}

func (r *copyAuditRecorder) InsertAuditLog(_ context.Context, e AuditLog) error {
	r.entries = append(r.entries, e)
	return r.err
}

func copySourceRowJSON(stmt, details, kind, scopeType, scopeID string, pii bool, meta string) []byte {
	m := map[string]any{
		"id":              "src-fact-1",
		"scope_type":      scopeType,
		"scope_id":        scopeID,
		"statement":       stmt,
		"details_markdown": details,
		"fact_kind":       kind,
		"confidence":      0.8,
		"importance":      0.6,
		"pii_flag":        pii,
		"metadata_json":   meta,
	}
	raw, _ := json.Marshal(m)
	return raw
}

func newCopyUsecase(deps *copyFactDeps, w *copyFactWriter, rec *copyAuditRecorder) *MemoryAdminUsecase {
	uc := NewMemoryAdminUsecase(deps, nil, nil, w, loggateway.NewNoop())
	if rec != nil {
		uc.SetMemoryFactCopyAuditRecorder(rec)
	}
	return uc
}

// TestCopyFactRow_UserNoteToWorkspace 钉死 acceptance #3 主路径：user note →
// workspace，字段继承 + SourceKind=copy + 血缘 metadata + 审计无明文。
func TestCopyFactRow_UserNoteToWorkspace(t *testing.T) {
	src := copySourceRowJSON("季度复盘：重点是交付节奏", "细节：Q3 OKR", "note", "user", "42", false, "{}")
	deps := &copyFactDeps{srcRows: [][]byte{src}}
	w := &copyFactWriter{raw: []byte(`{"id":"new-fact-9","scope_type":"workspace","scope_id":"ws-7"}`)}
	rec := &copyAuditRecorder{}
	uc := newCopyUsecase(deps, w, rec)

	raw, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID:    "src-fact-1",
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-7",
		Actor:           "42",
	})
	if err != nil {
		t.Fatalf("CopyFactRow: %v", err)
	}
	if w.calls != 1 {
		t.Fatalf("writer calls=%d", w.calls)
	}
	in := w.got
	if in.ScopeType != "workspace" || in.ScopeID != "ws-7" {
		t.Fatalf("target scope=%s/%s", in.ScopeType, in.ScopeID)
	}
	if in.Statement != "季度复盘：重点是交付节奏" {
		t.Fatalf("statement not inherited: %q", in.Statement)
	}
	if in.DetailsMarkdown != "细节：Q3 OKR" {
		t.Fatalf("details not inherited: %q", in.DetailsMarkdown)
	}
	if in.FactKind != "note" {
		t.Fatalf("kind not inherited: %q", in.FactKind)
	}
	if in.SourceKind != "copy" {
		t.Fatalf("SourceKind=%q", in.SourceKind)
	}
	if in.Confidence != 0.8 || in.Importance != 0.6 {
		t.Fatalf("confidence/importance not inherited: %v/%v", in.Confidence, in.Importance)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(in.MetadataJSON), &meta); err != nil {
		t.Fatalf("metadata not JSON: %v", err)
	}
	if meta["copied_from_fact_id"] != "src-fact-1" {
		t.Fatalf("copy lineage missing: %v", meta["copied_from_fact_id"])
	}
	if string(raw) == "" {
		t.Fatal("copy result empty")
	}

	// 审计：action/resource 钉死，detail 含血缘 + sha256 但绝不泄明文。
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries=%d", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Action != "memory.fact.copy" || e.Resource != "memory_fact" {
		t.Fatalf("audit action/resource=%q/%q", e.Action, e.Resource)
	}
	if e.ResourceID != "new-fact-9" {
		t.Fatalf("audit resource_id=%q", e.ResourceID)
	}
	if e.Actor != "42" {
		t.Fatalf("audit actor=%q", e.Actor)
	}
	if strings.Contains(e.Detail, "季度复盘") {
		t.Fatal("audit detail leaks plaintext statement")
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(e.Detail), &detail); err != nil {
		t.Fatalf("audit detail not JSON: %v", err)
	}
	if detail["source_fact_id"] != "src-fact-1" || detail["target_scope_type"] != "workspace" {
		t.Fatalf("audit detail lineage: %v", detail)
	}
	sha, _ := detail["statement_sha256"].(string)
	if len(sha) != 16 {
		t.Fatalf("statement_sha256=%q", sha)
	}
}

func TestCopyFactRow_StatementOverride(t *testing.T) {
	src := copySourceRowJSON("原文", "原细节", "note", "user", "42", false, "")
	deps := &copyFactDeps{srcRows: [][]byte{src}}
	w := &copyFactWriter{raw: []byte(`{"id":"n"}`)}
	uc := newCopyUsecase(deps, w, nil)

	if _, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID:    "src-fact-1",
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-7",
		Statement:       "  改写后的标题  ",
		FactKind:        "note",
	}); err != nil {
		t.Fatal(err)
	}
	if w.got.Statement != "改写后的标题" {
		t.Fatalf("override statement=%q", w.got.Statement)
	}
	if w.got.DetailsMarkdown != "原细节" {
		t.Fatalf("details should inherit when override empty: %q", w.got.DetailsMarkdown)
	}
}

func TestCopyFactRow_PIIFlaggedUserSourceRefused(t *testing.T) {
	src := copySourceRowJSON("隔离的敏感内容", "", "note", "user", "42", true, "")
	deps := &copyFactDeps{srcRows: [][]byte{src}}
	w := &copyFactWriter{raw: []byte(`{"id":"n"}`)}
	rec := &copyAuditRecorder{}
	uc := newCopyUsecase(deps, w, rec)

	_, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID:    "src-fact-1",
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-7",
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("want Forbidden, got %v", err)
	}
	if w.calls != 0 {
		t.Fatal("writer must not run on refused copy")
	}
	if len(rec.entries) != 0 {
		t.Fatal("refused copy must not write audit")
	}
}

// TestCopyFactRow_PIIFlagNonUserScopeAllowed：PII 标在非 user scope 只是审计
// 提示（factPIIGateClause 设计），不阻断复制。
func TestCopyFactRow_PIIFlagNonUserScopeAllowed(t *testing.T) {
	src := copySourceRowJSON("团队敏感提示", "", "note", "workspace", "ws-1", true, "")
	deps := &copyFactDeps{srcRows: [][]byte{src}}
	w := &copyFactWriter{raw: []byte(`{"id":"n"}`)}
	uc := newCopyUsecase(deps, w, nil)

	if _, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID:    "src-fact-1",
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-7",
	}); err != nil {
		t.Fatalf("non-user PII flag must not block copy: %v", err)
	}
}

func TestCopyFactRow_NotFoundAndBadRequest(t *testing.T) {
	uc := newCopyUsecase(&copyFactDeps{srcRows: nil}, &copyFactWriter{}, nil)
	_, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID:    "missing",
		TargetScopeType: "workspace",
	})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}

	src := copySourceRowJSON("x", "", "note", "user", "42", false, "")
	uc = newCopyUsecase(&copyFactDeps{srcRows: [][]byte{src}}, &copyFactWriter{}, nil)
	_, err = uc.CopyFactRow(context.Background(), FactCopy{SourceFactID: "src-fact-1"})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("want BadRequest, got %v", err)
	}
}

// TestCopyFactRow_NilAuditAndAuditFailure：审计器未注入或落库失败都不阻断
// 复制主流程（best-effort，与 client_tool 审计同契约）。
func TestCopyFactRow_NilAuditAndAuditFailure(t *testing.T) {
	src := copySourceRowJSON("正常复制", "", "note", "user", "42", false, "")

	// nil recorder
	uc := newCopyUsecase(&copyFactDeps{srcRows: [][]byte{src}}, &copyFactWriter{raw: []byte(`{"id":"n"}`)}, nil)
	if _, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID: "src-fact-1", TargetScopeType: "workspace", TargetScopeID: "ws-7",
	}); err != nil {
		t.Fatalf("nil recorder must not fail copy: %v", err)
	}

	// failing recorder
	rec := &copyAuditRecorder{err: context.DeadlineExceeded}
	uc = newCopyUsecase(&copyFactDeps{srcRows: [][]byte{src}}, &copyFactWriter{raw: []byte(`{"id":"n"}`)}, rec)
	if _, err := uc.CopyFactRow(context.Background(), FactCopy{
		SourceFactID: "src-fact-1", TargetScopeType: "workspace", TargetScopeID: "ws-7",
	}); err != nil {
		t.Fatalf("audit failure must degrade, got %v", err)
	}
}

func TestPeekFactScope(t *testing.T) {
	src := copySourceRowJSON("x", "", "note", "Workspace", " ws-9 ", true, "")
	uc := newCopyUsecase(&copyFactDeps{srcRows: [][]byte{src}}, &copyFactWriter{}, nil)
	st, sid, pii, err := uc.PeekFactScope(context.Background(), " src-fact-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if st != "workspace" || sid != "ws-9" || !pii {
		t.Fatalf("peek=%q/%q pii=%v", st, sid, pii)
	}

	uc = newCopyUsecase(&copyFactDeps{srcRows: nil}, &copyFactWriter{}, nil)
	if _, _, _, err := uc.PeekFactScope(context.Background(), "missing"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}
}

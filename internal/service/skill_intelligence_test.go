package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/skill_intelligence/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Stub repo for SkillIntelligenceUsecase ──────────────────────────────────

type stubExperienceReportReader struct {
	reports []biz.ExperienceReport
	byID    map[string]*biz.ExperienceReport
}

func newStubExperienceReportReader() *stubExperienceReportReader {
	return &stubExperienceReportReader{
		byID: make(map[string]*biz.ExperienceReport),
	}
}

func (s *stubExperienceReportReader) ListBySkill(_ context.Context, skillID string, limit, offset int) ([]biz.ExperienceReport, error) {
	var filtered []biz.ExperienceReport
	for _, r := range s.reports {
		if r.SkillID == skillID {
			filtered = append(filtered, r)
		}
	}
	if offset >= len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

func (s *stubExperienceReportReader) GetByID(_ context.Context, id string) (*biz.ExperienceReport, error) {
	r, ok := s.byID[id]
	if !ok {
		return nil, apierror.NotFound("SKILL_INTELLIGENCE", "experience report not found")
	}
	return r, nil
}

func (s *stubExperienceReportReader) ListByTimeRange(_ context.Context, _, _ time.Time, _, _ int) ([]biz.ExperienceReport, error) {
	return nil, nil
}

func (s *stubExperienceReportReader) ListFiltered(_ context.Context, skillID string, _, _ *time.Time, _, _ int) ([]biz.ExperienceReport, int, error) {
	if skillID == "" {
		return s.reports, len(s.reports), nil
	}
	var filtered []biz.ExperienceReport
	for _, r := range s.reports {
		if r.SkillID == skillID {
			filtered = append(filtered, r)
		}
	}
	return filtered, len(filtered), nil
}

// newTestSkillIntelligenceService creates a SkillIntelligenceService with a
// real Usecase backed by stub repos.
func newTestSkillIntelligenceService(reader *stubExperienceReportReader) *SkillIntelligenceService {
	uc := biz.NewSkillIntelligenceUsecase(reader, nil, nil, nil, loggateway.NewNoop())
	return NewSkillIntelligenceService(uc, loggateway.NewNoop())
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestSkillIntelligenceService_ListExperienceReports(t *testing.T) {
	reader := newStubExperienceReportReader()
	now := time.Now().UTC()
	reader.reports = []biz.ExperienceReport{
		{ID: "r1", SkillID: "sk1", IsSuccess: true, Score: 90, CreatedAt: now},
		{ID: "r2", SkillID: "sk1", IsSuccess: false, Score: 30, FailureTags: []string{"tool_timeout"}, CreatedAt: now},
		{ID: "r3", SkillID: "sk2", IsSuccess: true, Score: 80, CreatedAt: now},
	}
	reader.byID["r1"] = &reader.reports[0]
	reader.byID["r2"] = &reader.reports[1]
	reader.byID["r3"] = &reader.reports[2]

	svc := newTestSkillIntelligenceService(reader)
	resp, err := svc.ListExperienceReports(ctxBG(), &v1.ListExperienceReportsRequest{
		SkillId:  "sk1",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListExperienceReports: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].Id != "r1" {
		t.Errorf("expected first item id=r1, got %s", resp.Items[0].Id)
	}
	if resp.Items[1].Id != "r2" {
		t.Errorf("expected second item id=r2, got %s", resp.Items[1].Id)
	}
	if resp.Page != 1 || resp.PageSize != 10 {
		t.Errorf("expected page=1 pageSize=10, got page=%d pageSize=%d", resp.Page, resp.PageSize)
	}
}

func TestSkillIntelligenceService_GetExperienceReport(t *testing.T) {
	reader := newStubExperienceReportReader()
	now := time.Now().UTC()
	report := &biz.ExperienceReport{
		ID:          "r1",
		TenantID:    "t1",
		SessionID:   "s1",
		InvocationID: "inv1",
		SkillID:     "sk1",
		IsSuccess:   true,
		Score:       85,
		FlowSummary: "Skill completed successfully.",
		CreatedAt:   now,
	}
	reader.byID["r1"] = report

	svc := newTestSkillIntelligenceService(reader)
	resp, err := svc.GetExperienceReport(ctxBG(), &v1.GetExperienceReportRequest{Id: "r1"})
	if err != nil {
		t.Fatalf("GetExperienceReport: %v", err)
	}
	if resp.Report.Id != "r1" {
		t.Errorf("expected id=r1, got %s", resp.Report.Id)
	}
	if resp.Report.SkillId != "sk1" {
		t.Errorf("expected skill_id=sk1, got %s", resp.Report.SkillId)
	}
	if resp.Report.IsSuccess != true {
		t.Error("expected is_success=true")
	}
	if resp.Report.Score != 85 {
		t.Errorf("expected score=85, got %d", resp.Report.Score)
	}
	if resp.Report.FlowSummary != "Skill completed successfully." {
		t.Errorf("unexpected flow_summary: %s", resp.Report.FlowSummary)
	}
}

func TestSkillIntelligenceService_GetExperienceReport_NotFound(t *testing.T) {
	reader := newStubExperienceReportReader()
	svc := newTestSkillIntelligenceService(reader)

	_, err := svc.GetExperienceReport(ctxBG(), &v1.GetExperienceReportRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent report, got nil")
	}
}

func TestToProtoExperienceReport(t *testing.T) {
	now := time.Now().UTC()
	suggestionID := "sug-1"
	snapshot, _ := json.Marshal(map[string]interface{}{"reason": "user_request", "confidence": 0.9})

	r := biz.ExperienceReport{
		ID:                    "r1",
		TenantID:              "t1",
		SessionID:             "s1",
		InvocationID:          "inv1",
		SkillID:               "sk1",
		IsSuccess:             false,
		Score:                 42,
		FailureTags:           []string{"tool_timeout", "tool_api_error"},
		FlowSummary:           "Skill failed due to timeout.",
		OptimizationAdvice:    "Add retry logic.",
		SelectionSnapshot:     snapshot,
		GeneratedSuggestionID: &suggestionID,
		CreatedAt:             now,
	}

	pb := toProtoExperienceReport(r)

	// Scalar fields
	if pb.Id != "r1" {
		t.Errorf("expected id=r1, got %s", pb.Id)
	}
	if pb.TenantId != "t1" {
		t.Errorf("expected tenant_id=t1, got %s", pb.TenantId)
	}
	if pb.SessionId != "s1" {
		t.Errorf("expected session_id=s1, got %s", pb.SessionId)
	}
	if pb.InvocationId != "inv1" {
		t.Errorf("expected invocation_id=inv1, got %s", pb.InvocationId)
	}
	if pb.SkillId != "sk1" {
		t.Errorf("expected skill_id=sk1, got %s", pb.SkillId)
	}
	if pb.IsSuccess != false {
		t.Error("expected is_success=false")
	}
	if pb.Score != 42 {
		t.Errorf("expected score=42, got %d", pb.Score)
	}
	if pb.FlowSummary != "Skill failed due to timeout." {
		t.Errorf("unexpected flow_summary: %s", pb.FlowSummary)
	}
	if pb.OptimizationAdvice != "Add retry logic." {
		t.Errorf("unexpected optimization_advice: %s", pb.OptimizationAdvice)
	}
	if pb.GeneratedSuggestionId != "sug-1" {
		t.Errorf("expected generated_suggestion_id=sug-1, got %s", pb.GeneratedSuggestionId)
	}

	// Failure tags
	if len(pb.FailureTags) != 2 {
		t.Fatalf("expected 2 failure tags, got %d", len(pb.FailureTags))
	}
	if pb.FailureTags[0] != "tool_timeout" || pb.FailureTags[1] != "tool_api_error" {
		t.Errorf("unexpected failure_tags: %v", pb.FailureTags)
	}

	// Selection snapshot
	if pb.SelectionSnapshot == nil {
		t.Error("expected selection_snapshot to be set")
	} else {
		reason, _ := pb.SelectionSnapshot.Fields["reason"]
		if reason == nil || reason.GetStringValue() != "user_request" {
			t.Error("expected selection_snapshot.reason=user_request")
		}
	}

	// CreatedAt timestamp
	if pb.CreatedAt == nil {
		t.Error("expected created_at to be set")
	} else if !pb.CreatedAt.AsTime().Equal(now) {
		t.Errorf("created_at mismatch: got %v, want %v", pb.CreatedAt.AsTime(), now)
	}
}

func TestToProtoExperienceReport_NilOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	r := biz.ExperienceReport{
		ID:        "r2",
		SkillID:   "sk2",
		IsSuccess: true,
		Score:     100,
		CreatedAt: now,
	}

	pb := toProtoExperienceReport(r)

	if pb.SelectionSnapshot != nil {
		t.Error("expected nil selection_snapshot when source is nil")
	}
	if pb.GeneratedSuggestionId != "" {
		t.Errorf("expected empty generated_suggestion_id when source is nil, got %s", pb.GeneratedSuggestionId)
	}
	if len(pb.FailureTags) != 0 {
		t.Errorf("expected empty failure_tags, got %v", pb.FailureTags)
	}
}

func TestToProtoExperienceReport_InvalidSelectionSnapshot(t *testing.T) {
	now := time.Now().UTC()
	r := biz.ExperienceReport{
		ID:                "r3",
		SkillID:           "sk3",
		IsSuccess:         true,
		Score:             75,
		SelectionSnapshot: json.RawMessage(`{invalid json`),
		CreatedAt:         now,
	}

	pb := toProtoExperienceReport(r)

	// Invalid JSON should be silently ignored, not cause a panic.
	if pb.SelectionSnapshot != nil {
		t.Error("expected nil selection_snapshot for invalid JSON")
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func ctxBG() context.Context {
	return context.Background()
}

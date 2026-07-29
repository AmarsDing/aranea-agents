package importer

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func warnCandidateFixture(id string) candidateState {
	return candidateState{
		public: biz.SkillImportCandidate{
			CandidateID:      id,
			Name:             "Standup Summary Writer",
			Slug:             "standup-summary-writer",
			Description:      "standup summaries",
			ValidationStatus: "warn",
			StatusIcon:       "merge_suggested",
		},
		body:  "---\nname: Standup Summary Writer\n---\nbody",
		files: map[string][]byte{"SKILL.md": []byte("---\nname: Standup Summary Writer\n---\nbody")},
	}
}

// P-r4-keep-separate: warn candidates from groups the LLM judged keep_separate
// must be importable via a keep_separate decision, otherwise a genuinely
// different skill can never be installed.
func TestResolveDecision_keepSeparate_WarnCandidate(t *testing.T) {
	eng, _ := setupEngineWithTempRoot(t, &stubSkillRepo{})
	job := &jobState{
		public: biz.SkillImportJob{JobID: "job-ks-1", Status: "completed"},
		candidates: map[string]candidateState{
			"c1": warnCandidateFixture("c1"),
		},
	}

	params, skipped, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action:      "keep_separate",
		CandidateID: "c1",
	})
	if err != nil {
		t.Fatalf("expected success for keep_separate on warn candidate, got: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("expected no skipped candidates, got: %v", skipped)
	}
	if params == nil {
		t.Fatal("expected non-nil params for keep_separate decision")
	}
	if params.name != "Standup Summary Writer" || params.slug != "standup-summary-writer" {
		t.Errorf("unexpected params: name=%q slug=%q", params.name, params.slug)
	}
	if params.updateSkillID != "" || len(params.retireSkillIDs) != 0 {
		t.Errorf("keep_separate must not touch existing skills: update=%q retire=%v", params.updateSkillID, params.retireSkillIDs)
	}
}

// keep_separate rejects pass candidates (import_passed is their path) and
// unknown candidate IDs.
func TestResolveDecision_keepSeparate_RejectsNonWarn(t *testing.T) {
	eng, _ := setupEngineWithTempRoot(t, &stubSkillRepo{})
	passState := warnCandidateFixture("c-pass")
	passState.public.ValidationStatus = "pass"
	job := &jobState{
		public: biz.SkillImportJob{JobID: "job-ks-2", Status: "completed"},
		candidates: map[string]candidateState{
			"c-pass": passState,
		},
	}

	if _, _, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action: "keep_separate", CandidateID: "c-pass",
	}); err == nil {
		t.Error("expected error for keep_separate on pass candidate, got nil")
	}
	if _, _, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action: "keep_separate", CandidateID: "c-missing",
	}); err == nil {
		t.Error("expected error for keep_separate on missing candidate, got nil")
	}
}

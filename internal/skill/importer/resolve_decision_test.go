package importer

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// TestResolveDecision_mergeGroupWithAI_MissingGroupID verifies that
// merge_group_with_ai rejects decisions without a GroupID.
func TestResolveDecision_mergeGroupWithAI_MissingGroupID(t *testing.T) {
	eng, _ := setupEngineWithTempRoot(t, &stubSkillRepo{})
	job := &jobState{
		public: biz.SkillImportJob{
			JobID:  "job-merge-1",
			Status: "completed",
			ConflictGroups: []biz.SkillConflictGroup{
				{GroupID: "grp-1", CandidateIDs: []string{"c1"}},
			},
		},
		candidates: map[string]candidateState{},
	}

	_, _, err := eng.resolveDecision(job, biz.SkillImportDecision{
		Action:      "merge_group_with_ai",
		MergedName:  "Merged Skill",
		MergedBody:  "---\nname: Merged Skill\n---\nbody",
	})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
	if !strings.Contains(err.Error(), "group_id is required") {
		t.Errorf("expected group_id required error, got: %v", err)
	}
}

// TestResolveDecision_mergeGroupWithAI_GroupNotFound verifies that
// merge_group_with_ai rejects decisions referencing a non-existent conflict group.
func TestResolveDecision_mergeGroupWithAI_GroupNotFound(t *testing.T) {
	eng, _ := setupEngineWithTempRoot(t, &stubSkillRepo{})
	job := &jobState{
		public: biz.SkillImportJob{
			JobID:  "job-merge-2",
			Status: "completed",
			ConflictGroups: []biz.SkillConflictGroup{
				{GroupID: "grp-real", CandidateIDs: []string{"c1"}},
			},
		},
		candidates: map[string]candidateState{},
	}

	_, _, err := eng.resolveDecision(job, biz.SkillImportDecision{
		Action:      "merge_group_with_ai",
		GroupID:     "grp-nonexistent",
		MergedName:  "Merged Skill",
		MergedBody:  "---\nname: Merged Skill\n---\nbody",
	})
	if err == nil {
		t.Fatal("expected error for non-existent group_id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestResolveDecision_mergeGroupWithAI_ValidGroup verifies that
// merge_group_with_ai succeeds when GroupID references an existing conflict group.
func TestResolveDecision_mergeGroupWithAI_ValidGroup(t *testing.T) {
	eng, _ := setupEngineWithTempRoot(t, &stubSkillRepo{})
	job := &jobState{
		public: biz.SkillImportJob{
			JobID:  "job-merge-3",
			Status: "completed",
			ConflictGroups: []biz.SkillConflictGroup{
				{GroupID: "grp-valid", CandidateIDs: []string{"c1"}},
			},
		},
		candidates: map[string]candidateState{},
	}

	params, _, err := eng.resolveDecision(job, biz.SkillImportDecision{
		Action:             "merge_group_with_ai",
		GroupID:            "grp-valid",
		MergedName:         "Merged Skill",
		MergedBody:         "---\nname: Merged Skill\n---\nbody",
		MergedDescription:  "A merged skill",
	})
	if err != nil {
		t.Fatalf("expected success for valid group_id, got: %v", err)
	}
	if params == nil {
		t.Fatal("expected non-nil params for valid merge decision")
	}
	if params.name != "Merged Skill" {
		t.Errorf("expected name Merged Skill, got %s", params.name)
	}
}

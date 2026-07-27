package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
)

// seedDuplicateJob injects a completed job whose candidate carries a
// duplicate_name block (the state produced by Import when the slug/name
// collides with an existing skill).
func seedDuplicateJob(eng *Engine, jobID string) {
	candidate := biz.SkillImportCandidate{
		CandidateID:      "c-dup",
		Name:             "Dup Skill",
		Slug:             "dup-skill",
		ValidationStatus: "block",
		Blocks:           []biz.SkillImportIssue{{Type: "duplicate_name", Message: "slug already exists"}},
	}
	eng.jobsMu.Lock()
	eng.jobs[jobID] = &jobState{
		public: biz.SkillImportJob{JobID: jobID, Status: "completed"},
		candidates: map[string]candidateState{
			candidate.CandidateID: {
				public: candidate,
				body:   "# Dup Skill",
				files:  map[string][]byte{"SKILL.md": []byte("# Dup Skill")},
			},
		},
	}
	eng.jobsMu.Unlock()
}

// F4: skip_duplicate drops a duplicate-blocked candidate without installing.
func TestApplyImport_skipDuplicate(t *testing.T) {
	repo := &stubSkillRepo{}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedDuplicateJob(eng, "job-skip-dup")

	result, err := eng.ApplyImport(context.Background(), "job-skip-dup", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{{Action: "skip_duplicate", CandidateID: "c-dup"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SkippedCandidateIDs) != 1 || result.SkippedCandidateIDs[0] != "c-dup" {
		t.Errorf("expected c-dup skipped, got %v", result.SkippedCandidateIDs)
	}
	if len(result.CreatedSkillIDs) != 0 {
		t.Errorf("expected 0 created skills, got %v", result.CreatedSkillIDs)
	}
	if repo.createCalls != 0 {
		t.Errorf("expected no CreateSkillWithVersion calls, got %d", repo.createCalls)
	}
	if len(repo.appendedInputs) != 0 {
		t.Errorf("expected no AppendImportedVersion calls, got %d", len(repo.appendedInputs))
	}
}

// F4: skip_duplicate on a candidate that is not duplicate-blocked is rejected.
func TestApplyImport_skipDuplicate_notDuplicate(t *testing.T) {
	repo := &stubSkillRepo{}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedJob(eng, "job-skip-nondup", biz.SkillImportCandidate{
		CandidateID: "c1", Name: "Fresh", Slug: "fresh", ValidationStatus: "pass",
	})

	_, err := eng.ApplyImport(context.Background(), "job-skip-nondup", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{{Action: "skip_duplicate", CandidateID: "c1"}},
	})
	if err == nil {
		t.Fatal("expected error for skip_duplicate on non-duplicate candidate")
	}
	if !errors.Is(err, ErrCandidateNotDuplicate) {
		t.Errorf("expected ErrCandidateNotDuplicate, got %v", err)
	}
}

// F4: overwrite_duplicate resolves to the existing same-slug skill and its
// on-disk storage directory.
func TestResolveDecision_overwriteDuplicate(t *testing.T) {
	storageDir := t.TempDir()
	repo := &stubSkillRepo{
		byKeySkill:  skill.Skill{ID: "existing-1", Name: "Dup Skill", Slug: "dup-skill"},
		storageDirs: map[string]string{"existing-1": storageDir},
	}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedDuplicateJob(eng, "job-ow-resolve")
	eng.jobsMu.RLock()
	job := eng.jobs["job-ow-resolve"]
	eng.jobsMu.RUnlock()

	params, skipIDs, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action: "overwrite_duplicate", CandidateID: "c-dup",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipIDs) != 0 {
		t.Fatalf("expected no skips, got %v", skipIDs)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	if params.updateSkillID != "existing-1" {
		t.Errorf("expected updateSkillID existing-1, got %q", params.updateSkillID)
	}
	if params.targetDir != storageDir {
		t.Errorf("expected targetDir %q, got %q", storageDir, params.targetDir)
	}
}

// F4: overwrite_duplicate on a candidate that is not duplicate-blocked is rejected.
func TestResolveDecision_overwriteDuplicate_notDuplicate(t *testing.T) {
	repo := &stubSkillRepo{}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedJob(eng, "job-ow-nondup", biz.SkillImportCandidate{
		CandidateID: "c1", Name: "Fresh", Slug: "fresh", ValidationStatus: "pass",
	})
	eng.jobsMu.RLock()
	job := eng.jobs["job-ow-nondup"]
	eng.jobsMu.RUnlock()

	_, _, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action: "overwrite_duplicate", CandidateID: "c1",
	})
	if err == nil {
		t.Fatal("expected error for overwrite_duplicate on non-duplicate candidate")
	}
	if !errors.Is(err, ErrCandidateNotDuplicate) {
		t.Errorf("expected ErrCandidateNotDuplicate, got %v", err)
	}
}

// F4: overwrite_duplicate apply appends a new version to the existing skill
// and writes the package files into its storage directory — no new skill row,
// no compensation delete.
func TestApplyImport_overwriteDuplicate_AppendsVersion(t *testing.T) {
	storageDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(storageDir, "SKILL.md"), []byte("old body"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &stubSkillRepo{
		byKeySkill:  skill.Skill{ID: "existing-1", Name: "Dup Skill", Slug: "dup-skill"},
		storageDirs: map[string]string{"existing-1": storageDir},
	}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedDuplicateJob(eng, "job-ow-apply")

	result, err := eng.ApplyImport(context.Background(), "job-ow-apply", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{{Action: "overwrite_duplicate", CandidateID: "c-dup"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CreatedSkillIDs) != 1 || result.CreatedSkillIDs[0] != "existing-1" {
		t.Errorf("expected existing-1 in created IDs, got %v", result.CreatedSkillIDs)
	}
	if len(result.SkippedCandidateIDs) != 0 {
		t.Errorf("expected no skips, got %v", result.SkippedCandidateIDs)
	}
	if repo.createCalls != 0 {
		t.Errorf("expected no CreateSkillWithVersion calls, got %d", repo.createCalls)
	}
	if len(repo.appendedInputs) != 1 {
		t.Fatalf("expected 1 AppendImportedVersion call, got %d", len(repo.appendedInputs))
	}
	in := repo.appendedInputs[0]
	if in.SkillID != "existing-1" {
		t.Errorf("expected append to existing-1, got %q", in.SkillID)
	}
	if in.Body != "# Dup Skill" {
		t.Errorf("expected appended body '# Dup Skill', got %q", in.Body)
	}
	// The overwrite path is additive — the pre-existing skill must never be
	// compensation-deleted.
	if len(repo.deletedIDs) != 0 {
		t.Errorf("expected no DeleteSkill calls, got %v", repo.deletedIDs)
	}
	// Package files must land in the existing skill's storage dir.
	data, err := os.ReadFile(filepath.Join(storageDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Dup Skill" {
		t.Errorf("expected storage dir SKILL.md overwritten, got %q", string(data))
	}
}

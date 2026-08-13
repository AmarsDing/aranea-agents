package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
)

// stubSkillRepo is a minimal biz.SkillRepo that allows injecting per-call behaviour.
type stubSkillRepo struct {
	skill.Repo // embed to satisfy the interface with zero-value panics for unexercised methods

	// createCalls tracks how many times CreateSkillWithVersion has been called.
	createCalls int
	// failOnCreate causes CreateSkillWithVersion to fail when createCalls reaches this value (1-based).
	failOnCreate int

	// deletedIDs records every DeleteSkill call.
	deletedIDs []string

	// byKeySkill/byKeyErr back GetSkillBySkillKey (overwrite_duplicate target lookup).
	byKeySkill skill.Skill
	byKeyErr   error
	// storageDirs maps skill ID → on-disk storage dir (GetSkillStorageDir).
	storageDirs map[string]string
	// appendedInputs records every AppendImportedVersion call (overwrite path).
	appendedInputs []skill.ImportVersionInput
	// failOnAppend causes AppendImportedVersion to fail (overwrite DB-failure injection).
	failOnAppend bool
	// archivedIDs records every ArchiveSkill call (merge retire path).
	archivedIDs []string
	// derivedFrom records SetSkillDerivedFrom calls: skill ID → source IDs.
	derivedFrom map[string][]string
}

func (r *stubSkillRepo) CreateSkillWithVersion(_ context.Context, in skill.CreateInput) (skill.Skill, error) {
	r.createCalls++
	if r.failOnCreate > 0 && r.createCalls == r.failOnCreate {
		return skill.Skill{}, errors.New("simulated db error")
	}
	return skill.Skill{ID: fmt.Sprintf("skill-%d", r.createCalls), Slug: in.Slug}, nil
}

func (r *stubSkillRepo) DeleteSkill(_ context.Context, id string) error {
	r.deletedIDs = append(r.deletedIDs, id)
	return nil
}

func (r *stubSkillRepo) ListSkillSimilaritySources(_ context.Context) ([]skill.SimilaritySource, error) {
	return nil, nil
}

func (r *stubSkillRepo) GetSkillBySkillKey(_ context.Context, skillKey string) (skill.Skill, error) {
	if r.byKeyErr != nil {
		return skill.Skill{}, r.byKeyErr
	}
	return r.byKeySkill, nil
}

func (r *stubSkillRepo) GetSkillStorageDir(_ context.Context, id string) (string, error) {
	if dir, ok := r.storageDirs[id]; ok {
		return dir, nil
	}
	return "", errors.New("storage dir not configured for " + id)
}

func (r *stubSkillRepo) AppendImportedVersion(_ context.Context, in skill.ImportVersionInput) (skill.Skill, error) {
	r.appendedInputs = append(r.appendedInputs, in)
	if r.failOnAppend {
		return skill.Skill{}, errors.New("simulated append failure")
	}
	return skill.Skill{ID: in.SkillID}, nil
}

func (r *stubSkillRepo) ArchiveSkill(_ context.Context, id string) error {
	r.archivedIDs = append(r.archivedIDs, id)
	return nil
}

func (r *stubSkillRepo) SetSkillDerivedFrom(_ context.Context, id string, sourceIDs []string) error {
	if r.derivedFrom == nil {
		r.derivedFrom = map[string][]string{}
	}
	r.derivedFrom[id] = append([]string(nil), sourceIDs...)
	return nil
}

// setupEngineWithTempRoot creates an Engine whose file-system root is a temp dir that is
// cleaned up after the test. Pass sys=nil so resolveRoot falls back to $SKILL_STORAGE_ROOT.
func setupEngineWithTempRoot(t *testing.T, repo *stubSkillRepo) (*Engine, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("SKILL_STORAGE_ROOT", tmp)
	eng := &Engine{
		repo: repo,
		jobs: make(map[string]*jobState),
	}
	return eng, tmp
}

// seedJob directly injects a pre-baked "completed" job into the engine for testing.
func seedJob(eng *Engine, jobID string, candidates ...biz.SkillImportCandidate) {
	states := map[string]candidateState{}
	for _, c := range candidates {
		states[c.CandidateID] = candidateState{
			public: c,
			body:   "# " + c.Name,
			files:  map[string][]byte{"SKILL.md": []byte("# " + c.Name)},
		}
	}
	eng.jobsMu.Lock()
	eng.jobs[jobID] = &jobState{
		public: biz.SkillImportJob{
			JobID:  jobID,
			Status: "completed",
		},
		candidates: states,
	}
	eng.jobsMu.Unlock()
}

// TestApplyImport_compensatesOnPartialFailure verifies that when the Nth skill write fails:
//   - all previously committed skills are rolled back (DeleteSkill called + disk dir removed)
//   - skip decisions processed before the failure are preserved in the returned partial result
func TestApplyImport_compensatesOnPartialFailure(t *testing.T) {
	repo := &stubSkillRepo{failOnCreate: 2} // second write fails
	eng, root := setupEngineWithTempRoot(t, repo)

	cands := []biz.SkillImportCandidate{
		{CandidateID: "c1", Name: "Skill One", Slug: "skill-one", ValidationStatus: "pass"},
		{CandidateID: "c2", Name: "Skill Two", Slug: "skill-two", ValidationStatus: "pass"},
		{CandidateID: "c3", Name: "Risky", Slug: "risky", ValidationStatus: "block"},
	}
	seedJob(eng, "job-1", cands...)

	req := biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{
			{Action: "reject_risky_upload", CandidateID: "c3"}, // skip processed before failure
			{Action: "import_passed", CandidateID: "c1"},
			{Action: "import_passed", CandidateID: "c2"}, // this one fails
		},
	}
	partial, err := eng.ApplyImport(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error from second createImportedSkill, got nil")
	}

	// The first skill's DB row must have been compensated.
	if len(repo.deletedIDs) == 0 {
		t.Fatal("expected DeleteSkill to be called for first skill, got 0 deletes")
	}
	if repo.deletedIDs[0] != "skill-1" {
		t.Errorf("expected DeleteSkill(\"skill-1\"), got %q", repo.deletedIDs[0])
	}

	// The first skill's disk directory must have been removed.
	firstDir := filepath.Join(root, "skill-one")
	if _, statErr := os.Stat(firstDir); !os.IsNotExist(statErr) {
		t.Errorf("expected disk dir %s to be removed after compensation, but it still exists", firstDir)
	}

	// The failed skill's disk directory must also have been cleaned up
	// (createImportedSkill cleans up on DB failure to prevent orphan resources).
	secondDir := filepath.Join(root, "skill-two")
	if _, statErr := os.Stat(secondDir); !os.IsNotExist(statErr) {
		t.Errorf("expected failed skill dir %s to be cleaned up, but it still exists", secondDir)
	}

	// Skip decisions processed before the failure must be preserved in the partial result.
	if len(partial.SkippedCandidateIDs) != 1 || partial.SkippedCandidateIDs[0] != "c3" {
		t.Errorf("expected partial result to preserve skipped candidate c3, got %v", partial.SkippedCandidateIDs)
	}

	// Created IDs must be empty — all have been compensated.
	if len(partial.CreatedSkillIDs) != 0 {
		t.Errorf("expected 0 created IDs in partial result (compensated), got %v", partial.CreatedSkillIDs)
	}
}

// TestApplyImport_happyPath verifies that a successful multi-skill import returns all IDs.
func TestApplyImport_happyPath(t *testing.T) {
	repo := &stubSkillRepo{}
	eng, _ := setupEngineWithTempRoot(t, repo)

	cands := []biz.SkillImportCandidate{
		{CandidateID: "c1", Name: "Alpha", Slug: "alpha", ValidationStatus: "pass"},
		{CandidateID: "c2", Name: "Beta", Slug: "beta", ValidationStatus: "pass"},
	}
	seedJob(eng, "job-2", cands...)

	req := biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{
			{Action: "import_passed", CandidateID: "c1"},
			{Action: "import_passed", CandidateID: "c2"},
		},
	}
	result, err := eng.ApplyImport(context.Background(), "job-2", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CreatedSkillIDs) != 2 {
		t.Errorf("expected 2 created skills, got %d", len(result.CreatedSkillIDs))
	}
	if len(repo.deletedIDs) != 0 {
		t.Errorf("expected no compensating deletes on success, got %v", repo.deletedIDs)
	}
}

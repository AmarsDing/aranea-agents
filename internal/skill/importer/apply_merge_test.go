package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz"
)

// writeExistingSkillDir materialises an existing skill's on-disk directory
// (SKILL.md + aux files) and returns the dir.
func writeExistingSkillDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// seedMergeJob injects a completed job with one conflict group linking
// candidate c-merge to the given existing skills.
func seedMergeJob(eng *Engine, jobID, groupID string, existing []biz.SkillSimilaritySource) {
	candidate := biz.SkillImportCandidate{
		CandidateID:      "c-merge",
		Name:             "Cand",
		Slug:             "cand",
		ValidationStatus: "warn",
	}
	eng.jobsMu.Lock()
	eng.jobs[jobID] = &jobState{
		public: biz.SkillImportJob{
			JobID:  jobID,
			Status: "completed",
			ConflictGroups: []biz.SkillConflictGroup{{
				GroupID:        groupID,
				CandidateIDs:   []string{candidate.CandidateID},
				ExistingSkills: existing,
			}},
		},
		candidates: map[string]candidateState{
			candidate.CandidateID: {
				public: candidate,
				body:   "# Cand",
				files: map[string][]byte{
					"SKILL.md":    []byte("# Cand"),
					"new.py":      []byte("print('new')"),
					"shared.json": []byte(`{"from": "candidate"}`),
				},
			},
		},
	}
	eng.jobsMu.Unlock()
}

// F7: merge_group_with_ai must produce the union of auxiliary files —
// existing skills' on-disk aux files plus the candidate's aux files, with the
// candidate winning path conflicts; SKILL.md from any source is replaced by
// the merged body.
func TestResolveDecision_mergeGroupWithAI_FileUnion(t *testing.T) {
	existingDir := writeExistingSkillDir(t, map[string]string{
		"SKILL.md":    "old body — must not survive the merge",
		"extra.txt":   "existing aux",
		"shared.json": `{"from": "existing"}`,
	})
	repo := &stubSkillRepo{
		storageDirs: map[string]string{"exist-1": existingDir},
	}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedMergeJob(eng, "job-merge-union", "g1", []biz.SkillSimilaritySource{
		{ID: "exist-1", Name: "Old", Slug: "old-skill"},
	})
	eng.jobsMu.RLock()
	job := eng.jobs["job-merge-union"]
	eng.jobsMu.RUnlock()

	params, _, err := eng.resolveDecision(context.Background(), job, biz.SkillImportDecision{
		Action:     "merge_group_with_ai",
		GroupID:    "g1",
		MergedName: "Merged Skill",
		MergedBody: "---\nname: Merged Skill\ndescription: d\n---\nmerged body",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	// Merged body always becomes SKILL.md.
	if string(params.files["SKILL.md"]) != "---\nname: Merged Skill\ndescription: d\n---\nmerged body" {
		t.Errorf("expected merged body as SKILL.md, got %q", string(params.files["SKILL.md"]))
	}
	// Existing aux file is carried over.
	if string(params.files["extra.txt"]) != "existing aux" {
		t.Errorf("expected extra.txt from existing skill, got %q", string(params.files["extra.txt"]))
	}
	// Candidate aux file is carried over.
	if string(params.files["new.py"]) != "print('new')" {
		t.Errorf("expected new.py from candidate, got %q", string(params.files["new.py"]))
	}
	// Path conflict: candidate wins over existing.
	if string(params.files["shared.json"]) != `{"from": "candidate"}` {
		t.Errorf("expected candidate shared.json to win, got %q", string(params.files["shared.json"]))
	}
	// Provenance: the existing skill is recorded as a merge source.
	if len(params.derivedFromSkillIDs) != 1 || params.derivedFromSkillIDs[0] != "exist-1" {
		t.Errorf("expected derivedFromSkillIDs [exist-1], got %v", params.derivedFromSkillIDs)
	}
	// No retire requested → no retire list.
	if len(params.retireSkillIDs) != 0 {
		t.Errorf("expected no retireSkillIDs, got %v", params.retireSkillIDs)
	}
}

// F7: merge apply with retire_sources=true archives every existing skill in
// the conflict group and records provenance on the merged skill.
func TestApplyImport_mergeGroupWithAI_RetireSources(t *testing.T) {
	existingDir := writeExistingSkillDir(t, map[string]string{
		"SKILL.md":  "old body",
		"extra.txt": "existing aux",
	})
	repo := &stubSkillRepo{
		storageDirs: map[string]string{"exist-1": existingDir},
	}
	eng, root := setupEngineWithTempRoot(t, repo)
	seedMergeJob(eng, "job-merge-retire", "g1", []biz.SkillSimilaritySource{
		{ID: "exist-1", Name: "Old", Slug: "old-skill"},
	})

	result, err := eng.ApplyImport(context.Background(), "job-merge-retire", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{{
			Action:        "merge_group_with_ai",
			GroupID:       "g1",
			MergedName:    "Merged Skill",
			MergedBody:    "---\nname: Merged Skill\ndescription: d\n---\nmerged body",
			RetireSources: true,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CreatedSkillIDs) != 1 {
		t.Fatalf("expected 1 created skill, got %v", result.CreatedSkillIDs)
	}
	createdID := result.CreatedSkillIDs[0]
	if len(repo.archivedIDs) != 1 || repo.archivedIDs[0] != "exist-1" {
		t.Errorf("expected exist-1 archived, got %v", repo.archivedIDs)
	}
	if got := repo.derivedFrom[createdID]; len(got) != 1 || got[0] != "exist-1" {
		t.Errorf("expected derived_from [exist-1] on %s, got %v", createdID, repo.derivedFrom)
	}
	// The merged skill's storage dir must contain the file union.
	mergedDir := filepath.Join(root, "merged-skill")
	for name, want := range map[string]string{
		"SKILL.md":  "---\nname: Merged Skill\ndescription: d\n---\nmerged body",
		"extra.txt": "existing aux",
		"new.py":    "print('new')",
	} {
		data, err := os.ReadFile(filepath.Join(mergedDir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("expected %s in merged dir: %v", name, err)
			continue
		}
		if string(data) != want {
			t.Errorf("merged dir %s: expected %q, got %q", name, want, string(data))
		}
	}
}

// F7: merge apply without retire_sources leaves the existing skills active.
func TestApplyImport_mergeGroupWithAI_NoRetire(t *testing.T) {
	existingDir := writeExistingSkillDir(t, map[string]string{
		"SKILL.md": "old body",
	})
	repo := &stubSkillRepo{
		storageDirs: map[string]string{"exist-1": existingDir},
	}
	eng, _ := setupEngineWithTempRoot(t, repo)
	seedMergeJob(eng, "job-merge-keep", "g1", []biz.SkillSimilaritySource{
		{ID: "exist-1", Name: "Old", Slug: "old-skill"},
	})

	result, err := eng.ApplyImport(context.Background(), "job-merge-keep", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{{
			Action:     "merge_group_with_ai",
			GroupID:    "g1",
			MergedName: "Merged Skill",
			MergedBody: "---\nname: Merged Skill\ndescription: d\n---\nmerged body",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CreatedSkillIDs) != 1 {
		t.Fatalf("expected 1 created skill, got %v", result.CreatedSkillIDs)
	}
	if len(repo.archivedIDs) != 0 {
		t.Errorf("expected no archived skills without retire_sources, got %v", repo.archivedIDs)
	}
}

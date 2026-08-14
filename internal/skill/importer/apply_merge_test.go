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

// S1: when a merge with retire_sources succeeds but a later decision fails,
// the source skills must NOT be archived — retirement is deferred until all
// decisions succeed, so compensate() only removes the newly created skills.
// (Old behaviour archived sources inside the decision loop, leaving archived
// sources behind while the merged skill was compensation-deleted.)
func TestApplyImport_mergeGroupWithAI_RetireDeferredOnLaterFailure(t *testing.T) {
	existDir1 := writeExistingSkillDir(t, map[string]string{"SKILL.md": "old body 1"})
	existDir2 := writeExistingSkillDir(t, map[string]string{"SKILL.md": "old body 2"})
	repo := &stubSkillRepo{
		storageDirs:  map[string]string{"exist-1": existDir1, "exist-2": existDir2},
		failOnCreate: 2, // second decision's create fails
	}
	eng, root := setupEngineWithTempRoot(t, repo)

	mkCandidate := func(id string) biz.SkillImportCandidate {
		return biz.SkillImportCandidate{CandidateID: id, Name: "Cand " + id, Slug: "cand-" + id, ValidationStatus: "warn"}
	}
	c1, c2 := mkCandidate("c-m1"), mkCandidate("c-m2")
	eng.jobsMu.Lock()
	eng.jobs["job-merge-partial"] = &jobState{
		public: biz.SkillImportJob{
			JobID:  "job-merge-partial",
			Status: "completed",
			ConflictGroups: []biz.SkillConflictGroup{
				{GroupID: "g1", CandidateIDs: []string{c1.CandidateID}, ExistingSkills: []biz.SkillSimilaritySource{{ID: "exist-1", Name: "Old1", Slug: "old-1"}}},
				{GroupID: "g2", CandidateIDs: []string{c2.CandidateID}, ExistingSkills: []biz.SkillSimilaritySource{{ID: "exist-2", Name: "Old2", Slug: "old-2"}}},
			},
		},
		candidates: map[string]candidateState{
			c1.CandidateID: {public: c1, body: "# C1", files: map[string][]byte{"SKILL.md": []byte("# C1")}},
			c2.CandidateID: {public: c2, body: "# C2", files: map[string][]byte{"SKILL.md": []byte("# C2")}},
		},
	}
	eng.jobsMu.Unlock()

	_, err := eng.ApplyImport(context.Background(), "job-merge-partial", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{
			{Action: "merge_group_with_ai", GroupID: "g1", MergedName: "Merged One", MergedBody: "---\nname: Merged One\ndescription: d\n---\nbody1", RetireSources: true},
			{Action: "merge_group_with_ai", GroupID: "g2", MergedName: "Merged Two", MergedBody: "---\nname: Merged Two\ndescription: d\n---\nbody2"},
		},
	})
	if err == nil {
		t.Fatal("expected error from second decision, got nil")
	}
	// Core S1 assertion: sources must stay untouched on partial failure.
	if len(repo.archivedIDs) != 0 {
		t.Errorf("expected no archived sources on partial failure, got %v", repo.archivedIDs)
	}
	// The first decision's merged skill must have been compensation-deleted.
	if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != "skill-1" {
		t.Errorf("expected compensated delete of skill-1, got %v", repo.deletedIDs)
	}
	if _, statErr := os.Stat(filepath.Join(root, "merged-one")); !os.IsNotExist(statErr) {
		t.Errorf("expected merged-one dir removed by compensation, stat err = %v", statErr)
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

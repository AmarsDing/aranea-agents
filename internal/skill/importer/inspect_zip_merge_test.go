package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// buildZipBytes packs name→content entries into an in-memory ZIP.
func buildZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func newInspectJob() *jobState {
	return &jobState{
		public: biz.SkillImportJob{
			JobID:          "job-inspect",
			Status:         "processing",
			StorageRoot:    "root",
			Candidates:     []biz.SkillImportCandidate{},
			ConflictGroups: []biz.SkillConflictGroup{},
		},
		candidates: map[string]candidateState{},
	}
}

func hasWarningType(candidate biz.SkillImportCandidate, warnType string) bool {
	for _, w := range candidate.Warnings {
		if w.Type == warnType {
			return true
		}
	}
	return false
}

// F2: a root SKILL.md plus subdirectory entries (core/x.py, assets/...) must be
// merged into the root candidate instead of being silently dropped.
func TestInspectSkillZip_MergesSubdirFilesIntoRootSkill(t *testing.T) {
	eng := &Engine{repo: &stubSkillRepo{}}
	zipBytes := buildZipBytes(t, map[string]string{
		"SKILL.md":         makeSkillMD("Root Skill", "root layout"),
		"core/x.py":        "print('x')",
		"assets/data.json": `{"k": "v"}`,
	})
	job := newInspectJob()
	if err := eng.inspectSkillZip(context.Background(), zipBytes, job); err != nil {
		t.Fatalf("inspectSkillZip failed: %v", err)
	}
	if len(job.public.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(job.public.Candidates))
	}
	candidate := job.public.Candidates[0]
	state := job.candidates[candidate.CandidateID]
	for _, want := range []string{"SKILL.md", "core/x.py", "assets/data.json"} {
		if _, ok := state.files[want]; !ok {
			t.Errorf("expected merged files to contain %q, got keys %v", want, keysOf(state.files))
		}
	}
	if string(state.files["core/x.py"]) != "print('x')" {
		t.Errorf("expected core/x.py content preserved, got %q", string(state.files["core/x.py"]))
	}
	if hasWarningType(candidate, "files_skipped") {
		t.Errorf("expected no files_skipped warning when every group is merged, got %+v", candidate.Warnings)
	}
}

// F2: a package with several SKILL.md groups must never cross-merge files
// between skills; leftover groups without SKILL.md surface a files_skipped
// warning instead of vanishing.
func TestInspectSkillZip_MultiSkillLayoutNoCrossMerge(t *testing.T) {
	eng := &Engine{repo: &stubSkillRepo{}}
	zipBytes := buildZipBytes(t, map[string]string{
		"alpha/SKILL.md": makeSkillMD("Alpha", "skill alpha"),
		"alpha/alpha.py": "print('alpha')",
		"beta/SKILL.md":  makeSkillMD("Beta", "skill beta"),
		"beta/beta.py":   "print('beta')",
		"shared/util.py": "print('shared')",
	})
	job := newInspectJob()
	if err := eng.inspectSkillZip(context.Background(), zipBytes, job); err != nil {
		t.Fatalf("inspectSkillZip failed: %v", err)
	}
	if len(job.public.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(job.public.Candidates))
	}
	for _, candidate := range job.public.Candidates {
		state := job.candidates[candidate.CandidateID]
		for fname := range state.files {
			if strings.HasPrefix(fname, "shared/") {
				t.Errorf("candidate %s must not absorb shared group file %q", candidate.Slug, fname)
			}
			if candidate.Slug == "alpha" && strings.HasPrefix(fname, "beta/") {
				t.Errorf("alpha must not absorb beta file %q", fname)
			}
			if candidate.Slug == "beta" && strings.HasPrefix(fname, "alpha/") {
				t.Errorf("beta must not absorb alpha file %q", fname)
			}
		}
	}
	// The shared group (no SKILL.md, not merged) must produce a files_skipped
	// warning attached to the first candidate.
	first := job.public.Candidates[0]
	if !hasWarningType(first, "files_skipped") {
		t.Fatalf("expected files_skipped warning on first candidate, got %+v", first.Warnings)
	}
	foundShared := false
	for _, w := range first.Warnings {
		if w.Type == "files_skipped" && strings.Contains(w.Message, "shared") {
			foundShared = true
		}
	}
	if !foundShared {
		t.Errorf("expected files_skipped warning to name the shared group, got %+v", first.Warnings)
	}
}

// F2: a single skill living in a subdirectory keeps its own files (baseline
// grouping — no merging involved).
func TestInspectSkillZip_SubdirSkillKeepsOwnFiles(t *testing.T) {
	eng := &Engine{repo: &stubSkillRepo{}}
	zipBytes := buildZipBytes(t, map[string]string{
		"myskill/SKILL.md":       makeSkillMD("My Skill", "subdir layout"),
		"myskill/scripts/run.py": "print('run')",
	})
	job := newInspectJob()
	if err := eng.inspectSkillZip(context.Background(), zipBytes, job); err != nil {
		t.Fatalf("inspectSkillZip failed: %v", err)
	}
	if len(job.public.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(job.public.Candidates))
	}
	candidate := job.public.Candidates[0]
	state := job.candidates[candidate.CandidateID]
	if _, ok := state.files["scripts/run.py"]; !ok {
		t.Errorf("expected scripts/run.py in candidate files, got %v", keysOf(state.files))
	}
	if hasWarningType(candidate, "files_skipped") {
		t.Errorf("expected no files_skipped warning, got %+v", candidate.Warnings)
	}
}

func keysOf(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

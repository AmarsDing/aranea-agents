package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func makeStoreJob(id string) biz.SkillImportJob {
	return biz.SkillImportJob{
		JobID:            id,
		Status:           "completed",
		ValidationStatus: "warn",
		StorageRoot:      "/tmp/skills",
		Message:          "ready",
		TempDir:          "/tmp/skills/.import-tmp/" + id,
		Candidates: []biz.SkillImportCandidate{{
			CandidateID:      "c1",
			Name:             "Alpha",
			Slug:             "alpha",
			Description:      "skill alpha",
			ValidationStatus: "pass",
			StatusIcon:       "check",
			Warnings:         []biz.SkillImportIssue{{Type: "files_skipped", Message: "zip entries not imported: shared"}},
		}},
		ConflictGroups: []biz.SkillConflictGroup{{
			GroupID:                "g1",
			HighestSimilarityScore: 0.5,
			Reason:                 "similar",
			Evidence:               []string{"name overlap"},
			CandidateIDs:           []string{"c1"},
			ExistingSkills:         []biz.SkillSimilaritySource{{ID: "e1", Name: "Old", Slug: "old-skill"}},
		}},
	}
}

// F5: jobs persist to the database and round-trip with candidates, conflict
// groups, and temp dir intact.
func TestSkillImportJobStore_CreateGetRoundTrip(t *testing.T) {
	d := newTestDataPG(t)
	store := NewSkillImportJobStore(d)
	ctx := context.Background()

	if err := store.Create(ctx, makeStoreJob("job-store-rt")); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.Get(ctx, "job-store-rt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected job, got nil")
	}
	if got.Status != "completed" || got.ValidationStatus != "warn" {
		t.Errorf("status fields mismatch: %+v", got)
	}
	if got.StorageRoot != "/tmp/skills" || got.TempDir != "/tmp/skills/.import-tmp/job-store-rt" {
		t.Errorf("path fields mismatch: %+v", got)
	}
	if len(got.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got.Candidates))
	}
	c := got.Candidates[0]
	if c.CandidateID != "c1" || c.Slug != "alpha" {
		t.Errorf("candidate mismatch: %+v", c)
	}
	if len(c.Warnings) != 1 || c.Warnings[0].Type != "files_skipped" {
		t.Errorf("candidate warnings mismatch: %+v", c.Warnings)
	}
	if len(got.ConflictGroups) != 1 {
		t.Fatalf("expected 1 conflict group, got %d", len(got.ConflictGroups))
	}
	g := got.ConflictGroups[0]
	if g.GroupID != "g1" || len(g.CandidateIDs) != 1 || g.CandidateIDs[0] != "c1" {
		t.Errorf("conflict group mismatch: %+v", g)
	}
	if len(g.ExistingSkills) != 1 || g.ExistingSkills[0].ID != "e1" {
		t.Errorf("existing skills mismatch: %+v", g.ExistingSkills)
	}
}

// F5: Get on a missing job returns (nil, nil) so the engine can fall back to
// ErrImportJobNotFound.
func TestSkillImportJobStore_GetMissing(t *testing.T) {
	d := newTestDataPG(t)
	store := NewSkillImportJobStore(d)

	got, err := store.Get(context.Background(), "job-store-missing")
	if err != nil {
		t.Fatalf("expected nil error for missing job, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil job for missing id, got %+v", got)
	}
}

// F5: UpdateStatus transitions status/message; "applied" also stamps
// applied_at.
func TestSkillImportJobStore_UpdateStatus(t *testing.T) {
	d := newTestDataPG(t)
	store := NewSkillImportJobStore(d)
	ctx := context.Background()

	if err := store.Create(ctx, makeStoreJob("job-store-us")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.UpdateStatus(ctx, "job-store-us", "applied", "import completed"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, err := store.Get(ctx, "job-store-us")
	if err != nil || got == nil {
		t.Fatalf("get: %v, %+v", err, got)
	}
	if got.Status != "applied" || got.Message != "import completed" {
		t.Errorf("expected applied/import completed, got %s/%s", got.Status, got.Message)
	}
}

// F5: CompareAndSwapStatus succeeds only when the current status matches —
// this guards ApplyImport against duplicate applies after a restart.
func TestSkillImportJobStore_CompareAndSwapStatus(t *testing.T) {
	d := newTestDataPG(t)
	store := NewSkillImportJobStore(d)
	ctx := context.Background()

	if err := store.Create(ctx, makeStoreJob("job-store-cas")); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Mismatched expected status → no swap.
	swapped, err := store.CompareAndSwapStatus(ctx, "job-store-cas", "applied", "applying", "")
	if err != nil {
		t.Fatalf("cas mismatch: %v", err)
	}
	if swapped {
		t.Fatal("expected no swap when current status != expectedStatus")
	}
	// Matching expected status → swap.
	swapped, err = store.CompareAndSwapStatus(ctx, "job-store-cas", "completed", "applying", "")
	if err != nil {
		t.Fatalf("cas: %v", err)
	}
	if !swapped {
		t.Fatal("expected swap when current status == expectedStatus")
	}
	// Second identical swap must fail (status is now "applying").
	swapped, err = store.CompareAndSwapStatus(ctx, "job-store-cas", "completed", "applying", "")
	if err != nil {
		t.Fatalf("cas repeat: %v", err)
	}
	if swapped {
		t.Fatal("expected second CAS to fail after status changed")
	}
	got, err := store.Get(ctx, "job-store-cas")
	if err != nil || got == nil {
		t.Fatalf("get: %v, %+v", err, got)
	}
	if got.Status != "applying" {
		t.Errorf("expected status applying, got %s", got.Status)
	}
}

// F5: UpdateCandidates replaces the stored candidate/conflict-group payloads.
func TestSkillImportJobStore_UpdateCandidates(t *testing.T) {
	d := newTestDataPG(t)
	store := NewSkillImportJobStore(d)
	ctx := context.Background()

	if err := store.Create(ctx, makeStoreJob("job-store-uc")); err != nil {
		t.Fatalf("create: %v", err)
	}
	updated := []biz.SkillImportCandidate{{
		CandidateID: "c2", Name: "Beta", Slug: "beta", ValidationStatus: "block",
		Blocks: []biz.SkillImportIssue{{Type: "duplicate_name", Message: "slug exists"}},
	}}
	if err := store.UpdateCandidates(ctx, "job-store-uc", updated, []biz.SkillConflictGroup{}); err != nil {
		t.Fatalf("update candidates: %v", err)
	}
	got, err := store.Get(ctx, "job-store-uc")
	if err != nil || got == nil {
		t.Fatalf("get: %v, %+v", err, got)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].CandidateID != "c2" {
		t.Fatalf("expected replaced candidate c2, got %+v", got.Candidates)
	}
	if len(got.Candidates[0].Blocks) != 1 || got.Candidates[0].Blocks[0].Type != "duplicate_name" {
		t.Errorf("expected duplicate_name block, got %+v", got.Candidates[0].Blocks)
	}
	if len(got.ConflictGroups) != 0 {
		t.Errorf("expected conflict groups cleared, got %+v", got.ConflictGroups)
	}
}

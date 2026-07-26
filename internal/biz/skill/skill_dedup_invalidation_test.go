package skill

import (
	"testing"
)

// fakeDedupInvalidator records InvalidateDedupCache calls.
type fakeDedupInvalidator struct{ calls int }

func (f *fakeDedupInvalidator) InvalidateDedupCache() { f.calls++ }

// A3: skill mutations must invalidate the dedup result cache so merged /
// deleted / updated skills do not linger in DetectDuplicateGroups results.
func TestDedupCacheInvalidated_OnMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(u *Usecase) error
	}{
		{"Create", func(u *Usecase) error {
			_, err := u.Create(adminCtx(), CreateInput{Name: "N", Slug: "n"})
			return err
		}},
		{"ToggleEnabled", func(u *Usecase) error {
			_, err := u.ToggleEnabled(adminCtx(), "s1", false)
			return err
		}},
		{"Duplicate", func(u *Usecase) error {
			_, err := u.Duplicate(adminCtx(), "s1")
			return err
		}},
		{"Delete", func(u *Usecase) error {
			return u.Delete(adminCtx(), "s1")
		}},
		{"Patch", func(u *Usecase) error {
			_, err := u.Patch(adminCtx(), "s1", UpdateDraft{HasName: true, Name: "New"})
			return err
		}},
		{"UpsertSkillFromDisk", func(u *Usecase) error {
			_, _, err := u.UpsertSkillFromDisk(adminCtx(), DiskSyncInput{Name: "N", Slug: "n"})
			return err
		}},
		{"RollbackVersion", func(u *Usecase) error {
			_, err := u.RollbackVersion(adminCtx(), "s1", "v1")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newMockRepo()
			r.skills["s1"] = sampleSkill("s1", "Test", "test")
			inv := &fakeDedupInvalidator{}
			u := NewUsecase(r, nil)
			u.SetDedupCacheInvalidator(inv)
			if err := tc.mutate(u); err != nil {
				t.Fatalf("mutate failed: %v", err)
			}
			if inv.calls == 0 {
				t.Errorf("%s: expected dedup cache invalidation, got 0 calls", tc.name)
			}
		})
	}
}

// Nil invalidator must not panic (optional dependency).
func TestDedupCacheInvalidation_NilSafe(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	if _, err := u.Create(adminCtx(), CreateInput{Name: "N", Slug: "n"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

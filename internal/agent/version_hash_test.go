package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestComputeSkillVersionHash_Empty(t *testing.T) {
	if got := ComputeSkillVersionHash(nil); got != "" {
		t.Fatalf("empty refs must yield empty hash, got %q", got)
	}
}

func TestComputeSkillVersionHash_OrderIndependent(t *testing.T) {
	a := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	b := []biz.SkillEnabledRef{a[1], a[0]}
	if ComputeSkillVersionHash(a) != ComputeSkillVersionHash(b) {
		t.Fatal("hash must be order-independent")
	}
}

func TestComputeSkillVersionHash_ContentChangeBumpsHash(t *testing.T) {
	before := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	after := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-03T00:00:00Z"}, // content mutated
	}
	if ComputeSkillVersionHash(before) == ComputeSkillVersionHash(after) {
		t.Fatal("hash must change when a skill's content version changes")
	}
}

func TestComputeSkillVersionHash_StableWhenUnchanged(t *testing.T) {
	refs := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	if ComputeSkillVersionHash(refs) != ComputeSkillVersionHash(refs) {
		t.Fatal("hash must be stable across calls when nothing changed")
	}
}

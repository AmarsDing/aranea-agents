package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// 物理删除语义（B1 修复）：DeleteSkill 必须硬删除 skill + versions 行，
// 释放 skill_key 全表唯一约束，使同 slug 可以立即重建/重导入。
// 旧行为（软删除墓碑）下本测试必然失败：recreate 撞 skill_key 唯一约束。

func TestDeleteSkill_AllowsRecreateSameSlug(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()

	sk, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Recreate Me", Slug: "recreate-me", Body: "# V1 body",
	})
	if err != nil {
		t.Fatalf("CreateSkillWithVersion: %v", err)
	}

	if err := r.DeleteSkill(ctx, sk.ID); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}

	// 同 slug 重建必须成功（物理删除已释放唯一键）。
	sk2, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Recreate Me v2", Slug: "recreate-me", Body: "# V2 body",
	})
	if err != nil {
		t.Fatalf("recreate same slug after delete: %v", err)
	}
	if sk2.ID == sk.ID {
		t.Fatal("recreated skill reuses the old ID, want a fresh row")
	}

	// 旧版本的版本行必须随 skill 物理删除，不得残留孤儿版本。
	versions, err := r.ListSkillVersions(ctx, biz.SkillVersionListQuery{SkillID: sk.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListSkillVersions for deleted skill: %v", err)
	}
	if len(versions.Items) != 0 {
		t.Fatalf("deleted skill still has %d version rows, want 0", len(versions.Items))
	}
}

func TestDeleteSkill_NotFound(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	if err := r.DeleteSkill(context.Background(), "skill_nonexistent"); err == nil {
		t.Fatal("DeleteSkill on nonexistent id: expected NotFound error, got nil")
	}
}

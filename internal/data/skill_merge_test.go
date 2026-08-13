package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 批次 C2：合并废弃源 Skill 生成软删墓碑时，必须释放 skill_key 全表唯一键
// （skill_key 唯一索引不含状态过滤），否则墓碑永久占用 slug，阻塞同名重建/导入。
func TestApplyMerge_ReleasesSourceSkillKey(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()

	src, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Merge Src", Slug: "merge-src", Body: "# src body",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	tgt, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Merge Tgt", Slug: "merge-tgt", Body: "# tgt body",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	mr := NewSkillMergeRepo(d, loggateway.NewNoop())
	if _, err := mr.ApplyMerge(ctx, biz.SkillMergeApplyParams{
		TargetID:    tgt.ID,
		SourceID:    src.ID,
		FusedBody:   "# fused body",
		FusedTags:   []string{"merged"},
		MergeReason: "test merge",
	}); err != nil {
		t.Fatalf("ApplyMerge: %v", err)
	}

	// 源 slug 必须已释放：同名重建不得报唯一键冲突。
	if _, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Merge Src Reborn", Slug: "merge-src", Body: "# reborn body",
	}); err != nil {
		t.Fatalf("source slug must be released after merge tombstone, got: %v", err)
	}
}

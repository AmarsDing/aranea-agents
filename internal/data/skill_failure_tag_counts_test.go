package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// seedReport 经 repo 层写入一条经验报告（走 ent builder，与生产写路径一致）。
func seedReport(t *testing.T, r *SkillIntelligenceRepo, id, skillID string, isSuccess bool, tags []string, createdAt time.Time) {
	t.Helper()
	err := r.Create(context.Background(), biz.ExperienceReport{
		ID:          id,
		SkillID:     skillID,
		SkillName:   "seed-" + skillID,
		IsSuccess:   isSuccess,
		Score:       50,
		FailureTags: tags,
		CreatedAt:   createdAt,
	})
	if err != nil {
		t.Fatalf("seed report %s: %v", id, err)
	}
}

// 审计 T8 验收：失败标签聚合已下沉为方言感知 SQL（json 数组展开 + GROUP BY），
// 本测试锁定其语义与原 Go 内存聚合一致——谓词过滤、跨标签计数、count desc
// 排序、空结果非 nil 切片。
func TestSkillIntelligenceRepo_FailureTagCountsSQL(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillIntelligenceRepo(d, loggateway.NewNoop())
	ctx := context.Background()

	base := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	// skill-a：timeout×2、api_error×1、无标签失败×1；success 行与 skill-b
	// 行不得计入 skill-a 的聚合。
	seedReport(t, r, "ft-r1", "skill-a", false, []string{"timeout", "api_error"}, base)
	seedReport(t, r, "ft-r2", "skill-a", false, []string{"timeout"}, base.Add(time.Hour))
	seedReport(t, r, "ft-r3", "skill-a", false, nil, base.Add(2*time.Hour))
	seedReport(t, r, "ft-r4", "skill-a", true, []string{"timeout"}, base)
	seedReport(t, r, "ft-r5", "skill-b", false, []string{"timeout"}, base)

	since := base.Add(-time.Hour)

	t.Run("GetFailureTagCounts 单技能聚合与排序", func(t *testing.T) {
		got, err := r.GetFailureTagCounts(ctx, "skill-a", since)
		if err != nil {
			t.Fatalf("GetFailureTagCounts: %v", err)
		}
		want := []biz.FailureTagCount{{Tag: "timeout", Count: 2}, {Tag: "api_error", Count: 1}}
		if len(got) != len(want) {
			t.Fatalf("len mismatch: got %+v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("row %d mismatch: got %+v want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("GetFailureTagCountsFiltered 技能过滤", func(t *testing.T) {
		got, err := r.GetFailureTagCountsFiltered(ctx, "skill-a", nil, nil)
		if err != nil {
			t.Fatalf("GetFailureTagCountsFiltered: %v", err)
		}
		if len(got) != 2 || got[0].Tag != "timeout" || got[0].Count != 2 || got[1].Tag != "api_error" || got[1].Count != 1 {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("GetFailureTagCountsFiltered 跨技能", func(t *testing.T) {
		got, err := r.GetFailureTagCountsFiltered(ctx, "", nil, nil)
		if err != nil {
			t.Fatalf("GetFailureTagCountsFiltered: %v", err)
		}
		if len(got) != 2 || got[0].Tag != "timeout" || got[0].Count != 3 || got[1].Tag != "api_error" || got[1].Count != 1 {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("GetFailureTagCountsFiltered 时间窗过滤", func(t *testing.T) {
		// startTime 晚于 ft-r1 → timeout 只剩 ft-r2 一次。
		start := base.Add(30 * time.Minute)
		got, err := r.GetFailureTagCountsFiltered(ctx, "skill-a", &start, nil)
		if err != nil {
			t.Fatalf("GetFailureTagCountsFiltered: %v", err)
		}
		if len(got) != 1 || got[0].Tag != "timeout" || got[0].Count != 1 {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("无命中返回空非 nil 切片", func(t *testing.T) {
		got, err := r.GetFailureTagCountsFiltered(ctx, "skill-nonexistent", nil, nil)
		if err != nil {
			t.Fatalf("GetFailureTagCountsFiltered: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("expected empty non-nil slice, got %#v", got)
		}
	})
}

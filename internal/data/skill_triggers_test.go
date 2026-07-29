package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// P1-3：frontmatter triggers 必须经 metadata envelope 落库，
// 并出现在运行时路由候选（ListEnabledPublishedSkillCandidates）中。
func TestSkillMetadataTriggers_Roundtrip(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()

	sk := seedDiskSkill(t, r, dir, "trig-roundtrip", "---\nname: trig-roundtrip\ntriggers: [报销, invoice]\n---\n# body")
	_ = sk

	// 重新 upsert 携带 triggers（watcher 从 frontmatter 解析后传入）。
	if _, _, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "trig-roundtrip", Slug: "trig-roundtrip",
		Body:       "---\nname: trig-roundtrip\ntriggers: [报销, invoice]\n---\n# body",
		StorageDir: dir,
		Triggers:   []string{"报销", "invoice"},
	}); err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}

	if _, err := r.PublishSkill(ctx, sk.ID, "pass"); err != nil {
		t.Fatalf("PublishSkill: %v", err)
	}
	if _, err := r.UpdateSkillEnabled(ctx, sk.ID, true); err != nil {
		t.Fatalf("UpdateSkillEnabled: %v", err)
	}

	got, err := r.GetSkillByID(ctx, sk.ID)
	if err != nil {
		t.Fatalf("GetSkillByID: %v", err)
	}
	if len(got.Triggers) != 2 || got.Triggers[0] != "报销" || got.Triggers[1] != "invoice" {
		t.Fatalf("Skill.Triggers = %v, want [报销 invoice]", got.Triggers)
	}

	cands, err := r.ListEnabledPublishedSkillCandidates(ctx)
	if err != nil {
		t.Fatalf("ListEnabledPublishedSkillCandidates: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.Slug == "trig-roundtrip" {
			found = true
			if len(c.Triggers) != 2 {
				t.Fatalf("candidate Triggers = %v, want 2 entries", c.Triggers)
			}
		}
	}
	if !found {
		t.Fatal("enabled+published skill not found in runtime candidates")
	}
}

// P1-3：旧格式 metadata（无 triggers 字段）必须解析为空切片而非报错。
func TestParseSkillTriggers_LegacyMetadata(t *testing.T) {
	if got := parseSkillTriggers(`{"tags":[{"name":"a","source":"user"}]}`); len(got) != 0 {
		t.Fatalf("expected empty triggers for legacy envelope, got %v", got)
	}
	if got := parseSkillTriggers(""); len(got) != 0 {
		t.Fatalf("expected empty triggers for empty metadata, got %v", got)
	}
	// 去重 + 去空 + trim
	got := parseSkillTriggers(`{"triggers":[" 报销 ","报销","","invoice"]}`)
	if len(got) != 2 || got[0] != "报销" || got[1] != "invoice" {
		t.Fatalf("expected normalized [报销 invoice], got %v", got)
	}
}

// P1-3：import overwrite 替换正文时必须同步刷新 triggers，
// 否则残留旧 frontmatter 的触发词；storage_dir 等既有元数据必须保留。
func TestAppendImportedVersion_RefreshesTriggers(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()

	sk := seedDiskSkill(t, r, dir, "trig-overwrite", "---\nname: trig-overwrite\ntriggers: [旧词]\n---\n# body")
	if _, _, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "trig-overwrite", Slug: "trig-overwrite",
		Body: "---\nname: trig-overwrite\ntriggers: [旧词]\n---\n# body", StorageDir: dir,
		Triggers: []string{"旧词"},
	}); err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}

	if _, err := r.AppendImportedVersion(ctx, biz.SkillImportVersionInput{
		SkillID: sk.ID, Name: "trig-overwrite", Description: "d",
		Body:     "---\nname: trig-overwrite\ntriggers: [报销, invoice]\n---\n# new body",
		Tags:     []biz.SkillTag{{Name: "t", Source: "user"}},
		Triggers: []string{"报销", "invoice"},
	}); err != nil {
		t.Fatalf("AppendImportedVersion: %v", err)
	}

	got, err := r.GetSkillByID(ctx, sk.ID)
	if err != nil {
		t.Fatalf("GetSkillByID: %v", err)
	}
	if len(got.Triggers) != 2 || got.Triggers[0] != "报销" || got.Triggers[1] != "invoice" {
		t.Fatalf("Skill.Triggers = %v, want [报销 invoice]", got.Triggers)
	}
	if got.StorageDir != dir {
		t.Fatalf("StorageDir = %q, want preserved %q", got.StorageDir, dir)
	}
}

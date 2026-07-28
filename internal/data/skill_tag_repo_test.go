package data

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func apiErrCode(t *testing.T, err error) apierror.Code {
	t.Helper()
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apierror.Error, got %T: %v", err, err)
	}
	return ae.Code
}

func seedSkillWithTags(t *testing.T, r biz.SkillRepo, slug string, tags []biz.SkillTag) biz.Skill {
	t.Helper()
	sk, err := r.CreateSkillWithVersion(context.Background(), biz.SkillCreateInput{
		Name: "Seed " + slug,
		Slug: slug,
		Body: "# seed",
		Tags: tags,
	})
	if err != nil {
		t.Fatalf("seed skill %s: %v", slug, err)
	}
	return sk
}

func readSkillTags(t *testing.T, r biz.SkillRepo, id string) []biz.SkillTag {
	t.Helper()
	sk, err := r.GetSkillByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetSkillByID: %v", err)
	}
	return sk.Tags
}

func tagNames(tags []biz.SkillTag) []string {
	out := make([]string, 0, len(tags))
	for _, tg := range tags {
		out = append(out, tg.Name)
	}
	return out
}

func TestSkillTagRepo_CreateAndListWithUsage(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "file_type:xlsx"); err != nil {
		t.Fatalf("CreateSkillTag: %v", err)
	}
	seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "file_type:xlsx"}, {Name: "figma"}})
	seedSkillWithTags(t, skillR, "s2", []biz.SkillTag{{Name: "File_Type:XLSX"}}) // 大小写不敏感计数

	items, err := tagR.ListSkillTags(ctx)
	if err != nil {
		t.Fatalf("ListSkillTags: %v", err)
	}
	byName := map[string]biz.SkillTagInfo{}
	for _, it := range items {
		byName[it.Name] = it
	}
	dictTag, ok := byName["file_type:xlsx"]
	if !ok {
		t.Fatalf("dictionary tag missing from list: %+v", items)
	}
	if dictTag.Dimension != "file_type" || dictTag.Source != "user" {
		t.Errorf("unexpected dict tag: %+v", dictTag)
	}
	if dictTag.UsedCount != 2 {
		t.Errorf("used_count = %d, want 2 (case-insensitive)", dictTag.UsedCount)
	}
	orphan, ok := byName["figma"]
	if !ok || orphan.Source != "orphan" || orphan.UsedCount != 1 {
		t.Errorf("expected orphan figma with count 1, got %+v (present=%v)", orphan, ok)
	}
}

func TestSkillTagRepo_ListNamesMergesDictAndUsage(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "domain:sales"); err != nil {
		t.Fatalf("CreateSkillTag: %v", err)
	}
	seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "unused-in-dict"}})

	names, err := tagR.ListSkillTagNames(ctx)
	if err != nil {
		t.Fatalf("ListSkillTagNames: %v", err)
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if !seen["domain:sales"] || !seen["unused-in-dict"] {
		t.Errorf("names must merge dictionary + usage, got %v", names)
	}
}

func TestSkillTagRepo_CreateConflict(t *testing.T) {
	d := newTestDataPG(t)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()
	if _, err := tagR.CreateSkillTag(ctx, "figma"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := tagR.CreateSkillTag(ctx, "figma")
	if err == nil {
		t.Fatal("expected conflict on duplicate create")
	}
	if apiErrCode(t, err) != apierror.CodeConflict {
		t.Errorf("expected CodeConflict, got %v", err)
	}
}

func TestSkillTagRepo_RenameRewritesSkills(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "figma"); err != nil {
		t.Fatalf("CreateSkillTag: %v", err)
	}
	sk := seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "figma"}, {Name: "other"}})
	seedSkillWithTags(t, skillR, "s2", []biz.SkillTag{{Name: "untouched"}})

	n, err := tagR.RenameSkillTag(ctx, "figma", "design:figma")
	if err != nil {
		t.Fatalf("RenameSkillTag: %v", err)
	}
	if n != 1 {
		t.Errorf("rewritten = %d, want 1", n)
	}
	got := tagNames(readSkillTags(t, skillR, sk.ID))
	want := map[string]bool{"design:figma": true, "other": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("skill tags after rename = %v", got)
	}
	// 字典已改名。
	items, err := tagR.ListSkillTags(ctx)
	if err != nil {
		t.Fatalf("ListSkillTags: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Name == "design:figma" {
			found = true
			if it.Dimension != "design" {
				t.Errorf("dimension = %q, want design", it.Dimension)
			}
			if it.UsedCount != 1 {
				t.Errorf("used_count = %d, want 1", it.UsedCount)
			}
		}
		if it.Name == "figma" && it.Source != "orphan" {
			t.Errorf("old dictionary row must be renamed, got %+v", it)
		}
	}
	if !found {
		t.Error("renamed dictionary row missing")
	}
}

func TestSkillTagRepo_RenamePreservesUnknownMetadataKeys(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "figma"); err != nil {
		t.Fatalf("CreateSkillTag: %v", err)
	}
	sk := seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "figma"}})
	// 注入未知键（模拟 taxonomy_paths 等 envelope 外字段）。
	raw, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw.MetadataJSON), &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta["taxonomy_paths"] = []string{"design/ui"}
	blob, _ := json.Marshal(meta)
	if err := d.RW().Write(ctx).PlatformSkill.UpdateOneID(sk.ID).
		SetMetadataJSON(string(blob)).Exec(ctx); err != nil {
		t.Fatalf("inject unknown key: %v", err)
	}

	if _, err := tagR.RenameSkillTag(ctx, "figma", "figma2"); err != nil {
		t.Fatalf("RenameSkillTag: %v", err)
	}
	after, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	var afterMeta map[string]any
	if err := json.Unmarshal([]byte(after.MetadataJSON), &afterMeta); err != nil {
		t.Fatalf("unmarshal after: %v", err)
	}
	paths, ok := afterMeta["taxonomy_paths"].([]any)
	if !ok || len(paths) != 1 || paths[0] != "design/ui" {
		t.Errorf("unknown metadata keys must survive tag rewrite, got %v", afterMeta)
	}
}

func TestSkillTagRepo_RenameMergeIntoExisting(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "figma"); err != nil {
		t.Fatalf("create figma: %v", err)
	}
	if _, err := tagR.CreateSkillTag(ctx, "design:figma"); err != nil {
		t.Fatalf("create design:figma: %v", err)
	}
	// 同一 skill 同时引用两个标签：合并后必须去重。
	sk := seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "figma"}, {Name: "design:figma"}})

	n, err := tagR.RenameSkillTag(ctx, "figma", "design:figma")
	if err != nil {
		t.Fatalf("RenameSkillTag merge: %v", err)
	}
	if n != 1 {
		t.Errorf("rewritten = %d, want 1", n)
	}
	got := tagNames(readSkillTags(t, skillR, sk.ID))
	if len(got) != 1 || got[0] != "design:figma" {
		t.Errorf("merged tags = %v, want [design:figma]", got)
	}
	// 旧字典行已删除。
	names, err := tagR.ListSkillTagNames(ctx)
	if err != nil {
		t.Fatalf("ListSkillTagNames: %v", err)
	}
	for _, name := range names {
		if name == "figma" {
			t.Errorf("old dictionary row must be deleted after merge, names=%v", names)
		}
	}
}

func TestSkillTagRepo_RenameNotInDictionary(t *testing.T) {
	d := newTestDataPG(t)
	tagR := NewSkillTagRepo(d)
	_, err := tagR.RenameSkillTag(context.Background(), "ghost", "ghost2")
	if err == nil {
		t.Fatal("expected NotFound for tag not in dictionary")
	}
	if apiErrCode(t, err) != apierror.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", err)
	}
}

func TestSkillTagRepo_DeleteRemovesReferences(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	if _, err := tagR.CreateSkillTag(ctx, "figma"); err != nil {
		t.Fatalf("CreateSkillTag: %v", err)
	}
	sk1 := seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "figma"}, {Name: "keep"}})
	sk2 := seedSkillWithTags(t, skillR, "s2", []biz.SkillTag{{Name: "Figma"}}) // 大小写不敏感移除

	n, err := tagR.DeleteSkillTag(ctx, "figma")
	if err != nil {
		t.Fatalf("DeleteSkillTag: %v", err)
	}
	if n != 2 {
		t.Errorf("rewritten = %d, want 2", n)
	}
	if got := tagNames(readSkillTags(t, skillR, sk1.ID)); len(got) != 1 || got[0] != "keep" {
		t.Errorf("s1 tags = %v, want [keep]", got)
	}
	if got := tagNames(readSkillTags(t, skillR, sk2.ID)); len(got) != 0 {
		t.Errorf("s2 tags = %v, want []", got)
	}
	// 字典行已删。
	items, err := tagR.ListSkillTags(ctx)
	if err != nil {
		t.Fatalf("ListSkillTags: %v", err)
	}
	for _, it := range items {
		if it.Name == "figma" {
			t.Errorf("figma must be gone from dictionary and usage, got %+v", it)
		}
	}
}

func TestSkillTagRepo_DeleteOrphanTolerated(t *testing.T) {
	d := newTestDataPG(t)
	skillR := NewSkillRepo(d)
	tagR := NewSkillTagRepo(d)
	ctx := context.Background()

	// 标签未收录进字典，但 skill 引用存在：删除仍应清理引用。
	sk := seedSkillWithTags(t, skillR, "s1", []biz.SkillTag{{Name: "orphan-tag"}})
	n, err := tagR.DeleteSkillTag(ctx, "orphan-tag")
	if err != nil {
		t.Fatalf("DeleteSkillTag orphan: %v", err)
	}
	if n != 1 {
		t.Errorf("rewritten = %d, want 1", n)
	}
	if got := tagNames(readSkillTags(t, skillR, sk.ID)); len(got) != 0 {
		t.Errorf("tags = %v, want []", got)
	}
}

package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// F3 (P-evo-1)：进化版本注册后必须同步落盘，保持 DB 与磁盘两个真相源一致；
// 且 watcher 对 evolution_reason != "" 的最新版本做保护——磁盘陈旧内容不得
// 反向覆盖进化版本（不落新版本、不回退 draft），而是以 DB 为准刷新磁盘。

func seedDiskSkill(t *testing.T, r biz.SkillRepo, dir, slug, body string) biz.Skill {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	sk, _, err := r.UpsertSkillFromDisk(context.Background(), biz.SkillDiskSyncInput{
		Name: "Seed", Slug: slug, Body: body, StorageDir: dir,
	})
	if err != nil {
		t.Fatalf("seed UpsertSkillFromDisk: %v", err)
	}
	return sk
}

func readDiskBody(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	return string(raw)
}

func TestCreateSkillVersion_SyncsBodyToDisk(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "evo-sync", "# V1\noriginal body")

	evolved := "# V2\nevolved body"
	if _, err := r.CreateSkillVersion(context.Background(), biz.SkillCreateVersionInput{
		SkillID: sk.ID, Body: evolved, EvolutionReason: "evolution: fix_failure",
	}); err != nil {
		t.Fatalf("CreateSkillVersion: %v", err)
	}
	if got := readDiskBody(t, dir); got != evolved {
		t.Fatalf("disk SKILL.md = %q, want evolved body %q (CreateSkillVersion must sync disk)", got, evolved)
	}
}

func TestUpsertSkillFromDisk_EvolutionDivergence_RefreshesDiskFromDB(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "evo-protect", "# V1\nbuggy body")
	if _, err := r.PublishSkill(ctx, sk.ID, "pass"); err != nil {
		t.Fatalf("PublishSkill: %v", err)
	}
	evolved := "# V2\nevolved body"
	if _, err := r.CreateSkillVersion(ctx, biz.SkillCreateVersionInput{
		SkillID: sk.ID, Body: evolved, EvolutionReason: "evolution: fix_failure",
	}); err != nil {
		t.Fatalf("CreateSkillVersion: %v", err)
	}

	// 模拟 watcher 以进化前的陈旧磁盘内容重新扫描。
	stale := "# V1\nbuggy body"
	_, outcome, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "Seed", Slug: "evo-protect", Body: stale, StorageDir: dir,
	})
	if err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}
	if outcome.ContentChanged {
		t.Fatal("ContentChanged = true, want false: stale disk content must not overwrite an evolution version")
	}
	if outcome.RevertedToDraft {
		t.Fatal("RevertedToDraft = true, want false: evolution versions are exempt from draft revert")
	}
	if got := readDiskBody(t, dir); got != evolved {
		t.Fatalf("disk SKILL.md = %q, want refreshed to DB evolution body %q", got, evolved)
	}
	versions, err := r.ListSkillVersions(ctx, biz.SkillVersionListQuery{SkillID: sk.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListSkillVersions: %v", err)
	}
	if len(versions.Items) != 2 {
		t.Fatalf("version count = %d, want 2 (no rollback version created)", len(versions.Items))
	}
	after, err := r.GetSkillByID(ctx, sk.ID)
	if err != nil {
		t.Fatalf("GetSkillByID: %v", err)
	}
	if after.Status != "published" {
		t.Fatalf("skill status = %q, want published (must not be silently taken offline)", after.Status)
	}
}

func TestUpsertSkillFromDisk_NonEvolutionDivergence_KeepsLegacyBehavior(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "plain-diverge", "# V1\noriginal")
	// 非进化版本（evolution_reason 为空）。
	if _, err := r.CreateSkillVersion(ctx, biz.SkillCreateVersionInput{
		SkillID: sk.ID, Body: "# V2\nmanual edit",
	}); err != nil {
		t.Fatalf("CreateSkillVersion: %v", err)
	}

	changed := "# V3\ndisk edited"
	_, outcome, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "Seed", Slug: "plain-diverge", Body: changed, StorageDir: dir,
	})
	if err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}
	if !outcome.ContentChanged {
		t.Fatal("ContentChanged = false, want true for non-evolution divergence (legacy watcher behavior)")
	}
	versions, err := r.ListSkillVersions(ctx, biz.SkillVersionListQuery{SkillID: sk.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListSkillVersions: %v", err)
	}
	if len(versions.Items) != 3 {
		t.Fatalf("version count = %d, want 3 (disk divergence creates a new version)", len(versions.Items))
	}
}

// P-r4-1：watcher 周期同步重建 metadata 时不得抹除 merge 血缘（derived_from）。
func TestUpsertSkillFromDisk_PreservesDerivedFrom(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "merged-skill", "# M\nmerged body")

	sources := []string{"skill_src_a", "skill_src_b"}
	if err := r.SetSkillDerivedFrom(ctx, sk.ID, sources); err != nil {
		t.Fatalf("SetSkillDerivedFrom: %v", err)
	}

	// 模拟 watcher 以相同内容周期重扫（内容未变，仅 metadata 重建路径）。
	if _, _, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "Seed", Slug: "merged-skill", Body: "# M\nmerged body", StorageDir: dir,
	}); err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}

	row, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
	if err != nil {
		t.Fatalf("PlatformSkill.Get: %v", err)
	}
	md := parseSkillMetadata(d.lg, row.MetadataJSON)
	if len(md.DerivedFrom) != 2 || md.DerivedFrom[0] != "skill_src_a" || md.DerivedFrom[1] != "skill_src_b" {
		t.Fatalf("derived_from = %v, want %v (watcher must preserve merge provenance)", md.DerivedFrom, sources)
	}
}

// 缓存键契约（internal/agent/version_hash.go）：platform_skill.updated_at 是
// SkillVersionHash 的内容版本标记，仅允许在内容/元数据/可用性实际变化时 bump。
// reconcile 每 5 分钟全量无变化扫描若触碰 updated_at，SkillVersionHash 漂移，
// 全量 agent 构建缓存周期性失效（每次冷构建约 8s）。
func TestUpsertSkillFromDisk_NoChange_KeepsUpdatedAt(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "cache-stable", "# body\nsame")

	const sentinel = "2020-01-01T00:00:00Z"
	if _, err := d.RW().Write(ctx).PlatformSkill.UpdateOneID(sk.ID).SetUpdatedAt(sentinel).Save(ctx); err != nil {
		t.Fatalf("set sentinel updated_at: %v", err)
	}

	_, outcome, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "Seed", Slug: "cache-stable", Body: "# body\nsame", StorageDir: dir,
	})
	if err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}
	if outcome.ContentChanged || outcome.RevertedToDraft {
		t.Fatalf("outcome = %+v, want no change", outcome)
	}

	row, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
	if err != nil {
		t.Fatalf("PlatformSkill.Get: %v", err)
	}
	if row.UpdatedAt != sentinel {
		t.Fatalf("updated_at = %q, want sentinel %q (no-change disk sync must not bump updated_at)", row.UpdatedAt, sentinel)
	}
}

// 反向守卫：内容真实变化时必须 bump updated_at（缓存失效信号），不得跳过写库。
func TestUpsertSkillFromDisk_ContentChange_BumpsUpdatedAt(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "cache-bump", "# body\nv1")

	const sentinel = "2020-01-01T00:00:00Z"
	if _, err := d.RW().Write(ctx).PlatformSkill.UpdateOneID(sk.ID).SetUpdatedAt(sentinel).Save(ctx); err != nil {
		t.Fatalf("set sentinel updated_at: %v", err)
	}

	_, outcome, err := r.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name: "Seed", Slug: "cache-bump", Body: "# body\nv2", StorageDir: dir,
	})
	if err != nil {
		t.Fatalf("UpsertSkillFromDisk: %v", err)
	}
	if !outcome.ContentChanged {
		t.Fatal("ContentChanged = false, want true")
	}

	row, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
	if err != nil {
		t.Fatalf("PlatformSkill.Get: %v", err)
	}
	if row.UpdatedAt == sentinel {
		t.Fatal("updated_at kept sentinel, want bumped (content change must invalidate agent build cache)")
	}
}

// MarkSkillFilesystemMissing 同样受缓存键契约约束：missing 标志无变化时不得
// 写库（reconcile 每个存活 skill 都会调一次 missing=false）。
func TestMarkSkillFilesystemMissing_NoChange_KeepsUpdatedAt(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	ctx := context.Background()
	dir := t.TempDir()
	sk := seedDiskSkill(t, r, dir, "cache-mark", "# body")

	const sentinel = "2020-01-01T00:00:00Z"
	setSentinel := func() {
		t.Helper()
		if _, err := d.RW().Write(ctx).PlatformSkill.UpdateOneID(sk.ID).SetUpdatedAt(sentinel).Save(ctx); err != nil {
			t.Fatalf("set sentinel updated_at: %v", err)
		}
	}
	readRow := func() string {
		t.Helper()
		row, err := d.RW().Read(ctx).PlatformSkill.Get(ctx, sk.ID)
		if err != nil {
			t.Fatalf("PlatformSkill.Get: %v", err)
		}
		return row.UpdatedAt
	}

	// missing=false 已是当前状态 → no-op，不得 bump。
	setSentinel()
	if err := r.MarkSkillFilesystemMissing(ctx, "cache-mark", false); err != nil {
		t.Fatalf("MarkSkillFilesystemMissing(false): %v", err)
	}
	if got := readRow(); got != sentinel {
		t.Fatalf("updated_at = %q, want sentinel %q (no-op mark must not write)", got, sentinel)
	}

	// missing=true 状态变化 → 必须写库并 bump。
	setSentinel()
	if err := r.MarkSkillFilesystemMissing(ctx, "cache-mark", true); err != nil {
		t.Fatalf("MarkSkillFilesystemMissing(true): %v", err)
	}
	if got := readRow(); got == sentinel {
		t.Fatal("updated_at kept sentinel, want bumped (missing transition must write)")
	}

	// 重复 missing=true（已为目标状态）→ no-op。
	setSentinel()
	if err := r.MarkSkillFilesystemMissing(ctx, "cache-mark", true); err != nil {
		t.Fatalf("MarkSkillFilesystemMissing(true) repeat: %v", err)
	}
	if got := readRow(); got != sentinel {
		t.Fatalf("updated_at = %q, want sentinel %q (repeated mark must not write)", got, sentinel)
	}

	// 不存在的 slug 仍须返回 NotFound（no-op 不得吞掉真正的缺失）。
	if err := r.MarkSkillFilesystemMissing(ctx, "no-such-slug", true); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("MarkSkillFilesystemMissing(unknown) err = %v, want NOT_FOUND", err)
	}
}

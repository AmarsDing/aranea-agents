package skill

import (
	"strings"
	"testing"
)

type mockRuntimeInvalidator struct{ calls int }

func (m *mockRuntimeInvalidator) InvalidateSkillRuntimeCache() { m.calls++ }

// P0：ToggleEnabled 成功后必须主动失效运行时缓存（DBRepositoryAdapter 快照），
// 否则已构建 Agent 要等快照 TTL（2min）才感知启用状态变化。
func TestToggleEnabled_InvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Status: "published", Enabled: false}
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	if _, err := u.ToggleEnabled(adminCtx(), "s1", true); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P0：删除 Skill 后必须失效运行时缓存，否则快照中的幽灵 Skill 仍可被 skill_load。
func TestDelete_InvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Status: "published", Enabled: true}
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	if err := u.Delete(adminCtx(), "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P0：版本回滚改变线上正文，必须失效运行时缓存中的已加载 body。
func TestRollbackVersion_InvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Status: "published", Enabled: true}
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	if _, err := u.RollbackVersion(adminCtx(), "s1", "v1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P0：磁盘同步导致内容变化时必须失效运行时缓存（正文快照）。
func TestUpsertSkillFromDisk_ContentChanged_InvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	if _, _, err := u.UpsertSkillFromDisk(adminCtx(), DiskSyncInput{Name: "a", Slug: "a", Body: "x"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P0：Patch 可改变 name/description/tags/body/triggers（均为路由信号），
// 必须失效运行时缓存，否则已发布且启用的 Skill 沿用陈旧快照。
func TestPatch_InvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Status: "published", Enabled: true}
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	if _, err := u.Patch(adminCtx(), "s1", UpdateDraft{HasName: true, Name: "New"}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P2-7a：发布即启用——Publish 成功后应自动 enabled=true 并失效运行时缓存，
// 消除「发布→再点启用」的两步反直觉操作。
func TestPublish_AutoEnablesAndInvalidatesRuntimeCache(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Name: "A", Status: "draft", Enabled: false, Description: "当需要处理报销流程时使用"}
	repo.markdown["s1"] = "# a\n\nbody"
	inv := &mockRuntimeInvalidator{}
	u := NewUsecase(repo, nil)
	u.SetRuntimeCacheInvalidator(inv)
	s, err := u.Publish(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !s.Enabled {
		t.Fatalf("expected skill auto-enabled after publish")
	}
	if enabled, ok := repo.updateEnabled["s1"]; !ok || !enabled {
		t.Fatalf("expected repo.UpdateSkillEnabled(s1, true), got %v", repo.updateEnabled)
	}
	if inv.calls != 1 {
		t.Fatalf("expected 1 runtime cache invalidation, got %d", inv.calls)
	}
}

// P2-7a：已启用的 Skill 重复发布（先回退 draft 再发）不应产生多余写。
func TestPublish_AlreadyEnabled_NoExtraEnableWrite(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Name: "A", Status: "draft", Enabled: true, Description: "当需要处理报销流程时使用"}
	repo.markdown["s1"] = "# a\n\nbody"
	u := NewUsecase(repo, nil)
	if _, err := u.Publish(adminCtx(), "s1"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, ok := repo.updateEnabled["s1"]; ok {
		t.Fatalf("expected no UpdateSkillEnabled write for already-enabled skill")
	}
}

// 长度合规的正文（>=40 runes），隔离长度类 warn，只验证 trigger-cue 逻辑。
const publishTestBody = "# A\n\n报销处理助手详细说明：审核发票、校验金额、提交打款并归档凭证，支持多级审批流配置与异常回退。\n"

// P1-4：description 无触发条件且 frontmatter 无 triggers 时，发布校验应给出
// warn（不 block），引导补全路由信号。
func TestPublishValidation_WarnsWhenMissingTriggerCue(t *testing.T) {
	s := Skill{Slug: "a", Name: "A", Description: "报销单处理助手，覆盖发票审核与打款流程"}
	status, _ := evaluatePublishValidation(s, publishTestBody)
	if status != "warn" {
		t.Fatalf("expected warn for description without trigger cue, got %q", status)
	}
}

// P1-4：frontmatter 已声明 triggers 时不再warn。
func TestPublishValidation_NoWarnWhenTriggersPresent(t *testing.T) {
	s := Skill{Slug: "a", Name: "A", Description: "报销单处理助手，覆盖发票审核与打款流程", Triggers: []string{"报销"}}
	status, _ := evaluatePublishValidation(s, publishTestBody)
	if status != "pass" {
		t.Fatalf("expected pass when triggers present, got %q", status)
	}
}

// P1-4：description 含触发条件 cue 时不再 warn。
func TestPublishValidation_NoWarnWhenDescriptionHasCue(t *testing.T) {
	s := Skill{Slug: "a", Name: "A", Description: "当需要处理报销流程时使用"}
	status, _ := evaluatePublishValidation(s, publishTestBody)
	if status != "pass" {
		t.Fatalf("expected pass when description has cue, got %q", status)
	}
}

// 防回归：warn 消息必须真正进入 Publish 后的 ValidationStatus。
func TestPublish_PersistsWarnValidationStatus(t *testing.T) {
	repo := newMockRepo()
	repo.skills["s1"] = Skill{ID: "s1", Slug: "a", Name: "A", Status: "draft", Description: "报销单处理助手，覆盖发票审核与打款流程"}
	repo.markdown["s1"] = publishTestBody
	u := NewUsecase(repo, nil)
	s, err := u.Publish(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if s.CurrentVersion == nil || s.CurrentVersion.ValidationStatus != "warn" {
		t.Fatalf("expected validation_status=warn, got %+v", s.CurrentVersion)
	}
}

// 防回归：block 优先于 trigger-cue warn。
func TestPublishValidation_BlockPrecedesTriggerCueWarn(t *testing.T) {
	s := Skill{Slug: "a", Name: "A", Description: "短"}
	status, msg := evaluatePublishValidation(s, "")
	if status != "block" {
		t.Fatalf("expected block, got %q", status)
	}
	if strings.Contains(msg, "触发条件") {
		t.Fatalf("block message should not mention trigger cue, got %q", msg)
	}
}

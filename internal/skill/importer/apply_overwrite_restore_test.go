package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
)

// overwrite 磁盘一致性（S1 修复）：applyOverwrite 先写盘再写库，
// 当 AppendImportedVersion 失败时必须把磁盘恢复为旧内容——
// 否则用户看到"导入失败已回滚"，但 watcher 会把磁盘新内容同步成新版本，
// 使"失败的导入"实际生效。旧实现下本测试必然失败（磁盘残留新内容）。

func TestApplyImport_overwriteRestoresDiskOnDBFailure(t *testing.T) {
	repo := &stubSkillRepo{
		byKeySkill:    skill.Skill{ID: "skill-existing", Slug: "dup-skill"},
		failOnAppend:  true,
	}
	eng, root := setupEngineWithTempRoot(t, repo)

	// 已有 skill 的磁盘目录与旧内容。
	existingDir := filepath.Join(root, "dup-skill")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldBody := []byte("# Old Body")
	if err := os.WriteFile(filepath.Join(existingDir, "SKILL.md"), oldBody, 0o644); err != nil {
		t.Fatalf("write old SKILL.md: %v", err)
	}
	repo.storageDirs = map[string]string{"skill-existing": existingDir}

	seedJob(eng, "job-overwrite", biz.SkillImportCandidate{
		CandidateID:      "c1",
		Name:             "New Body",
		Slug:             "dup-skill",
		ValidationStatus: "block",
		Blocks:           []biz.SkillImportIssue{{Type: "duplicate_name", Message: "same slug"}},
	})

	_, err := eng.ApplyImport(context.Background(), "job-overwrite", biz.SkillImportApplyRequest{
		Decisions: []biz.SkillImportDecision{
			{Action: "overwrite_duplicate", CandidateID: "c1"},
		},
	})
	if err == nil {
		t.Fatal("expected AppendImportedVersion failure, got nil")
	}

	got, readErr := os.ReadFile(filepath.Join(existingDir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("read SKILL.md after failed overwrite: %v", readErr)
	}
	if string(got) != string(oldBody) {
		t.Fatalf("disk content = %q after failed overwrite, want restored old body %q", got, oldBody)
	}
}

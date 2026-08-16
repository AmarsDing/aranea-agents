package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	skillstorage "aranea-agents/internal/skill/storage"
	"aranea-agents/pkg/loggateway"
)

// syncSkillBodyToDisk best-effort 将 body 写入 skill 磁盘目录的 SKILL.md。
// 失败只记日志不返回错误——UpsertSkillFromDisk 的进化版保护会在下次扫描时
// 以 DB 为准重新收敛磁盘。无磁盘载体（storage_dir 未配置）的 skill 直接跳过。
func (r *skillRepo) syncSkillBodyToDisk(ctx context.Context, skillID, body string) {
	dir, err := r.GetSkillStorageDir(ctx, skillID)
	if err != nil {
		return
	}
	writeSkillBodyToDisk(r.data.lg, dir, body)
}

func writeSkillBodyToDisk(lg loggateway.Logger, dir, body string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		lg.Warn("skill disk sync: mkdir failed", loggateway.StepID("data.skill"), loggateway.Str("dir", dir), loggateway.Err(err))
		return
	}
	// P1-3：原子写——回写中途崩溃不得留下截断的 SKILL.md（watcher 会把
	// 截断文件当损坏包拒绝，进化版保护的收敛循环会被误打断）。
	if err := skillstorage.AtomicWriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		lg.Warn("skill disk sync: write SKILL.md failed", loggateway.StepID("data.skill"), loggateway.Str("dir", dir), loggateway.Err(err))
	}
}

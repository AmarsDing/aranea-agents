package skill

import (
	"context"
	"os"
	"path/filepath"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type FileSystemSkillRegistrar struct {
	agentRootDir func(agentID string) string
	lg           loggateway.Logger
}

func NewFileSystemSkillRegistrar(agentRootDir func(agentID string) string, lg loggateway.Logger) *FileSystemSkillRegistrar {
	return &FileSystemSkillRegistrar{agentRootDir: agentRootDir, lg: lg}
}

func (r *FileSystemSkillRegistrar) RegisterSkill(ctx context.Context, agentID string, name string, skillMD string) error {
	if r.agentRootDir == nil {
		return kerrors.InternalServer("SKILL_EVO", "agent root dir resolver is nil")
	}
	root := r.agentRootDir(agentID)
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return kerrors.InternalServer("SKILL_EVO", "create skill dir: "+err.Error())
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillMD), 0o644); err != nil {
		return kerrors.InternalServer("SKILL_EVO", "write SKILL.md: "+err.Error())
	}
	r.lg.Info("skill registered",
		loggateway.StepID("skill.register"),
		loggateway.Str("agent_id", agentID),
		loggateway.Str("skill_name", name),
	)
	return nil
}

func (r *FileSystemSkillRegistrar) SkillExists(ctx context.Context, agentID string, name string) (bool, error) {
	if r.agentRootDir == nil {
		return false, kerrors.InternalServer("SKILL_EVO", "agent root dir resolver is nil")
	}
	root := r.agentRootDir(agentID)
	skillPath := filepath.Join(root, name, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, kerrors.InternalServer("SKILL_EVO", "stat skill: "+err.Error())
	}
	return true, nil
}

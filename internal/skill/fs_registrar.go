package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
		return apierror.Internal(apierror.DomainSkill, "agent root dir resolver is nil")
	}
	if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return apierror.BadRequest(apierror.DomainSkill, "skill name contains invalid path characters")
	}
	root := r.agentRootDir(agentID)
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return apierror.Internal(apierror.DomainSkill, "create skill dir").WithCause(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillMD), 0o644); err != nil {
		return apierror.Internal(apierror.DomainSkill, "write SKILL.md").WithCause(err)
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
		return false, apierror.Internal(apierror.DomainSkill, "agent root dir resolver is nil")
	}
	root := r.agentRootDir(agentID)
	skillPath := filepath.Join(root, name, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, apierror.Internal(apierror.DomainSkill, "stat skill").WithCause(err)
	}
	return true, nil
}

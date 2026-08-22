package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/agent/reposkills"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/manifest"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// workspaceSkillRepo overlays trusted-workspace SKILL.md trees onto the
// platform skill repository so skill_load / $slug can read repo-local
// manuals without importing them into the DB (F3).
type workspaceSkillRepo struct {
	inner trpcskill.Repository
	cwd   string
	roots []string
}

func wrapWorkspaceSkillsRepo(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, inner trpcskill.Repository) trpcskill.Repository {
	if inner == nil || !ShouldAttachWorkingContract(ag) {
		return inner
	}
	cwd, err := resolveToolWorkspaceRoot(ctx, ag, deps, "")
	if err != nil || cwd == "" {
		return inner
	}
	return &workspaceSkillRepo{inner: inner, cwd: cwd, roots: []string{cwd}}
}

func workspaceSkillEntries(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) []reposkills.Entry {
	if !ShouldAttachWorkingContract(ag) {
		return nil
	}
	cwd, err := resolveToolWorkspaceRoot(ctx, ag, deps, "")
	if err != nil || cwd == "" {
		return nil
	}
	return reposkills.Scan(cwd, []string{cwd})
}

func (r *workspaceSkillRepo) entries() []reposkills.Entry {
	if r == nil {
		return nil
	}
	return reposkills.Scan(r.cwd, r.roots)
}

func (r *workspaceSkillRepo) Summaries() []trpcskill.Summary {
	out := r.inner.Summaries()
	return mergeWorkspaceSummaries(out, r.entries())
}

func (r *workspaceSkillRepo) Get(name string) (*trpcskill.Skill, error) {
	if sk, err := r.inner.Get(name); err == nil {
		return sk, nil
	}
	return r.getWorkspace(name)
}

func (r *workspaceSkillRepo) Path(name string) (string, error) {
	if p, err := r.inner.Path(name); err == nil {
		return p, nil
	}
	e, ok := reposkills.Lookup(r.entries(), name)
	if !ok || strings.TrimSpace(e.Dir) == "" {
		return "", fmt.Errorf("skill %q not found", name)
	}
	return e.Dir, nil
}

func (r *workspaceSkillRepo) SummariesForContext(ctx context.Context) []trpcskill.Summary {
	return mergeWorkspaceSummaries(trpcskill.SummariesForContext(ctx, r.inner), r.entries())
}

func (r *workspaceSkillRepo) GetForContext(ctx context.Context, name string) (*trpcskill.Skill, error) {
	if sk, err := trpcskill.GetForContext(ctx, r.inner, name); err == nil {
		return sk, nil
	}
	return r.getWorkspace(name)
}

func (r *workspaceSkillRepo) PathForContext(ctx context.Context, name string) (string, error) {
	if p, err := trpcskill.PathForContext(ctx, r.inner, name); err == nil {
		return p, nil
	}
	return r.Path(name)
}

func (r *workspaceSkillRepo) getWorkspace(name string) (*trpcskill.Skill, error) {
	e, ok := reposkills.Lookup(r.entries(), name)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	body, ok := readWorkspaceSkillMD(e.Dir)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	m := manifest.Parse(body)
	sum := trpcskill.Summary{
		Name:        e.Slug,
		Description: e.Description,
	}
	if sum.Description == "" {
		sum.Description = strings.TrimSpace(m.Description)
	}
	return &trpcskill.Skill{Summary: sum, Body: strings.TrimSpace(m.Body)}, nil
}

func mergeWorkspaceSummaries(inner []trpcskill.Summary, entries []reposkills.Entry) []trpcskill.Summary {
	seen := make(map[string]bool, len(inner)+len(entries))
	out := make([]trpcskill.Summary, 0, len(inner)+len(entries))
	for _, s := range inner {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, e := range entries {
		key := strings.ToLower(strings.TrimSpace(e.Slug))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		desc := strings.TrimSpace(e.Description)
		if desc == "" {
			desc = strings.TrimSpace(e.Name)
		}
		if desc != "" {
			desc += " (workspace)"
		} else {
			desc = "(workspace)"
		}
		out = append(out, trpcskill.Summary{Name: e.Slug, Description: desc})
	}
	return out
}

func readWorkspaceSkillMD(dir string) (string, bool) {
	for _, name := range []string{"SKILL.md", "skill.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(data) == 0 {
			continue
		}
		return string(data), true
	}
	return "", false
}

var _ trpcskill.Repository = (*workspaceSkillRepo)(nil)
var _ trpcskill.ContextRepository = (*workspaceSkillRepo)(nil)

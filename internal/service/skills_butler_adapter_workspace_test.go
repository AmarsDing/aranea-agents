package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// E2E-E2：butler 注册技能的 workspace 归属 fail-closed。
// system 旁路上下文下的注册等于落 workspace_id 空串（全平台共享、经
// ListEnabledPublishedSkillCandidates 全租户可路由），必须拒绝而非放行。

func TestButlerSkillWorkspace_TenantCtxReturned(t *testing.T) {
	ctx := workspace.WithContext(context.Background(), "ws-tenant-a")
	got, err := butlerSkillWorkspace(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws-tenant-a" {
		t.Fatalf("workspace = %q, want ws-tenant-a", got)
	}
}

func TestButlerSkillWorkspace_NoCtxFallsBackToDefault(t *testing.T) {
	got, err := butlerSkillWorkspace(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != workspace.DefaultWorkspaceID {
		t.Fatalf("workspace = %q, want %q (单租户部署兼容回落)", got, workspace.DefaultWorkspaceID)
	}
}

func TestButlerSkillWorkspace_SystemCtxForbidden(t *testing.T) {
	ctx := workspace.WithSystemWorkspace(context.Background())
	if _, err := butlerSkillWorkspace(ctx); err == nil {
		t.Fatal("system workspace ctx must be refused — shared skill registration is a cross-tenant leak vector")
	} else if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeForbidden {
		t.Fatalf("error = %v, want FORBIDDEN", err)
	}
}

// RegisterSkill 全路径：system ctx 在到达 uc.Create 之前即被拒（uc 带 nil
// repo 也不允许被触达——若 Create 先跑会在取 repo 时 panic）。
func TestRegisterSkill_SystemCtxForbiddenBeforeCreate(t *testing.T) {
	uc := biz.NewSkillUsecase(nil, nil, nil)
	reg := NewSkillsButlerRegistrationAdapter(uc)
	ctx := workspace.WithSystemWorkspace(context.Background())
	err := reg.RegisterSkill(ctx, "agent-1", "tenant-secret-skill", "# S\n\nbody")
	if err == nil {
		t.Fatal("RegisterSkill under system workspace must fail closed")
	}
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeForbidden {
		t.Fatalf("error = %v, want FORBIDDEN", err)
	}
}

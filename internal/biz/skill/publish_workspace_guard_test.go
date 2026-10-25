package skill

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	authpkg "aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// E2E-E2：publish fail-closed ——发布是可见性跃迁（published+enabled ⇒ 经
// ListEnabledPublishedSkillCandidates 进入全租户运行时路由面），必须经
// AssertWorkspaceMutate 门禁：非属主租户拒绝；共享草稿（workspace_id 空串，
// 含历史 fail-open 注册遗留行）禁止租户发布、仅 system 旁可流转。

func adminTenantCtxForPublish(wsID string) context.Context {
	return workspace.WithContext(
		authpkg.NewContext(context.Background(), &authpkg.Auth{UserID: 1, Access: "admin"}), wsID)
}

func seedPublishableDraft(r *mockRepo, wsID string) {
	r.skills["s1"] = Skill{ID: "s1", Slug: "a", Name: "A", Status: "draft",
		Description: "报销处理助手详细说明", WorkspaceID: wsID}
	r.markdown["s1"] = publishTestBody
}

func TestPublish_RequiresOwningWorkspace(t *testing.T) {
	cases := []struct {
		name     string
		callerWS string
		skillWS  string
		wantErr  bool
	}{
		{"owning tenant publishes own draft", "ws-a", "ws-a", false},
		{"other tenant cannot publish", "ws-a", "ws-b", true},
		{"tenant cannot publish shared draft", "ws-a", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newMockRepo()
			seedPublishableDraft(r, tc.skillWS)
			u := NewUsecase(r, nil, loggateway.NewNoop())
			_, err := u.Publish(adminTenantCtxForPublish(tc.callerWS), "s1")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected Forbidden for non-owner publish")
				}
				if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeForbidden {
					t.Fatalf("error = %v, want FORBIDDEN", err)
				}
				if got := r.skills["s1"].Status; got != "draft" {
					t.Fatalf("status = %q, want draft (blocked publish must not mutate)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPublish_SystemCtxMayPublishSharedDraft(t *testing.T) {
	r := newMockRepo()
	seedPublishableDraft(r, "")
	u := NewUsecase(r, nil, loggateway.NewNoop())
	ctx := workspace.WithSystemWorkspace(
		authpkg.NewContext(context.Background(), &authpkg.Auth{UserID: 1, Access: "admin"}))
	s, err := u.Publish(ctx, "s1")
	if err != nil {
		t.Fatalf("system ctx must keep the shared-draft publish path (cron/admin 内置流转), got %v", err)
	}
	if s.Status != "published" {
		t.Fatalf("status = %q, want published", s.Status)
	}
}

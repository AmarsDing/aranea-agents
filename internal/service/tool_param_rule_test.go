package service

import (
	"context"
	"sort"
	"testing"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// fakeParamRuleStore is an in-memory biztool.ToolParamRuleStore for service tests.
type fakeParamRuleStore struct {
	rules map[string]biztool.ToolParamRule // by ID
}

func newFakeParamRuleStore() *fakeParamRuleStore {
	return &fakeParamRuleStore{rules: make(map[string]biztool.ToolParamRule)}
}

func (f *fakeParamRuleStore) list(_ context.Context, toolKey string, enabledOnly bool) []biztool.ToolParamRule {
	var out []biztool.ToolParamRule
	for _, r := range f.rules {
		if r.ToolKey != toolKey {
			continue
		}
		if enabledOnly && !r.Enabled {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (f *fakeParamRuleStore) ListEnabledParamRules(ctx context.Context, toolKey string) ([]biztool.ToolParamRule, error) {
	return f.list(ctx, toolKey, true), nil
}

func (f *fakeParamRuleStore) ListParamRules(ctx context.Context, toolKey string) ([]biztool.ToolParamRule, error) {
	return f.list(ctx, toolKey, false), nil
}

func (f *fakeParamRuleStore) GetParamRuleByID(_ context.Context, id string) (biztool.ToolParamRule, error) {
	r, ok := f.rules[id]
	if !ok {
		return biztool.ToolParamRule{}, shared.ErrNotFound
	}
	return r, nil
}

func (f *fakeParamRuleStore) UpsertParamRule(_ context.Context, rule biztool.ToolParamRule) error {
	f.rules[rule.ID] = rule
	return nil
}

func (f *fakeParamRuleStore) DeleteParamRule(_ context.Context, id string) error {
	delete(f.rules, id)
	return nil
}

func newParamRuleTestService(store *fakeParamRuleStore) *ToolService {
	uc := biztool.NewToolUsecase(nil, nil, loggateway.NewNoop(), biztool.WithToolParamRuleStore(store))
	agents := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   &batchAgentReader{agents: map[string]biz.Agent{"agent-1": {ID: "agent-1"}}},
		Settings: batchSettingsRepo{},
		Files:    batchFilesRepo{},
		Lg:       loggateway.NewNoop(),
	})
	return NewToolService(uc, agents, nil)
}

// paramRuleAdminCtx 注入 admin claims（P5.4 H4：参数规则端点仅管理员可达）。
func paramRuleAdminCtx() context.Context {
	return auth.NewContext(toolGrantCtx(), &auth.Auth{UserID: 1, Access: "admin"})
}

// TestToolService_ParamRuleRPCs_RequireAdmin 钉死 H4：无 claims → Unauthorized，
// 非 admin claims → Forbidden；三个端点同标准。
func TestToolService_ParamRuleRPCs_RequireAdmin(t *testing.T) {
	svc := newParamRuleTestService(newFakeParamRuleStore())
	noAuth := toolGrantCtx()
	userAuth := auth.NewContext(toolGrantCtx(), &auth.Auth{UserID: 2, Access: "user"})

	if _, err := svc.ListToolParamRules(noAuth, &v1.ListToolParamRulesRequest{ToolKey: "shell"}); err != auth.ErrUnauthorized {
		t.Fatalf("list no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ListToolParamRules(userAuth, &v1.ListToolParamRulesRequest{ToolKey: "shell"}); err != auth.ErrForbidden {
		t.Fatalf("list user err = %v, want ErrForbidden", err)
	}
	up := &v1.UpsertToolParamRuleRequest{Id: "r1", ToolKey: "exec_command", Pattern: "ls *", Effect: "allow", Enabled: true}
	if _, err := svc.UpsertToolParamRule(noAuth, up); err != auth.ErrUnauthorized {
		t.Fatalf("upsert no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.UpsertToolParamRule(userAuth, up); err != auth.ErrForbidden {
		t.Fatalf("upsert user err = %v, want ErrForbidden", err)
	}
	del := &v1.DeleteToolParamRuleRequest{Id: "r1"}
	if _, err := svc.DeleteToolParamRule(noAuth, del); err != auth.ErrUnauthorized {
		t.Fatalf("delete no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.DeleteToolParamRule(userAuth, del); err != auth.ErrForbidden {
		t.Fatalf("delete user err = %v, want ErrForbidden", err)
	}
}

func TestToolService_ListToolParamRules_AliasCanonicalized(t *testing.T) {
	store := newFakeParamRuleStore()
	if err := store.UpsertParamRule(context.Background(), biztool.ToolParamRule{
		ID: "r1", ToolKey: "exec_command", Pattern: "rm -rf /*", Effect: "deny", Priority: 10, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	svc := newParamRuleTestService(store)
	// "shell" 经别名链 shell → shell_exec → exec_command 归一，命中同一行集合。
	resp, err := svc.ListToolParamRules(paramRuleAdminCtx(), &v1.ListToolParamRulesRequest{ToolKey: "shell"})
	if err != nil {
		t.Fatalf("ListToolParamRules err = %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetId() != "r1" {
		t.Fatalf("items = %+v, want [r1]", resp.GetItems())
	}
}

func TestToolService_ListToolParamRules_RequiresToolKey(t *testing.T) {
	svc := newParamRuleTestService(newFakeParamRuleStore())
	if _, err := svc.ListToolParamRules(paramRuleAdminCtx(), &v1.ListToolParamRulesRequest{}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty tool_key err = %v, want BadRequest", err)
	}
}

func TestToolService_UpsertToolParamRule_SuccessCanonicalizes(t *testing.T) {
	store := newFakeParamRuleStore()
	svc := newParamRuleTestService(store)
	got, err := svc.UpsertToolParamRule(paramRuleAdminCtx(), &v1.UpsertToolParamRuleRequest{
		Id: "r1", ToolKey: "shell", Pattern: "sudo *", Effect: "ask", Priority: 50, Enabled: true,
	})
	if err != nil {
		t.Fatalf("UpsertToolParamRule err = %v", err)
	}
	if got.GetToolKey() != "exec_command" {
		t.Fatalf("response tool_key = %q, want exec_command", got.GetToolKey())
	}
	stored := store.rules["r1"]
	if stored.ToolKey != "exec_command" || stored.Pattern != "sudo *" || stored.Effect != "ask" {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.CreatedAt == 0 {
		t.Fatal("CreatedAt must be stamped by biz layer")
	}
}

func TestToolService_UpsertToolParamRule_Validation(t *testing.T) {
	svc := newParamRuleTestService(newFakeParamRuleStore())
	cases := []struct {
		name string
		req  *v1.UpsertToolParamRuleRequest
	}{
		{"missing fields", &v1.UpsertToolParamRuleRequest{Id: "r1"}},
		{"bad effect", &v1.UpsertToolParamRuleRequest{Id: "r1", ToolKey: "exec_command", Pattern: "ls *", Effect: "block"}},
		{"bad regex pattern", &v1.UpsertToolParamRuleRequest{Id: "r1", ToolKey: "exec_command", Pattern: "re:([", Effect: "deny"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.UpsertToolParamRule(paramRuleAdminCtx(), tc.req); !apierror.IsCode(err, apierror.CodeBadRequest) {
				t.Fatalf("err = %v, want BadRequest", err)
			}
		})
	}
}

func TestToolService_UpsertToolParamRule_BuiltinEffectReadOnly(t *testing.T) {
	store := newFakeParamRuleStore()
	if err := store.UpsertParamRule(context.Background(), biztool.ToolParamRule{
		ID: "builtin-exec-deny-rmrf", ToolKey: "exec_command", Pattern: "rm -rf /*", Effect: "deny", Priority: 10, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	svc := newParamRuleTestService(store)
	// effect 变更 → BadRequest。
	if _, err := svc.UpsertToolParamRule(paramRuleAdminCtx(), &v1.UpsertToolParamRuleRequest{
		Id: "builtin-exec-deny-rmrf", ToolKey: "exec_command", Pattern: "rm -rf /*", Effect: "allow", Priority: 10, Enabled: true,
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("effect change err = %v, want BadRequest", err)
	}
	// 同 effect 改 priority/enabled → 允许。
	if _, err := svc.UpsertToolParamRule(paramRuleAdminCtx(), &v1.UpsertToolParamRuleRequest{
		Id: "builtin-exec-deny-rmrf", ToolKey: "exec_command", Pattern: "rm -rf /*", Effect: "deny", Priority: 20, Enabled: false,
	}); err != nil {
		t.Fatalf("non-effect update err = %v", err)
	}
	if got := store.rules["builtin-exec-deny-rmrf"]; got.Priority != 20 || got.Enabled {
		t.Fatalf("after update = %+v", got)
	}
}

func TestToolService_DeleteToolParamRule(t *testing.T) {
	store := newFakeParamRuleStore()
	if err := store.UpsertParamRule(context.Background(), biztool.ToolParamRule{
		ID: "r1", ToolKey: "exec_command", Pattern: "sudo *", Effect: "ask", Priority: 50, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertParamRule(context.Background(), biztool.ToolParamRule{
		ID: "builtin-gns3-allow-show", ToolKey: "gns3_exec", Pattern: "show *", Effect: "allow", Priority: 10, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	svc := newParamRuleTestService(store)
	// builtin 禁删。
	if _, err := svc.DeleteToolParamRule(paramRuleAdminCtx(), &v1.DeleteToolParamRuleRequest{Id: "builtin-gns3-allow-show"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("builtin delete err = %v, want BadRequest", err)
	}
	// 自定义删除 + 幂等。
	if _, err := svc.DeleteToolParamRule(paramRuleAdminCtx(), &v1.DeleteToolParamRuleRequest{Id: "r1"}); err != nil {
		t.Fatalf("delete err = %v", err)
	}
	if _, ok := store.rules["r1"]; ok {
		t.Fatal("r1 must be deleted")
	}
	if _, err := svc.DeleteToolParamRule(paramRuleAdminCtx(), &v1.DeleteToolParamRuleRequest{Id: "r1"}); err != nil {
		t.Fatalf("second delete err = %v", err)
	}
}

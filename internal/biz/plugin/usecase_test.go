package plugin

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestUsecase_List_DefaultLimit(t *testing.T) {
	var captured ListQuery
	repo := noOpRepo()
	repo.searchFn = func(_ context.Context, q ListQuery) (ListResult, error) {
		captured = q
		return ListResult{}, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.List(context.Background(), ListQuery{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Limit != 20 {
		t.Errorf("Limit = %d, want 20", captured.Limit)
	}
}

func TestUsecase_List_MaxLimit(t *testing.T) {
	var captured ListQuery
	repo := noOpRepo()
	repo.searchFn = func(_ context.Context, q ListQuery) (ListResult, error) {
		captured = q
		return ListResult{}, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.List(context.Background(), ListQuery{Limit: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Limit != 100 {
		t.Errorf("Limit = %d, want 100", captured.Limit)
	}
}

func TestUsecase_List_NegativeOffset(t *testing.T) {
	var captured ListQuery
	repo := noOpRepo()
	repo.searchFn = func(_ context.Context, q ListQuery) (ListResult, error) {
		captured = q
		return ListResult{}, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.List(context.Background(), ListQuery{Limit: 10, Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Offset != 0 {
		t.Errorf("Offset = %d, want 0", captured.Offset)
	}
}

func TestUsecase_ToggleEnabled_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.ToggleEnabled(context.Background(), "", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_GetByKey_EmptyKey(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.GetByKey(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_Create_EmptyKey(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{Key: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_Create_GeneratesID(t *testing.T) {
	var captured Plugin
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, p Plugin) (Plugin, error) {
		captured = p
		return p, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{Key: "my-plugin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "builtin-my-plugin"
	if captured.ID != want {
		t.Errorf("ID = %q, want %q", captured.ID, want)
	}
}

func TestUsecase_Create_DefaultConfigJSON(t *testing.T) {
	var captured Plugin
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, p Plugin) (Plugin, error) {
		captured = p
		return p, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{Key: "k"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ConfigJSON != "{}" {
		t.Errorf("ConfigJSON = %q, want %q", captured.ConfigJSON, "{}")
	}
}

func TestUsecase_Create_InvalidConfigJSON(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{Key: "k", ConfigJSON: "{invalid"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_Create_DefaultScope(t *testing.T) {
	var captured Plugin
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, p Plugin) (Plugin, error) {
		captured = p
		return p, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{Key: "k"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Scope != "global" {
		t.Errorf("Scope = %q, want %q", captured.Scope, "global")
	}
}

func TestUsecase_Create_ValidatesSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, p Plugin) (Plugin, error) {
		return p, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.Create(context.Background(), Plugin{
		Key:              "k",
		ConfigSchemaJSON: schema,
		ConfigJSON:       `{"age":1}`,
	})
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateConfig_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateConfig(context.Background(), "", `{"k":"v"}`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateConfig_EmptyConfig(t *testing.T) {
	var capturedCfg string
	repo := noOpRepo()
	repo.getFn = func(_ context.Context, id string) (Plugin, error) {
		return Plugin{ID: id, ConfigSchemaJSON: ""}, nil
	}
	repo.updateCfgFn = func(_ context.Context, id string, configJSON string) (Plugin, error) {
		capturedCfg = configJSON
		return Plugin{}, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateConfig(context.Background(), "p1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCfg != "{}" {
		t.Errorf("configJSON = %q, want %q", capturedCfg, "{}")
	}
}

func TestUsecase_UpdateConfig_InvalidJSON(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateConfig(context.Background(), "p1", "{bad")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateSortOrder_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateSortOrder(context.Background(), "", 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateScope_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateScope(context.Background(), "", "agent-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateScope_DefaultScope(t *testing.T) {
	var capturedScope string
	repo := noOpRepo()
	repo.updateScFn = func(_ context.Context, id string, scope string) (Plugin, error) {
		capturedScope = scope
		return Plugin{}, nil
	}
	u := NewUsecase(repo, noOpRunRepo(), noOpScopeAgentLookup())
	_, err := u.UpdateScope(context.Background(), "p1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedScope != "global" {
		t.Errorf("scope = %q, want %q", capturedScope, "global")
	}
}

func TestUsecase_RecordRun_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.RecordRun(context.Background(), Run{})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestUsecase_ListRuns_NilUsecase(t *testing.T) {
	var u *Usecase
	result, err := u.ListRuns(context.Background(), RunQuery{})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(result.Items))
	}
}

func TestUsecase_ListRuns_DefaultLimit(t *testing.T) {
	var captured RunQuery
	runs := noOpRunRepo()
	runs.listFn = func(_ context.Context, q RunQuery) (RunListResult, error) {
		captured = q
		return RunListResult{}, nil
	}
	u := NewUsecase(noOpRepo(), runs, noOpScopeAgentLookup())
	_, err := u.ListRuns(context.Background(), RunQuery{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Limit != 50 {
		t.Errorf("Limit = %d, want 50", captured.Limit)
	}
}

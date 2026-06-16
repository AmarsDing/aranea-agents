package plugin

import (
	"context"
	"aranea-agents/internal/biz/shared"
	"testing"

	"aranea-agents/pkg/apierror"
)

type mockRepo struct {
	searchFn   func(ctx context.Context, q ListQuery) (ListResult, error)
	getFn      func(ctx context.Context, id string) (Plugin, error)
	getByKeyFn func(ctx context.Context, key string) (Plugin, error)
	createFn   func(ctx context.Context, p Plugin) (Plugin, error)
	updateEnFn func(ctx context.Context, id string, enabled bool) (Plugin, error)
	updateCfgFn func(ctx context.Context, id string, configJSON string) (Plugin, error)
	updateSOFn func(ctx context.Context, id string, sortOrder int) (Plugin, error)
	updateScFn func(ctx context.Context, id string, scope string) (Plugin, error)
	incrStatFn func(ctx context.Context, pluginKey string, delta StatUpdate) error
}

func (m *mockRepo) SearchPlugins(ctx context.Context, q ListQuery) (ListResult, error) {
	return m.searchFn(ctx, q)
}
func (m *mockRepo) GetPlugin(ctx context.Context, id string) (Plugin, error) {
	return m.getFn(ctx, id)
}
func (m *mockRepo) GetByKey(ctx context.Context, key string) (Plugin, error) {
	return m.getByKeyFn(ctx, key)
}
func (m *mockRepo) CreatePlugin(ctx context.Context, p Plugin) (Plugin, error) {
	return m.createFn(ctx, p)
}
func (m *mockRepo) UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error) {
	return m.updateEnFn(ctx, id, enabled)
}
func (m *mockRepo) UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error) {
	return m.updateCfgFn(ctx, id, configJSON)
}
func (m *mockRepo) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error) {
	return m.updateSOFn(ctx, id, sortOrder)
}
func (m *mockRepo) UpdatePluginScope(ctx context.Context, id string, scope string) (Plugin, error) {
	return m.updateScFn(ctx, id, scope)
}
func (m *mockRepo) IncrementStats(ctx context.Context, pluginKey string, delta StatUpdate) error {
	return m.incrStatFn(ctx, pluginKey, delta)
}

type mockRunRepo struct {
	insertFn   func(ctx context.Context, run Run) error
	listFn     func(ctx context.Context, q RunQuery) (RunListResult, error)
	deleteAllFn func(ctx context.Context) (int32, error)
}

func (m *mockRunRepo) Insert(ctx context.Context, run Run) error  { return m.insertFn(ctx, run) }
func (m *mockRunRepo) List(ctx context.Context, q RunQuery) (RunListResult, error) {
	return m.listFn(ctx, q)
}
func (m *mockRunRepo) DeleteAll(ctx context.Context) (int32, error) { return m.deleteAllFn(ctx) }

type mockScopeAgentLookup struct {
	agentExistsFn func(ctx context.Context, id string) error
}

func (m *mockScopeAgentLookup) AgentExists(ctx context.Context, id string) error {
	return m.agentExistsFn(ctx, id)
}

func noOpRepo() *mockRepo {
	return &mockRepo{
		searchFn:   func(_ context.Context, _ ListQuery) (ListResult, error) { return ListResult{}, nil },
		getFn:      func(_ context.Context, id string) (Plugin, error) { return Plugin{ID: id}, nil },
		getByKeyFn: func(_ context.Context, key string) (Plugin, error) { return Plugin{Key: key}, nil },
		createFn:   func(_ context.Context, p Plugin) (Plugin, error) { return p, nil },
		updateEnFn: func(_ context.Context, id string, enabled bool) (Plugin, error) { return Plugin{ID: id, Enabled: enabled}, nil },
		updateCfgFn: func(_ context.Context, id string, configJSON string) (Plugin, error) { return Plugin{ID: id, ConfigJSON: configJSON}, nil },
		updateSOFn: func(_ context.Context, id string, sortOrder int) (Plugin, error) { return Plugin{ID: id, SortOrder: sortOrder}, nil },
		updateScFn: func(_ context.Context, id string, scope string) (Plugin, error) { return Plugin{ID: id, Scope: scope}, nil },
		incrStatFn: func(_ context.Context, _ string, _ StatUpdate) error { return nil },
	}
}

func noOpRunRepo() *mockRunRepo {
	return &mockRunRepo{
		insertFn:   func(_ context.Context, _ Run) error { return nil },
		listFn:     func(_ context.Context, _ RunQuery) (RunListResult, error) { return RunListResult{}, nil },
		deleteAllFn: func(_ context.Context) (int32, error) { return 0, nil },
	}
}

func noOpScopeAgentLookup() *mockScopeAgentLookup {
	return &mockScopeAgentLookup{
		agentExistsFn: func(_ context.Context, _ string) error { return nil },
	}
}

func TestUsecase_List(t *testing.T) {
	tests := []struct {
		name      string
		q         ListQuery
		wantLimit int
		wantOff   int
	}{
		{"zero limit defaults to 20", ListQuery{Limit: 0, Offset: 0}, 20, 0},
		{"negative limit defaults to 20", ListQuery{Limit: -1, Offset: 0}, 20, 0},
		{"limit capped at 100", ListQuery{Limit: 200, Offset: 0}, 100, 0},
		{"negative offset normalized to 0", ListQuery{Limit: 10, Offset: -5}, 10, 0},
		{"valid limit and offset preserved", ListQuery{Limit: 50, Offset: 25}, 50, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured ListQuery
			mr := noOpRepo()
			mr.searchFn = func(_ context.Context, q ListQuery) (ListResult, error) {
				captured = q
				return ListResult{}, nil
			}
			u := NewUsecase(mr, noOpRunRepo(), noOpScopeAgentLookup())
			_, err := u.List(context.Background(), tt.q)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if captured.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", captured.Limit, tt.wantLimit)
			}
			if captured.Offset != tt.wantOff {
				t.Errorf("Offset = %d, want %d", captured.Offset, tt.wantOff)
			}
		})
	}
}

func TestUsecase_ToggleEnabled(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   bool
		errReason string
	}{
		{"empty id rejected", "", true, "PLUGIN"},
		{"whitespace id rejected", "  ", true, "PLUGIN"},
		{"valid id passes", "p-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUsecase(noOpRepo(), noOpRunRepo(), noOpScopeAgentLookup())
			_, err := u.ToggleEnabled(context.Background(), tt.id, true)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e, ok := apierror.From(err); ok && e.Domain != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Domain, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_GetByKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		repoFn    func(_ context.Context, key string) (Plugin, error)
		wantErr   bool
		errReason string
	}{
		{
			"empty key rejected",
			"",
			nil, true, "PLUGIN",
		},
		{
			"whitespace key rejected",
			"  ",
			nil, true, "PLUGIN",
		},
		{
			"key trimmed and passed",
			"  my-key  ",
			func(_ context.Context, key string) (Plugin, error) {
				return Plugin{Key: key}, nil
			},
			false, "",
		},
		{
			"sql err no rows",
			"missing",
			func(_ context.Context, _ string) (Plugin, error) {
				return Plugin{}, shared.ErrNotFound
			},
			true, "",
		},
		{
			"other repo error",
			"bad",
			func(_ context.Context, _ string) (Plugin, error) {
				return Plugin{}, apierror.Internal("PLUGIN", "db error")
			},
			true, "PLUGIN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.repoFn != nil {
				mr.getByKeyFn = tt.repoFn
			}
			u := NewUsecase(mr, noOpRunRepo(), noOpScopeAgentLookup())
			_, err := u.GetByKey(context.Background(), tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e, ok := apierror.From(err); ok && e.Domain != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Domain, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_Create(t *testing.T) {
	tests := []struct {
		name      string
		in        Plugin
		repoFn    func(_ context.Context, p Plugin) (Plugin, error)
		wantErr   bool
		errReason string
		check     func(t *testing.T, got Plugin)
	}{
		{
			"empty key rejected",
			Plugin{Name: "test"},
			nil, true, "PLUGIN", nil,
		},
		{
			"whitespace key rejected",
			Plugin{Key: "  ", Name: "test"},
			nil, true, "PLUGIN", nil,
		},
		{
			"auto id and default config",
			Plugin{Key: "my-plugin"},
			func(_ context.Context, p Plugin) (Plugin, error) { return p, nil },
			false, "",
			func(t *testing.T, got Plugin) {
				if got.ID != "builtin-my-plugin" {
					t.Errorf("ID = %q, want %q", got.ID, "builtin-my-plugin")
				}
				if got.ConfigJSON != "{}" {
					t.Errorf("ConfigJSON = %q, want %q", got.ConfigJSON, "{}")
				}
				if got.Scope != "global" {
					t.Errorf("Scope = %q, want %q", got.Scope, "global")
				}
			},
		},
		{
			"explicit id and config preserved",
			Plugin{Key: "k", ID: "custom-id", ConfigJSON: `{"x":1}`, Scope: "agent-1"},
			func(_ context.Context, p Plugin) (Plugin, error) { return p, nil },
			false, "",
			func(t *testing.T, got Plugin) {
				if got.ID != "custom-id" {
					t.Errorf("ID = %q, want %q", got.ID, "custom-id")
				}
				if got.ConfigJSON != `{"x":1}` {
					t.Errorf("ConfigJSON = %q, want preserved", got.ConfigJSON)
				}
				if got.Scope != "agent-1" {
					t.Errorf("Scope = %q, want %q", got.Scope, "agent-1")
				}
			},
		},
		{
			"invalid config json rejected",
			Plugin{Key: "k", ConfigJSON: "not-json"},
			nil, true, "PLUGIN", nil,
		},
		{
			"schema validation failure",
			Plugin{
				Key:              "k",
				ConfigJSON:       `{"name":123}`,
				ConfigSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			},
			nil, true, "PLUGIN", nil,
		},
		{
			"schema validation success",
			Plugin{
				Key:              "k",
				ConfigJSON:       `{"name":"hello"}`,
				ConfigSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			},
			func(_ context.Context, p Plugin) (Plugin, error) { return p, nil },
			false, "", nil,
		},
		{
			"empty schema skipped",
			Plugin{
				Key:              "k",
				ConfigJSON:       `{"name":123}`,
				ConfigSchemaJSON: "{}",
			},
			func(_ context.Context, p Plugin) (Plugin, error) { return p, nil },
			false, "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.repoFn != nil {
				mr.createFn = tt.repoFn
			}
			u := NewUsecase(mr, noOpRunRepo(), noOpScopeAgentLookup())
			got, err := u.Create(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e, ok := apierror.From(err); ok && e.Domain != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Domain, tt.errReason)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_UpdateConfig(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		config    string
		getFn     func(_ context.Context, id string) (Plugin, error)
		wantErr   bool
		errReason string
	}{
		{
			"empty id rejected",
			"", "{}", nil, true, "PLUGIN",
		},
		{
			"whitespace id rejected",
			"  ", "{}", nil, true, "PLUGIN",
		},
		{
			"empty config defaults to empty object",
			"p-1", "  ",
			func(_ context.Context, id string) (Plugin, error) {
				return Plugin{ID: id}, nil
			},
			false, "",
		},
		{
			"invalid config json rejected",
			"p-1", "not-json",
			nil, true, "PLUGIN",
		},
		{
			"get plugin error",
			"p-1", `{"x":1}`,
			func(_ context.Context, _ string) (Plugin, error) {
				return Plugin{}, apierror.NotFound("PLUGIN", "not found")
			},
			true, "PLUGIN",
		},
		{
			"schema validation failure",
			"p-1", `{"name":123}`,
			func(_ context.Context, id string) (Plugin, error) {
				return Plugin{ID: id, ConfigSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`}, nil
			},
			true, "PLUGIN",
		},
		{
			"schema validation success",
			"p-1", `{"name":"hello"}`,
			func(_ context.Context, id string) (Plugin, error) {
				return Plugin{ID: id, ConfigSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`}, nil
			},
			false, "",
		},
		{
			"no schema passes",
			"p-1", `{"anything":1}`,
			func(_ context.Context, id string) (Plugin, error) {
				return Plugin{ID: id, ConfigSchemaJSON: ""}, nil
			},
			false, "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.getFn != nil {
				mr.getFn = tt.getFn
			}
			u := NewUsecase(mr, noOpRunRepo(), noOpScopeAgentLookup())
			_, err := u.UpdateConfig(context.Background(), tt.id, tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e, ok := apierror.From(err); ok && e.Domain != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Domain, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_UpdateScope(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		scope     string
		agentFn   func(_ context.Context, id string) error
		wantErr   bool
		errReason string
	}{
		{
			"empty id rejected",
			"", "global", nil, true, "PLUGIN",
		},
		{
			"whitespace id rejected",
			"  ", "global", nil, true, "PLUGIN",
		},
		{
			"empty scope defaults to global",
			"p-1", "", nil, false, "",
		},
		{
			"whitespace scope defaults to global",
			"p-1", "  ", nil, false, "",
		},
		{
			"global scope skips agent check",
			"p-1", "global", nil, false, "",
		},
		{
			"GLOBAL case insensitive skips agent check",
			"p-1", "GLOBAL", nil, false, "",
		},
		{
			"agent scope with existing agent",
			"p-1", "agent-1",
			func(_ context.Context, _ string) error { return nil },
			false, "",
		},
		{
			"agent scope with missing agent",
			"p-1", "agent-missing",
			func(_ context.Context, _ string) error { return shared.ErrNotFound },
			true, "PLUGIN",
		},
		{
			"agent scope with other error",
			"p-1", "agent-1",
			func(_ context.Context, _ string) error { return apierror.Internal("AGENT", "db error") },
			true, "AGENT",
		},
		{
			"nil agents skips check",
			"p-1", "agent-1", nil, false, "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			var agents ScopeAgentLookup
			if tt.name == "nil agents skips check" {
				agents = nil
			} else if tt.agentFn != nil {
				agents = &mockScopeAgentLookup{agentExistsFn: tt.agentFn}
			} else {
				agents = noOpScopeAgentLookup()
			}
			u := NewUsecase(mr, noOpRunRepo(), agents)
			_, err := u.UpdateScope(context.Background(), tt.id, tt.scope)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e, ok := apierror.From(err); ok && e.Domain != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Domain, tt.errReason)
				}
			}
		})
	}
}

func isAPIErrorCode(err error, code apierror.Code) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == code
}

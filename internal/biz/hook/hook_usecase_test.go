package hook

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type mockRepo struct {
	listHooks  func(ctx context.Context) ([]Hook, error)
	getHook    func(ctx context.Context, id string) (Hook, error)
	createHook func(ctx context.Context, h Hook) (Hook, error)
	updateHook func(ctx context.Context, h Hook) (Hook, error)
	deleteHook func(ctx context.Context, id string) error
}

func (m *mockRepo) ListHooks(ctx context.Context) ([]Hook, error) {
	if m.listHooks != nil {
		return m.listHooks(ctx)
	}
	return nil, nil
}

func (m *mockRepo) GetHook(ctx context.Context, id string) (Hook, error) {
	if m.getHook != nil {
		return m.getHook(ctx, id)
	}
	return Hook{}, nil
}

func (m *mockRepo) CreateHook(ctx context.Context, h Hook) (Hook, error) {
	if m.createHook != nil {
		return m.createHook(ctx, h)
	}
	return h, nil
}

func (m *mockRepo) UpdateHook(ctx context.Context, h Hook) (Hook, error) {
	if m.updateHook != nil {
		return m.updateHook(ctx, h)
	}
	return h, nil
}

func (m *mockRepo) DeleteHook(ctx context.Context, id string) error {
	if m.deleteHook != nil {
		return m.deleteHook(ctx, id)
	}
	return nil
}

func isAPIError(err error, domain, message string) bool {
	if err == nil {
		return false
	}
	ae, ok := apierror.From(err)
	if !ok {
		return false
	}
	return ae.Domain == domain && ae.Message == message
}

func isAPIErrorDomain(err error, domain string) bool {
	if err == nil {
		return false
	}
	ae, ok := apierror.From(err)
	if !ok {
		return false
	}
	return ae.Domain == domain
}

func TestUsecase_Create(t *testing.T) {
	tests := []struct {
		name     string
		input    Hook
		wantErr  bool
		reason   string
		message  string
		assertFn func(t *testing.T, captured Hook)
	}{
		{
			name:    "valid creation fills status default and auto generates ID",
			input:   Hook{Key: "my_hook", Name: "My Hook"},
			wantErr: false,
			assertFn: func(t *testing.T, captured Hook) {
				if captured.Status != "active" {
					t.Errorf("Status = %q, want %q", captured.Status, "active")
				}
				if captured.ID == "" {
					t.Error("ID should be auto-generated")
				}
			},
		},
		{
			name:    "valid creation preserves provided status",
			input:   Hook{Key: "my_hook", Name: "My Hook", Status: "inactive"},
			wantErr: false,
			assertFn: func(t *testing.T, captured Hook) {
				if captured.Status != "inactive" {
					t.Errorf("Status = %q, want %q", captured.Status, "inactive")
				}
			},
		},
		{
			name:    "valid creation with valid notify config",
			input:   Hook{Key: "my_hook", Name: "My Hook", ConfigJSON: `{"callback_point":"before_agent","action":{"type":"notify","webhook_url":"https://example.com/hook"}}`},
			wantErr: false,
		},
		{
			name:    "empty key returns error",
			input:   Hook{Name: "My Hook"},
			wantErr: true,
			reason:  "HOOK",
			message: "key and name are required",
		},
		{
			name:    "empty name returns error",
			input:   Hook{Key: "my_hook"},
			wantErr: true,
			reason:  "HOOK",
			message: "key and name are required",
		},
		{
			name:    "whitespace only key returns error",
			input:   Hook{Key: "   ", Name: "My Hook"},
			wantErr: true,
			reason:  "HOOK",
			message: "key and name are required",
		},
		{
			name:    "whitespace only name returns error",
			input:   Hook{Key: "my_hook", Name: "   "},
			wantErr: true,
			reason:  "HOOK",
			message: "key and name are required",
		},
		{
			name:    "invalid config_json returns error",
			input:   Hook{Key: "my_hook", Name: "My Hook", ConfigJSON: "not-json"},
			wantErr: true,
			reason:  "HOOK",
		},
		{
			name:    "notify action without webhook_url returns error",
			input:   Hook{Key: "my_hook", Name: "My Hook", ConfigJSON: `{"action":{"type":"notify"}}`},
			wantErr: true,
			reason:  "HOOK",
			message: "webhook_url required for notify action",
		},
		{
			name:    "preserves provided ID",
			input:   Hook{ID: "custom-id", Key: "my_hook", Name: "My Hook"},
			wantErr: false,
			assertFn: func(t *testing.T, captured Hook) {
				if captured.ID != "custom-id" {
					t.Errorf("ID = %q, want %q", captured.ID, "custom-id")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured Hook
			repo := &mockRepo{
				createHook: func(_ context.Context, h Hook) (Hook, error) {
					captured = h
					return h, nil
				},
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			_, err := uc.Create(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.message != "" {
					if !isAPIError(err, tt.reason, tt.message) {
						ae, _ := apierror.From(err)
						t.Fatalf("expected apierror domain=%q message=%q, got domain=%q message=%q", tt.reason, tt.message, ae.Domain, ae.Message)
					}
				} else if !isAPIErrorDomain(err, tt.reason) {
					ae, _ := apierror.From(err)
					t.Fatalf("expected apierror domain=%q, got domain=%q", tt.reason, ae.Domain)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.assertFn != nil {
				tt.assertFn(t, captured)
			}
		})
	}
}

func TestUsecase_Update(t *testing.T) {
	existingHook := Hook{
		ID:           "h1",
		Key:          "old_key",
		Name:         "Old Name",
		Status:       "active",
		Description:  "old desc",
		Enabled:      true,
		SortOrder:    5,
		ConfigJSON:   `{"callback_point":"before_agent"}`,
		MetadataJSON: `{"old":true}`,
	}

	tests := []struct {
		name     string
		id       string
		current  Hook
		patch    HookPatch
		getErr   error
		wantErr  bool
		reason   string
		message  string
		assertFn func(t *testing.T, merged Hook)
	}{
		{
			name:    "merge patch fields into existing",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				Key:          StrPtr("new_key"),
				Name:         StrPtr("New Name"),
				Status:       StrPtr("inactive"),
				Description:  StrPtr("new desc"),
				Enabled:      BoolPtr(false),
				SortOrder:    IntPtr(10),
				ConfigJSON:   StrPtr(`{"callback_point":"after_agent"}`),
				MetadataJSON: StrPtr(`{"new":true}`),
			},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.Key != "new_key" {
					t.Errorf("Key = %q, want %q", merged.Key, "new_key")
				}
				if merged.Name != "New Name" {
					t.Errorf("Name = %q, want %q", merged.Name, "New Name")
				}
				if merged.Status != "inactive" {
					t.Errorf("Status = %q, want %q", merged.Status, "inactive")
				}
				if merged.Description != "new desc" {
					t.Errorf("Description = %q, want %q", merged.Description, "new desc")
				}
				if merged.Enabled != false {
					t.Errorf("Enabled = %v, want false", merged.Enabled)
				}
				if merged.SortOrder != 10 {
					t.Errorf("SortOrder = %d, want 10", merged.SortOrder)
				}
				if merged.ConfigJSON != `{"callback_point":"after_agent"}` {
					t.Errorf("ConfigJSON = %q, want updated value", merged.ConfigJSON)
				}
				if merged.MetadataJSON != `{"new":true}` {
					t.Errorf("MetadataJSON = %q, want updated value", merged.MetadataJSON)
				}
			},
		},
		{
			name:    "preserve key name status when patch nil",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				Description: StrPtr("updated desc"),
				Enabled:     BoolPtr(false),
				SortOrder:   IntPtr(0),
				ConfigJSON:  StrPtr(`{"callback_point":"before_agent"}`),
			},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.Key != "old_key" {
					t.Errorf("Key = %q, want %q (preserved)", merged.Key, "old_key")
				}
				if merged.Name != "Old Name" {
					t.Errorf("Name = %q, want %q (preserved)", merged.Name, "Old Name")
				}
				if merged.Status != "active" {
					t.Errorf("Status = %q, want %q (preserved)", merged.Status, "active")
				}
			},
		},
		{
			name:    "status change from active to inactive",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				Status:     StrPtr("inactive"),
				ConfigJSON: StrPtr(`{"callback_point":"before_agent"}`),
			},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.Status != "inactive" {
					t.Errorf("Status = %q, want %q", merged.Status, "inactive")
				}
			},
		},
		{
			name:    "config_json replaced by patch",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				ConfigJSON: StrPtr(`{"callback_point":"after_tool"}`),
			},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.ConfigJSON != `{"callback_point":"after_tool"}` {
					t.Errorf("ConfigJSON = %q, want %q", merged.ConfigJSON, `{"callback_point":"after_tool"}`)
				}
			},
		},
		{
			name:    "explicit zero values via pointer",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				Description:  StrPtr(""),
				Enabled:      BoolPtr(false),
				SortOrder:    IntPtr(0),
				ConfigJSON:   StrPtr(""),
				MetadataJSON: StrPtr(""),
			},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.Description != "" {
					t.Errorf("Description = %q, want empty", merged.Description)
				}
				if merged.Enabled != false {
					t.Errorf("Enabled = %v, want false", merged.Enabled)
				}
				if merged.SortOrder != 0 {
					t.Errorf("SortOrder = %d, want 0", merged.SortOrder)
				}
				if merged.ConfigJSON != "" {
					t.Errorf("ConfigJSON = %q, want empty", merged.ConfigJSON)
				}
				if merged.MetadataJSON != "" {
					t.Errorf("MetadataJSON = %q, want empty", merged.MetadataJSON)
				}
			},
		},
		{
			name:    "nil fields preserve existing values",
			id:      "h1",
			current: existingHook,
			patch:   HookPatch{},
			wantErr: false,
			assertFn: func(t *testing.T, merged Hook) {
				if merged.Key != "old_key" {
					t.Errorf("Key = %q, want %q (preserved)", merged.Key, "old_key")
				}
				if merged.Name != "Old Name" {
					t.Errorf("Name = %q, want %q (preserved)", merged.Name, "Old Name")
				}
				if merged.Description != "old desc" {
					t.Errorf("Description = %q, want %q (preserved)", merged.Description, "old desc")
				}
				if merged.Enabled != true {
					t.Errorf("Enabled = %v, want true (preserved)", merged.Enabled)
				}
				if merged.SortOrder != 5 {
					t.Errorf("SortOrder = %d, want 5 (preserved)", merged.SortOrder)
				}
			},
		},
		{
			name:    "non-existent hook returns error from repo",
			id:      "missing",
			getErr:  apierror.NotFound("HOOK", "hook not found"),
			wantErr: true,
			reason:  "HOOK",
			message: "hook not found",
		},
		{
			name:    "empty id returns error",
			id:      "",
			wantErr: true,
			reason:  "HOOK",
			message: "id is required",
		},
		{
			name:    "invalid merged config returns error",
			id:      "h1",
			current: existingHook,
			patch: HookPatch{
				ConfigJSON: StrPtr("not-json"),
			},
			wantErr: true,
			reason:  "HOOK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured Hook
			repo := &mockRepo{
				getHook: func(_ context.Context, _ string) (Hook, error) {
					if tt.getErr != nil {
						return Hook{}, tt.getErr
					}
					return tt.current, nil
				},
				updateHook: func(_ context.Context, h Hook) (Hook, error) {
					captured = h
					return h, nil
				},
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			_, err := uc.Update(context.Background(), tt.id, tt.patch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.message != "" {
					if !isAPIError(err, tt.reason, tt.message) {
						ae, _ := apierror.From(err)
						t.Fatalf("expected apierror domain=%q message=%q, got domain=%q message=%q", tt.reason, tt.message, ae.Domain, ae.Message)
					}
				} else if !isAPIErrorDomain(err, tt.reason) {
					ae, _ := apierror.From(err)
					t.Fatalf("expected apierror domain=%q, got domain=%q", tt.reason, ae.Domain)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.assertFn != nil {
				tt.assertFn(t, captured)
			}
		})
	}
}

func TestResolver(t *testing.T) {
	t.Run("reload loads enabled active hooks with callback_point", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h2", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"after_agent"}`},
					{ID: "h3", Enabled: false, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h4", Enabled: true, Status: "active", DeletedAt: "2024-01-01", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h5", Enabled: true, Status: "active", ConfigJSON: `{}`},
					{ID: "h6", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_tool"}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("any-agent", "")
		if len(result) != 3 {
			t.Errorf("Resolve returned %d hooks, want 3", len(result))
		}
	})

	t.Run("reload skips disabled hooks", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h2", Enabled: false, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("any", "")
		if len(result) != 1 {
			t.Fatalf("Resolve returned %d hooks, want 1", len(result))
		}
		if result[0].Hook.ID != "h1" {
			t.Errorf("got hook ID %q, want %q", result[0].Hook.ID, "h1")
		}
	})

	t.Run("reload skips deleted hooks", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h2", Enabled: true, Status: "active", DeletedAt: "2024-01-01", ConfigJSON: `{"callback_point":"before_agent"}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("any", "")
		if len(result) != 1 {
			t.Errorf("Resolve returned %d hooks, want 1", len(result))
		}
	})

	t.Run("reload skips hooks without callback_point", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
					{ID: "h2", Enabled: true, Status: "active", ConfigJSON: `{}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("any", "")
		if len(result) != 1 {
			t.Errorf("Resolve returned %d hooks, want 1", len(result))
		}
	})

	t.Run("resolve matches by agent_id", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent","condition":{"agent_id":"agent-1"}}`},
					{ID: "h2", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"after_agent","condition":{"agent_id":"agent-2"}}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("agent-1", "")
		if len(result) != 1 {
			t.Fatalf("Resolve returned %d hooks, want 1", len(result))
		}
		if result[0].Hook.ID != "h1" {
			t.Errorf("got hook ID %q, want %q", result[0].Hook.ID, "h1")
		}
	})

	t.Run("resolve matches by agent_key", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent","condition":{"agent_id":"my-key"}}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("some-id", "my-key")
		if len(result) != 1 {
			t.Fatalf("Resolve returned %d hooks, want 1", len(result))
		}
		if result[0].Hook.ID != "h1" {
			t.Errorf("got hook ID %q, want %q", result[0].Hook.ID, "h1")
		}
	})

	t.Run("resolve empty condition matches all agents", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("any-agent", "any-key")
		if len(result) != 1 {
			t.Errorf("Resolve returned %d hooks, want 1", len(result))
		}
	})

	t.Run("resolve returns empty when no hooks match", func(t *testing.T) {
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent","condition":{"agent_id":"agent-1"}}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		result := r.Resolve("other-agent", "")
		if len(result) != 0 {
			t.Errorf("Resolve returned %d hooks, want 0", len(result))
		}
	})

	t.Run("cache is used after reload", func(t *testing.T) {
		callCount := 0
		repo := &mockRepo{
			listHooks: func(_ context.Context) ([]Hook, error) {
				callCount++
				return []Hook{
					{ID: "h1", Enabled: true, Status: "active", ConfigJSON: `{"callback_point":"before_agent"}`},
				}, nil
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		r := NewResolver(uc, loggateway.NewNoop())
		err := r.Reload(context.Background())
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
		if callCount != 1 {
			t.Fatalf("expected 1 ListHooks call after Reload, got %d", callCount)
		}
		result := r.Resolve("any", "")
		if len(result) != 1 {
			t.Errorf("Resolve returned %d hooks, want 1", len(result))
		}
		if callCount != 1 {
			t.Errorf("expected no additional ListHooks call, callCount=%d", callCount)
		}
	})
}

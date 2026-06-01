package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"
)

func TestToProtoPlugin(t *testing.T) {
	cases := []struct {
		name string
		in   biz.Plugin
		want *v1.Plugin
	}{
		{
			name: "full_fields",
			in: biz.Plugin{
				ID:                "builtin-content-filter",
				Key:               "content-filter",
				Name:              "Content Filter",
				Description:       "Filters harmful content",
				Category:          "safety",
				RiskLevel:         "high",
				Enabled:           true,
				Scope:             "global",
				CallbackPoints:    []string{"before_invoke", "after_invoke"},
				SortOrder:         10,
				ConfigSchemaJSON:  `{"type":"object"}`,
				ConfigJSON:        `{"strict":true}`,
				DefaultConfigJSON: `{"strict":false}`,
				InvokeCount:       42,
				BlockCount:        7,
				ErrorCount:        1,
				LastInvokedAt:     "2025-06-01T12:00:00Z",
				LastStatus:        "ok",
				CreatedAt:         "2025-01-01T00:00:00Z",
				UpdatedAt:         "2025-06-01T12:00:00Z",
				Permissions: biz.PluginPermissions{
					CanView:       true,
					CanToggle:     true,
					CanEditConfig: false,
					CanViewLogs:   true,
				},
			},
			want: &v1.Plugin{
				Id:                "builtin-content-filter",
				Key:               "content-filter",
				Name:              "Content Filter",
				Description:       "Filters harmful content",
				Category:          "safety",
				RiskLevel:         "high",
				Enabled:           true,
				Scope:             "global",
				CallbackPoints:    []string{"before_invoke", "after_invoke"},
				SortOrder:         10,
				ConfigSchemaJson:  `{"type":"object"}`,
				ConfigJson:        `{"strict":true}`,
				DefaultConfigJson: `{"strict":false}`,
				InvokeCount:       42,
				BlockCount:        7,
				ErrorCount:        1,
				LastInvokedAt:     "2025-06-01T12:00:00Z",
				LastStatus:        "ok",
				CreatedAt:         "2025-01-01T00:00:00Z",
				UpdatedAt:         "2025-06-01T12:00:00Z",
				Permissions: &v1.PluginPermissions{
					CanView:       true,
					CanToggle:     true,
					CanEditConfig: false,
					CanViewLogs:   true,
				},
			},
		},
		{
			name: "zero_value",
			in:   biz.Plugin{},
			want: &v1.Plugin{
				Permissions: &v1.PluginPermissions{},
			},
		},
		{
			name: "empty_callback_points_slice",
			in: biz.Plugin{
				ID:             "p-1",
				CallbackPoints: []string{},
			},
			want: &v1.Plugin{
				Id:             "p-1",
				CallbackPoints: []string{},
				Permissions:    &v1.PluginPermissions{},
			},
		},
		{
			name: "nil_callback_points",
			in: biz.Plugin{
				ID:             "p-2",
				CallbackPoints: nil,
			},
			want: &v1.Plugin{
				Id:          "p-2",
				Permissions: &v1.PluginPermissions{},
			},
		},
		{
			name: "int32_cast_large_values",
			in: biz.Plugin{
				ID:          "p-3",
				SortOrder:   999,
				InvokeCount: 100000,
				BlockCount:  50000,
				ErrorCount:  25000,
			},
			want: &v1.Plugin{
				Id:          "p-3",
				SortOrder:   999,
				InvokeCount: 100000,
				BlockCount:  50000,
				ErrorCount:  25000,
				Permissions: &v1.PluginPermissions{},
			},
		},
		{
			name: "permissions_all_false",
			in: biz.Plugin{
				ID: "p-4",
				Permissions: biz.PluginPermissions{
					CanView:       false,
					CanToggle:     false,
					CanEditConfig: false,
					CanViewLogs:   false,
				},
			},
			want: &v1.Plugin{
				Id: "p-4",
				Permissions: &v1.PluginPermissions{
					CanView:       false,
					CanToggle:     false,
					CanEditConfig: false,
					CanViewLogs:   false,
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoPlugin(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.Key != tc.want.Key {
				t.Errorf("Key = %q, want %q", got.Key, tc.want.Key)
			}
			if got.Name != tc.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tc.want.Name)
			}
			if got.Description != tc.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tc.want.Description)
			}
			if got.Category != tc.want.Category {
				t.Errorf("Category = %q, want %q", got.Category, tc.want.Category)
			}
			if got.RiskLevel != tc.want.RiskLevel {
				t.Errorf("RiskLevel = %q, want %q", got.RiskLevel, tc.want.RiskLevel)
			}
			if got.Enabled != tc.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tc.want.Enabled)
			}
			if got.Scope != tc.want.Scope {
				t.Errorf("Scope = %q, want %q", got.Scope, tc.want.Scope)
			}
			if len(got.CallbackPoints) != len(tc.want.CallbackPoints) {
				t.Errorf("CallbackPoints len = %d, want %d", len(got.CallbackPoints), len(tc.want.CallbackPoints))
			} else {
				for i, cp := range got.CallbackPoints {
					if cp != tc.want.CallbackPoints[i] {
						t.Errorf("CallbackPoints[%d] = %q, want %q", i, cp, tc.want.CallbackPoints[i])
					}
				}
			}
			if got.SortOrder != tc.want.SortOrder {
				t.Errorf("SortOrder = %d, want %d", got.SortOrder, tc.want.SortOrder)
			}
			if got.ConfigSchemaJson != tc.want.ConfigSchemaJson {
				t.Errorf("ConfigSchemaJson = %q, want %q", got.ConfigSchemaJson, tc.want.ConfigSchemaJson)
			}
			if got.ConfigJson != tc.want.ConfigJson {
				t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, tc.want.ConfigJson)
			}
			if got.DefaultConfigJson != tc.want.DefaultConfigJson {
				t.Errorf("DefaultConfigJson = %q, want %q", got.DefaultConfigJson, tc.want.DefaultConfigJson)
			}
			if got.InvokeCount != tc.want.InvokeCount {
				t.Errorf("InvokeCount = %d, want %d", got.InvokeCount, tc.want.InvokeCount)
			}
			if got.BlockCount != tc.want.BlockCount {
				t.Errorf("BlockCount = %d, want %d", got.BlockCount, tc.want.BlockCount)
			}
			if got.ErrorCount != tc.want.ErrorCount {
				t.Errorf("ErrorCount = %d, want %d", got.ErrorCount, tc.want.ErrorCount)
			}
			if got.LastInvokedAt != tc.want.LastInvokedAt {
				t.Errorf("LastInvokedAt = %q, want %q", got.LastInvokedAt, tc.want.LastInvokedAt)
			}
			if got.LastStatus != tc.want.LastStatus {
				t.Errorf("LastStatus = %q, want %q", got.LastStatus, tc.want.LastStatus)
			}
			if got.CreatedAt != tc.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tc.want.CreatedAt)
			}
			if got.UpdatedAt != tc.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tc.want.UpdatedAt)
			}
			if tc.want.Permissions == nil {
				if got.Permissions != nil {
					t.Error("Permissions = non-nil, want nil")
				}
			} else {
				if got.Permissions == nil {
					t.Fatal("Permissions = nil, want non-nil")
				}
				if got.Permissions.CanView != tc.want.Permissions.CanView {
					t.Errorf("Permissions.CanView = %v, want %v", got.Permissions.CanView, tc.want.Permissions.CanView)
				}
				if got.Permissions.CanToggle != tc.want.Permissions.CanToggle {
					t.Errorf("Permissions.CanToggle = %v, want %v", got.Permissions.CanToggle, tc.want.Permissions.CanToggle)
				}
				if got.Permissions.CanEditConfig != tc.want.Permissions.CanEditConfig {
					t.Errorf("Permissions.CanEditConfig = %v, want %v", got.Permissions.CanEditConfig, tc.want.Permissions.CanEditConfig)
				}
				if got.Permissions.CanViewLogs != tc.want.Permissions.CanViewLogs {
					t.Errorf("Permissions.CanViewLogs = %v, want %v", got.Permissions.CanViewLogs, tc.want.Permissions.CanViewLogs)
				}
			}
		})
	}
}

func TestToProtoPluginRun(t *testing.T) {
	cases := []struct {
		name string
		in   biz.PluginRun
		want *v1.PluginRun
	}{
		{
			name: "full_fields",
			in: biz.PluginRun{
				ID:            "run-1",
				PluginKey:     "content-filter",
				PluginID:      "builtin-content-filter",
				SessionID:     "sess-1",
				AgentID:       "agent-1",
				CallbackPoint: "before_invoke",
				Status:        "completed",
				DurationMS:    150,
				DetailJSON:    `{"blocked":false}`,
				CreatedAt:     "2025-06-01T12:00:00Z",
			},
			want: &v1.PluginRun{
				Id:            "run-1",
				PluginKey:     "content-filter",
				PluginId:      "builtin-content-filter",
				SessionId:     "sess-1",
				AgentId:       "agent-1",
				CallbackPoint: "before_invoke",
				Status:        "completed",
				DurationMs:    150,
				DetailJson:    `{"blocked":false}`,
				CreatedAt:     "2025-06-01T12:00:00Z",
			},
		},
		{
			name: "zero_value",
			in:   biz.PluginRun{},
			want: &v1.PluginRun{},
		},
		{
			name: "duration_ms_int32_cast",
			in: biz.PluginRun{
				ID:         "run-2",
				DurationMS: 5000,
			},
			want: &v1.PluginRun{
				Id:         "run-2",
				DurationMs: 5000,
			},
		},
		{
			name: "zero_duration_ms",
			in: biz.PluginRun{
				ID:         "run-3",
				DurationMS: 0,
			},
			want: &v1.PluginRun{
				Id:         "run-3",
				DurationMs: 0,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoPluginRun(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.PluginKey != tc.want.PluginKey {
				t.Errorf("PluginKey = %q, want %q", got.PluginKey, tc.want.PluginKey)
			}
			if got.PluginId != tc.want.PluginId {
				t.Errorf("PluginId = %q, want %q", got.PluginId, tc.want.PluginId)
			}
			if got.SessionId != tc.want.SessionId {
				t.Errorf("SessionId = %q, want %q", got.SessionId, tc.want.SessionId)
			}
			if got.AgentId != tc.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tc.want.AgentId)
			}
			if got.CallbackPoint != tc.want.CallbackPoint {
				t.Errorf("CallbackPoint = %q, want %q", got.CallbackPoint, tc.want.CallbackPoint)
			}
			if got.Status != tc.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tc.want.Status)
			}
			if got.DurationMs != tc.want.DurationMs {
				t.Errorf("DurationMs = %d, want %d", got.DurationMs, tc.want.DurationMs)
			}
			if got.DetailJson != tc.want.DetailJson {
				t.Errorf("DetailJson = %q, want %q", got.DetailJson, tc.want.DetailJson)
			}
			if got.CreatedAt != tc.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tc.want.CreatedAt)
			}
		})
	}
}

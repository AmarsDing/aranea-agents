package service

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestBizAuditToProto(t *testing.T) {
	cases := []struct {
		name string
		in   biz.AuditLog
		want *v1.AuditLog
	}{
		{
			name: "full_fields",
			in: biz.AuditLog{
				ID:           "audit-1",
				Action:       "create",
				Resource:     "agent",
				ResourceID:   "res-1",
				RequestID:    "req-1",
				Detail:       "created agent",
				CreatedAt:    "2025-01-01T00:00:00Z",
				Actor:        "admin",
				IP:           "127.0.0.1",
				UserAgent:    "test/1.0",
				Severity:     "info",
				MetadataJSON: `{"key":"val"}`,
			},
			want: &v1.AuditLog{
				Id:           "audit-1",
				Action:       "create",
				Resource:     "agent",
				ResourceId:   "res-1",
				RequestId:    "req-1",
				Detail:       "created agent",
				CreatedAt:    "2025-01-01T00:00:00Z",
				Actor:        "admin",
				Ip:           "127.0.0.1",
				UserAgent:    "test/1.0",
				Severity:     "info",
				MetadataJson: `{"key":"val"}`,
			},
		},
		{
			name: "zero_value",
			in:   biz.AuditLog{},
			want: &v1.AuditLog{},
		},
		{
			name: "partial_fields",
			in: biz.AuditLog{
				ID:     "audit-2",
				Action: "delete",
			},
			want: &v1.AuditLog{
				Id:     "audit-2",
				Action: "delete",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := bizAuditToProto(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.Action != tc.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tc.want.Action)
			}
			if got.Resource != tc.want.Resource {
				t.Errorf("Resource = %q, want %q", got.Resource, tc.want.Resource)
			}
			if got.ResourceId != tc.want.ResourceId {
				t.Errorf("ResourceId = %q, want %q", got.ResourceId, tc.want.ResourceId)
			}
			if got.RequestId != tc.want.RequestId {
				t.Errorf("RequestId = %q, want %q", got.RequestId, tc.want.RequestId)
			}
			if got.Detail != tc.want.Detail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.want.Detail)
			}
			if got.CreatedAt != tc.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tc.want.CreatedAt)
			}
			if got.Actor != tc.want.Actor {
				t.Errorf("Actor = %q, want %q", got.Actor, tc.want.Actor)
			}
			if got.Ip != tc.want.Ip {
				t.Errorf("Ip = %q, want %q", got.Ip, tc.want.Ip)
			}
			if got.UserAgent != tc.want.UserAgent {
				t.Errorf("UserAgent = %q, want %q", got.UserAgent, tc.want.UserAgent)
			}
			if got.Severity != tc.want.Severity {
				t.Errorf("Severity = %q, want %q", got.Severity, tc.want.Severity)
			}
			if got.MetadataJson != tc.want.MetadataJson {
				t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, tc.want.MetadataJson)
			}
		})
	}
}

func TestBizMonitorRowToProto(t *testing.T) {
	cases := []struct {
		name string
		in   biz.MonitorPlatformRow
		want *v1.MonitorPlatformRow
	}{
		{
			name: "full_fields",
			in: biz.MonitorPlatformRow{
				Resource:     "trace",
				ID:           "row-1",
				Key:          "runner.completion",
				Name:         "Runner Completion",
				Description:  "A runner completed",
				Status:       "ok",
				Enabled:      true,
				SortOrder:    5,
				ParentID:     "parent-1",
				Level:        "info",
				AgentID:      "agent-1",
				Provider:     "openai",
				Model:        "gpt-4",
				ConfigJSON:   `{"api_key":"sk-123","port":8080}`,
				MetadataJSON: `{"token":"abc","name":"test"}`,
				CreatedAt:    "2025-01-01T00:00:00Z",
				UpdatedAt:    "2025-01-02T00:00:00Z",
				DeletedAt:    "",
			},
			want: &v1.MonitorPlatformRow{
				Id:           "row-1",
				Resource:     "trace",
				Key:          "runner.completion",
				Name:         "Runner Completion",
				Description:  "A runner completed",
				Status:       "ok",
				Enabled:      true,
				SortOrder:    5,
				ParentId:     "parent-1",
				Level:        "info",
				AgentId:      "agent-1",
				Provider:     "openai",
				Model:        "gpt-4",
				ConfigJson:   `{"api_key":"******","port":8080}`,
				MetadataJson: `{"name":"test","token":"******"}`,
				CreatedAt:    "2025-01-01T00:00:00Z",
				UpdatedAt:    "2025-01-02T00:00:00Z",
				DeletedAt:    "",
			},
		},
		{
			name: "zero_value",
			in:   biz.MonitorPlatformRow{},
			want: &v1.MonitorPlatformRow{},
		},
		{
			name: "nil_json_fields",
			in: biz.MonitorPlatformRow{
				ID:           "row-2",
				ConfigJSON:   "",
				MetadataJSON: "",
			},
			want: &v1.MonitorPlatformRow{
				Id:           "row-2",
				ConfigJson:   "",
				MetadataJson: "",
			},
		},
		{
			name: "invalid_json_passthrough",
			in: biz.MonitorPlatformRow{
				ID:           "row-3",
				ConfigJSON:   "{not json}",
				MetadataJSON: "  ",
			},
			want: &v1.MonitorPlatformRow{
				Id:           "row-3",
				ConfigJson:   "{not json}",
				MetadataJson: "  ",
			},
		},
		{
			name: "sort_order_int32_cast",
			in: biz.MonitorPlatformRow{
				ID:        "row-4",
				SortOrder: 100,
			},
			want: &v1.MonitorPlatformRow{
				Id:        "row-4",
				SortOrder: 100,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := bizMonitorRowToProto(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.Resource != tc.want.Resource {
				t.Errorf("Resource = %q, want %q", got.Resource, tc.want.Resource)
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
			if got.Status != tc.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tc.want.Status)
			}
			if got.Enabled != tc.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tc.want.Enabled)
			}
			if got.SortOrder != tc.want.SortOrder {
				t.Errorf("SortOrder = %d, want %d", got.SortOrder, tc.want.SortOrder)
			}
			if got.ParentId != tc.want.ParentId {
				t.Errorf("ParentId = %q, want %q", got.ParentId, tc.want.ParentId)
			}
			if got.Level != tc.want.Level {
				t.Errorf("Level = %q, want %q", got.Level, tc.want.Level)
			}
			if got.AgentId != tc.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tc.want.AgentId)
			}
			if got.Provider != tc.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tc.want.Provider)
			}
			if got.Model != tc.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tc.want.Model)
			}
			if got.CreatedAt != tc.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tc.want.CreatedAt)
			}
			if got.UpdatedAt != tc.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tc.want.UpdatedAt)
			}
			if got.DeletedAt != tc.want.DeletedAt {
				t.Errorf("DeletedAt = %q, want %q", got.DeletedAt, tc.want.DeletedAt)
			}
			if tc.name == "full_fields" {
				if got.ConfigJson == tc.in.ConfigJSON {
					t.Error("ConfigJson was not sanitized")
				}
				if got.MetadataJson == tc.in.MetadataJSON {
					t.Error("MetadataJson was not sanitized")
				}
			}
			if tc.name == "nil_json_fields" || tc.name == "invalid_json_passthrough" {
				if got.ConfigJson != tc.want.ConfigJson {
					t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, tc.want.ConfigJson)
				}
				if got.MetadataJson != tc.want.MetadataJson {
					t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, tc.want.MetadataJson)
				}
			}
		})
	}
}

func TestToProtoAlertRule(t *testing.T) {
	cases := []struct {
		name string
		in   biz.MonitorAlertRule
		want *v1.MonitorAlertRule
	}{
		{
			name: "full_fields",
			in: biz.MonitorAlertRule{
				ID:               "rule-1",
				Name:             "High Error Rate",
				MetricKey:        "runner.error_rate",
				Threshold:        0.5,
				WindowMinutes:    30,
				Enabled:          true,
				Severity:         "critical",
				NotifyWebhookURL: "https://hooks.example.com/alert",
				NotifyChannelID:  "ch-1",
				CooldownMinutes:  15,
			},
			want: &v1.MonitorAlertRule{
				Id:               "rule-1",
				Name:             "High Error Rate",
				MetricKey:        "runner.error_rate",
				Threshold:        0.5,
				WindowMinutes:    30,
				Enabled:          true,
				Severity:         "critical",
				NotifyWebhookUrl: "https://hooks.example.com/alert",
				NotifyChannelId:  "ch-1",
				CooldownMinutes:  15,
			},
		},
		{
			name: "zero_value",
			in:   biz.MonitorAlertRule{},
			want: &v1.MonitorAlertRule{},
		},
		{
			name: "int32_cast_large_values",
			in: biz.MonitorAlertRule{
				ID:              "rule-2",
				WindowMinutes:   1440,
				CooldownMinutes: 720,
			},
			want: &v1.MonitorAlertRule{
				Id:              "rule-2",
				WindowMinutes:   1440,
				CooldownMinutes: 720,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoAlertRule(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.Name != tc.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tc.want.Name)
			}
			if got.MetricKey != tc.want.MetricKey {
				t.Errorf("MetricKey = %q, want %q", got.MetricKey, tc.want.MetricKey)
			}
			if got.Threshold != tc.want.Threshold {
				t.Errorf("Threshold = %v, want %v", got.Threshold, tc.want.Threshold)
			}
			if got.WindowMinutes != tc.want.WindowMinutes {
				t.Errorf("WindowMinutes = %d, want %d", got.WindowMinutes, tc.want.WindowMinutes)
			}
			if got.Enabled != tc.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tc.want.Enabled)
			}
			if got.Severity != tc.want.Severity {
				t.Errorf("Severity = %q, want %q", got.Severity, tc.want.Severity)
			}
			if got.NotifyWebhookUrl != tc.want.NotifyWebhookUrl {
				t.Errorf("NotifyWebhookUrl = %q, want %q", got.NotifyWebhookUrl, tc.want.NotifyWebhookUrl)
			}
			if got.NotifyChannelId != tc.want.NotifyChannelId {
				t.Errorf("NotifyChannelId = %q, want %q", got.NotifyChannelId, tc.want.NotifyChannelId)
			}
			if got.CooldownMinutes != tc.want.CooldownMinutes {
				t.Errorf("CooldownMinutes = %d, want %d", got.CooldownMinutes, tc.want.CooldownMinutes)
			}
		})
	}
}

func TestFromProtoAlertRule(t *testing.T) {
	cases := []struct {
		name string
		in   *v1.MonitorAlertRule
		want biz.MonitorAlertRule
	}{
		{
			name: "full_fields",
			in: &v1.MonitorAlertRule{
				Id:               "rule-1",
				Name:             "High Error Rate",
				MetricKey:        "runner.error_rate",
				Threshold:        0.5,
				WindowMinutes:    30,
				Enabled:          true,
				Severity:         "critical",
				NotifyWebhookUrl: "https://hooks.example.com/alert",
				NotifyChannelId:  "ch-1",
				CooldownMinutes:  15,
			},
			want: biz.MonitorAlertRule{
				ID:               "rule-1",
				Name:             "High Error Rate",
				MetricKey:        "runner.error_rate",
				Threshold:        0.5,
				WindowMinutes:    30,
				Enabled:          true,
				Severity:         "critical",
				NotifyWebhookURL: "https://hooks.example.com/alert",
				NotifyChannelID:  "ch-1",
				CooldownMinutes:  15,
			},
		},
		{
			name: "nil_input",
			in:   nil,
			want: biz.MonitorAlertRule{},
		},
		{
			name: "zero_value_proto",
			in:   &v1.MonitorAlertRule{},
			want: biz.MonitorAlertRule{},
		},
		{
			name: "int32_to_int_cast",
			in: &v1.MonitorAlertRule{
				Id:              "rule-3",
				WindowMinutes:   1440,
				CooldownMinutes: 720,
			},
			want: biz.MonitorAlertRule{
				ID:              "rule-3",
				WindowMinutes:   1440,
				CooldownMinutes: 720,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fromProtoAlertRule(tc.in)
			if got.ID != tc.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tc.want.ID)
			}
			if got.Name != tc.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tc.want.Name)
			}
			if got.MetricKey != tc.want.MetricKey {
				t.Errorf("MetricKey = %q, want %q", got.MetricKey, tc.want.MetricKey)
			}
			if got.Threshold != tc.want.Threshold {
				t.Errorf("Threshold = %v, want %v", got.Threshold, tc.want.Threshold)
			}
			if got.WindowMinutes != tc.want.WindowMinutes {
				t.Errorf("WindowMinutes = %d, want %d", got.WindowMinutes, tc.want.WindowMinutes)
			}
			if got.Enabled != tc.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tc.want.Enabled)
			}
			if got.Severity != tc.want.Severity {
				t.Errorf("Severity = %q, want %q", got.Severity, tc.want.Severity)
			}
			if got.NotifyWebhookURL != tc.want.NotifyWebhookURL {
				t.Errorf("NotifyWebhookURL = %q, want %q", got.NotifyWebhookURL, tc.want.NotifyWebhookURL)
			}
			if got.NotifyChannelID != tc.want.NotifyChannelID {
				t.Errorf("NotifyChannelID = %q, want %q", got.NotifyChannelID, tc.want.NotifyChannelID)
			}
			if got.CooldownMinutes != tc.want.CooldownMinutes {
				t.Errorf("CooldownMinutes = %d, want %d", got.CooldownMinutes, tc.want.CooldownMinutes)
			}
		})
	}
}

func TestNotFoundMonitor(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		wantNotFound    bool
		wantPassThrough bool
	}{
		{
			name:         "sql_err_no_rows",
			err:          sql.ErrNoRows,
			wantNotFound: true,
		},
		{
			name:            "other_error",
			err:             errors.New("some other error"),
			wantPassThrough: true,
		},
		{
			name:            "nil_error",
			err:             nil,
			wantPassThrough: true,
		},
		{
			name:         "wrapped_sql_err_no_rows",
			err:          fmt.Errorf("query: %w", sql.ErrNoRows),
			wantNotFound: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := notFoundMonitor(tc.err)
			if tc.wantNotFound {
				if got == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(got)
				if ke.Reason != "MONITOR_NOT_FOUND" {
					t.Errorf("Reason = %q, want %q", ke.Reason, "MONITOR_NOT_FOUND")
				}
				if ke.StatusCode != 404 {
					t.Errorf("StatusCode = %d, want 404", ke.StatusCode)
				}
				return
			}
			if tc.wantPassThrough {
				if got != tc.err {
					t.Errorf("got = %v, want %v", got, tc.err)
				}
			}
		})
	}
}

func TestDefaultAlertRules(t *testing.T) {
	rules := defaultAlertRules()
	if len(rules) != 1 {
		t.Fatalf("len = %d, want 1", len(rules))
	}
	r := rules[0]
	if r.ID != "default-runner-errors" {
		t.Errorf("ID = %q, want %q", r.ID, "default-runner-errors")
	}
	if r.Name != "Runner error rate" {
		t.Errorf("Name = %q, want %q", r.Name, "Runner error rate")
	}
	if r.MetricKey != "runner.error_rate" {
		t.Errorf("MetricKey = %q, want %q", r.MetricKey, "runner.error_rate")
	}
	if r.Threshold != 0.25 {
		t.Errorf("Threshold = %v, want 0.25", r.Threshold)
	}
	if r.WindowMinutes != 60 {
		t.Errorf("WindowMinutes = %d, want 60", r.WindowMinutes)
	}
	if !r.Enabled {
		t.Error("Enabled = false, want true")
	}
	if r.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", r.Severity, "warning")
	}
}

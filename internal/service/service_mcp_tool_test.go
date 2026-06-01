package service_test

import (
	"testing"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoMCP(t *testing.T) {
	tests := []struct {
		name string
		in   biz.MCPServer
		want mcpv1.MCPServer
	}{
		{
			name: "full_fields",
			in: biz.MCPServer{
				ID: "id1", Key: "key1", Name: "name1", Description: "desc1",
				Status: "active", Enabled: true, SortOrder: 5,
				ConfigJSON: `{"a":1}`, MetadataJSON: `{"b":2}`,
				CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02", DeletedAt: "",
			},
			want: mcpv1.MCPServer{
				Id: "id1", Key: "key1", Name: "name1", Description: "desc1",
				Status: "active", Enabled: true, SortOrder: 5,
				ConfigJson: `{"a":1}`, MetadataJson: `{"b":2}`,
				CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02", DeletedAt: "",
			},
		},
		{
			name: "zero_values",
			in:   biz.MCPServer{},
			want: mcpv1.MCPServer{},
		},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoMCP(tt.in)
			if got.Id != tt.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tt.want.Id)
			}
			if got.Key != tt.want.Key {
				t.Errorf("Key = %q, want %q", got.Key, tt.want.Key)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.want.Description)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.want.Status)
			}
			if got.Enabled != tt.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
			}
			if got.SortOrder != tt.want.SortOrder {
				t.Errorf("SortOrder = %d, want %d", got.SortOrder, tt.want.SortOrder)
			}
			if got.ConfigJson != tt.want.ConfigJson {
				t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, tt.want.ConfigJson)
			}
			if got.MetadataJson != tt.want.MetadataJson {
				t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, tt.want.MetadataJson)
			}
		})
	}
}

func TestPatchFromProtoMCP(t *testing.T) {
	tests := []struct {
		name string
		in   *mcpv1.MCPServer
		want biz.MCPServer
	}{
		{
			name: "full_fields",
			in: &mcpv1.MCPServer{
				Key: "k", Name: "n", Description: "d", Status: "active",
				Enabled: true, SortOrder: 3, ConfigJson: `{"x":1}`, MetadataJson: `{"y":2}`,
			},
			want: biz.MCPServer{
				Key: "k", Name: "n", Description: "d", Status: "active",
				Enabled: true, SortOrder: 3, ConfigJSON: `{"x":1}`, MetadataJSON: `{"y":2}`,
			},
		},
		{
			name: "nil_input",
			in:   nil,
			want: biz.MCPServer{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.PatchFromProtoMCP(tt.in)
			if got.Key != tt.want.Key {
				t.Errorf("Key = %q, want %q", got.Key, tt.want.Key)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Enabled != tt.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
			}
			if got.SortOrder != tt.want.SortOrder {
				t.Errorf("SortOrder = %d, want %d", got.SortOrder, tt.want.SortOrder)
			}
			if got.ConfigJSON != tt.want.ConfigJSON {
				t.Errorf("ConfigJSON = %q, want %q", got.ConfigJSON, tt.want.ConfigJSON)
			}
		})
	}
}

func TestToProtoMCPUserCred(t *testing.T) {
	in := biz.MCPServerUserCredential{
		ID: "uc1", MCPServerID: "s1", UserID: "u1",
		CredentialKey: "ck1", Status: "active", Configured: true,
		MaskedPreview: "mp1", CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	got := service.ToProtoMCPUserCred(in)
	if got.Id != "uc1" {
		t.Errorf("Id = %q, want %q", got.Id, "uc1")
	}
	if got.McpServerId != "s1" {
		t.Errorf("McpServerId = %q, want %q", got.McpServerId, "s1")
	}
	if got.CredentialKey != "ck1" {
		t.Errorf("CredentialKey = %q, want %q", got.CredentialKey, "ck1")
	}
	if !got.Configured {
		t.Errorf("Configured = false, want true")
	}
}

func TestBizToolToProto(t *testing.T) {
	avgMS := 123.5
	p95MS := 456.7
	tests := []struct {
		name string
		in   biz.Tool
	}{
		{
			name: "with_optional_durations",
			in: biz.Tool{
				ID: "t1", Key: "k1", DisplayName: "dn1", Description: "desc1",
				Category: "cat1", Source: "src1", RiskLevel: "high", Enabled: true,
				Readonly: true, RequiresConfirmation: false, SupportsStreaming: true, SupportsConcurrency: false,
				ParametersSchemaJSON: `{"type":"object"}`, ResultSchemaJSON: "", ConfigSchemaJSON: "",
				ConfigJSON: "", DefaultConfigJSON: "", MetadataJSON: "",
				RuntimeStatus: "online", RuntimeKind: "builtin",
				InvokeCount: 10, InvokeCount24h: 5, SuccessCount: 8, FailureCount: 2, BlockedCount: 0,
				AgentOverrideCount: 1, LastInvokedAt: "2024-01-01", LastStatus: "success",
				CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
				AvgDurationMS: &avgMS, P95DurationMS: p95MS,
				Permissions: biz.ToolPermissions{CanManage: true},
			},
		},
		{
			name: "nil_durations",
			in: biz.Tool{
				ID: "t2", Key: "k2", DisplayName: "dn2",
				Permissions: biz.ToolPermissions{CanManage: false},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.BizToolToProto(tt.in)
			if got.Id != tt.in.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.in.ID)
			}
			if got.Key != tt.in.Key {
				t.Errorf("Key = %q, want %q", got.Key, tt.in.Key)
			}
			if got.DisplayName != tt.in.DisplayName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.in.DisplayName)
			}
			if got.InvokeCount != int32(tt.in.InvokeCount) {
				t.Errorf("InvokeCount = %d, want %d", got.InvokeCount, int32(tt.in.InvokeCount))
			}
			if tt.in.AvgDurationMS != nil {
				if got.AvgDurationMs == nil || *got.AvgDurationMs != *tt.in.AvgDurationMS {
					t.Errorf("AvgDurationMs mismatch")
				}
			} else {
				if got.AvgDurationMs != nil {
					t.Errorf("AvgDurationMs should be nil")
				}
			}
			if got.P95DurationMs != tt.in.P95DurationMS {
				t.Errorf("P95DurationMs mismatch")
			}
			if got.Permissions == nil || got.Permissions.CanManage != tt.in.Permissions.CanManage {
				t.Errorf("Permissions.CanManage mismatch")
			}
		})
	}
}

func TestBizSummaryToProto(t *testing.T) {
	in := biz.ToolSummary{
		TotalTools: 10, EnabledTools: 8, HighRiskEnabled: 2, Calls24h: 100, FailureRate24h: 0.05,
	}
	got := service.BizSummaryToProto(in)
	if got.TotalTools != 10 {
		t.Errorf("TotalTools = %d, want 10", got.TotalTools)
	}
	if got.EnabledTools != 8 {
		t.Errorf("EnabledTools = %d, want 8", got.EnabledTools)
	}
	if got.HighRiskEnabled != 2 {
		t.Errorf("HighRiskEnabled = %d, want 2", got.HighRiskEnabled)
	}
	if got.Calls_24H != 100 {
		t.Errorf("Calls_24H = %d, want 100", got.Calls_24H)
	}
	if got.FailureRate_24H != 0.05 {
		t.Errorf("FailureRate_24H = %f, want 0.05", got.FailureRate_24H)
	}
}

func TestBizInvocationToProto(t *testing.T) {
	in := biz.ToolInvocation{
		ID: "inv1", RequestID: "req1", InvocationID: "iid1",
		ToolID: "t1", ToolKey: "tk1", ToolDisplayName: "TDN1",
		AgentID: "a1", AgentKey: "ak1", AgentDisplayName: "ADN1",
		SessionID: "s1", MessageID: "m1", UserID: "u1",
		Source: "web", Status: "completed",
		StartedAt: "2024-01-01", EndedAt: "2024-01-01",
		DurationMS: 500, InputPreview: "ip", InputHash: "ih",
		OutputPreview: "op", OutputHash: "oh",
		ErrorCode: "", ErrorMessage: "",
		RedactionApplied: true, MetadataJSON: `{"k":"v"}`,
		CreatedAt: "2024-01-01",
		Streaming: true, ChunkCount: 3,
	}
	got := service.BizInvocationToProto(in)
	if got.Id != "inv1" {
		t.Errorf("Id = %q, want %q", got.Id, "inv1")
	}
	if got.DurationMs != 500 {
		t.Errorf("DurationMs = %d, want 500", got.DurationMs)
	}
	if got.ChunkCount != 3 {
		t.Errorf("ChunkCount = %d, want 3", got.ChunkCount)
	}
	if !got.RedactionApplied {
		t.Errorf("RedactionApplied = false, want true")
	}
}

func TestBizOverrideToProto(t *testing.T) {
	in := biz.ToolAgentOverride{
		ID: "o1", ToolID: "t1", ToolKey: "tk1", AgentID: "a1",
		Enabled: true, Mode: "override", ConfigOverrideJSON: `{"x":1}`,
		RequiresConfirmation: true, CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	got := service.BizOverrideToProto(in)
	if got.Id != "o1" {
		t.Errorf("Id = %q, want %q", got.Id, "o1")
	}
	if got.Mode != "override" {
		t.Errorf("Mode = %q, want %q", got.Mode, "override")
	}
	if !got.RequiresConfirmation {
		t.Errorf("RequiresConfirmation = false, want true")
	}
}

func TestBizToolInvocationAuditToProto(t *testing.T) {
	in := biz.ToolInvocationAudit{
		ID: "a1", InvocationID: "inv1", ToolKey: "tk1",
		AgentID: "a1", UserID: "u1", SessionID: "s1",
		Action: "approve", ResultSummary: "ok", Status: "completed",
		Source: "admin", CreatedAt: "2024-01-01",
	}
	got := service.BizToolInvocationAuditToProto(in)
	if got.Id != "a1" {
		t.Errorf("Id = %q, want %q", got.Id, "a1")
	}
	if got.Action != "approve" {
		t.Errorf("Action = %q, want %q", got.Action, "approve")
	}
	if got.Source != "admin" {
		t.Errorf("Source = %q, want %q", got.Source, "admin")
	}
}

func TestToProtoMCP_RoundTrip(t *testing.T) {
	original := biz.MCPServer{
		ID: "rt1", Key: "rtk", Name: "rtn", Description: "rtd",
		Status: "active", Enabled: true, SortOrder: 7,
		ConfigJSON: `{"c":1}`, MetadataJSON: `{"m":2}`,
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	pb := service.ToProtoMCP(original)
	back := service.PatchFromProtoMCP(pb)
	if back.Key != original.Key {
		t.Errorf("roundtrip Key = %q, want %q", back.Key, original.Key)
	}
	if back.Name != original.Name {
		t.Errorf("roundtrip Name = %q, want %q", back.Name, original.Name)
	}
	if back.Enabled != original.Enabled {
		t.Errorf("roundtrip Enabled = %v, want %v", back.Enabled, original.Enabled)
	}
	if back.SortOrder != original.SortOrder {
		t.Errorf("roundtrip SortOrder = %d, want %d", back.SortOrder, original.SortOrder)
	}
	if back.ConfigJSON != original.ConfigJSON {
		t.Errorf("roundtrip ConfigJSON = %q, want %q", back.ConfigJSON, original.ConfigJSON)
	}
}

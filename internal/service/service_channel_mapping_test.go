package service_test

import (
	"encoding/json"
	"testing"

	v1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestMergeChannelMetadataJSON(t *testing.T) {
	tests := []struct {
		name string
		base string
		patch string
		want string
	}{
		{
			name:  "merge_patch_into_base",
			base:  `{"key1":"val1"}`,
			patch: `{"key2":"val2"}`,
			want:  `{"key1":"val1","key2":"val2"}`,
		},
		{
			name:  "patch_overwrites_base",
			base:  `{"key1":"old"}`,
			patch: `{"key1":"new"}`,
			want:  `{"key1":"new"}`,
		},
		{
			name:  "empty_patch",
			base:  `{"key1":"val1"}`,
			patch: "",
			want:  `{"key1":"val1"}`,
		},
		{
			name:  "whitespace_patch",
			base:  `{"key1":"val1"}`,
			patch: "   ",
			want:  `{"key1":"val1"}`,
		},
		{
			name:  "invalid_base_treated_as_empty",
			base:  "not-json",
			patch: `{"key2":"val2"}`,
			want:  `{"key2":"val2"}`,
		},
		{
			name:  "invalid_patch_ignored",
			base:  `{"key1":"val1"}`,
			patch: "not-json",
			want:  `{"key1":"val1"}`,
		},
		{
			name:  "both_empty",
			base:  "",
			patch: "",
			want:  `{}`,
		},
		{
			name:  "whitespace_base_treated_as_empty",
			base:  "  ",
			patch: `{"key1":"val1"}`,
			want:  `{"key1":"val1"}`,
		},
		{
			name:  "merge_nested_values",
			base:  `{"runtime_connected":true}`,
			patch: `{"runtime_connected_since":"2025-01-01T00:00:00Z"}`,
			want:  `{"runtime_connected":true,"runtime_connected_since":"2025-01-01T00:00:00Z"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.MergeChannelMetadataJSON(tt.base, tt.patch)
			var gotMap, wantMap map[string]any
			if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
				t.Fatalf("got is not valid JSON: %q, err: %v", got, err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("want is not valid JSON: %q, err: %v", tt.want, err)
			}
			for k, v := range wantMap {
				if gotMap[k] != v {
					t.Errorf("key %q: got %v, want %v", k, gotMap[k], v)
				}
			}
			if len(gotMap) != len(wantMap) {
				t.Errorf("got %d keys, want %d keys", len(gotMap), len(wantMap))
			}
		})
	}
}

func TestBizChannelToProto(t *testing.T) {
	ch := biz.Channel{
		ID:           "ch-1",
		Resource:     "channels",
		Key:          "feishu-default",
		Name:         "Feishu Bot",
		Description:  "Main Feishu channel",
		Status:       "active",
		Enabled:      true,
		SortOrder:    10,
		ParentID:     "",
		Level:        "root",
		AgentID:      "agent-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		ConfigJSON:   `{"type":"feishu"}`,
		MetadataJSON: `{"region":"cn"}`,
		CreatedAt:    "2025-01-01T00:00:00Z",
		UpdatedAt:    "2025-01-02T00:00:00Z",
		DeletedAt:    "",
	}

	got := service.BizChannelToProto(ch)
	if got.Id != ch.ID {
		t.Errorf("Id = %q, want %q", got.Id, ch.ID)
	}
	if got.Resource != ch.Resource {
		t.Errorf("Resource = %q, want %q", got.Resource, ch.Resource)
	}
	if got.Key != ch.Key {
		t.Errorf("Key = %q, want %q", got.Key, ch.Key)
	}
	if got.Name != ch.Name {
		t.Errorf("Name = %q, want %q", got.Name, ch.Name)
	}
	if got.Description != ch.Description {
		t.Errorf("Description = %q, want %q", got.Description, ch.Description)
	}
	if got.Status != ch.Status {
		t.Errorf("Status = %q, want %q", got.Status, ch.Status)
	}
	if got.Enabled != ch.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, ch.Enabled)
	}
	if got.SortOrder != int32(ch.SortOrder) {
		t.Errorf("SortOrder = %d, want %d", got.SortOrder, ch.SortOrder)
	}
	if got.AgentId != ch.AgentID {
		t.Errorf("AgentId = %q, want %q", got.AgentId, ch.AgentID)
	}
	if got.Provider != ch.Provider {
		t.Errorf("Provider = %q, want %q", got.Provider, ch.Provider)
	}
	if got.Model != ch.Model {
		t.Errorf("Model = %q, want %q", got.Model, ch.Model)
	}
	if got.ConfigJson != ch.ConfigJSON {
		t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, ch.ConfigJSON)
	}
	if got.MetadataJson != ch.MetadataJSON {
		t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, ch.MetadataJSON)
	}
}

func TestChannelRowToProto_WithRuntimeMeta(t *testing.T) {
	ch := biz.Channel{
		ID:           "ch-2",
		MetadataJSON: `{"region":"cn"}`,
	}
	runtimeMeta := `{"runtime_connected":true}`

	got := service.ChannelRowToProto(ch, runtimeMeta)
	if got.Id != "ch-2" {
		t.Errorf("Id = %q, want %q", got.Id, "ch-2")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(got.MetadataJson), &meta); err != nil {
		t.Fatalf("MetadataJson not valid JSON: %q", got.MetadataJson)
	}
	if meta["region"] != "cn" {
		t.Errorf("region = %v, want %q", meta["region"], "cn")
	}
	if meta["runtime_connected"] != true {
		t.Errorf("runtime_connected = %v, want true", meta["runtime_connected"])
	}
}

func TestChannelRowToProto_EmptyRuntimeMeta(t *testing.T) {
	ch := biz.Channel{
		ID:           "ch-3",
		MetadataJSON: `{"key":"val"}`,
	}

	got := service.ChannelRowToProto(ch, "")
	if got.MetadataJson != `{"key":"val"}` {
		t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, `{"key":"val"}`)
	}
}

func TestBizTypeItemToProto(t *testing.T) {
	item := biz.ChannelTypeItem{
		Type:             "feishu",
		Label:            "飞书",
		Description:      "Feishu/Lark Bot",
		Group:            "国内",
		ReceiveModes:     []string{"webhook"},
		Icon:             "feishu",
		Bundled:          true,
		SupportsTest:     true,
		SupportsWebhook:  true,
		ConfigSchema:     map[string]any{"type": "object"},
		CredentialSchema: map[string]any{"type": "object", "required": []string{"app_secret"}},
		UIHints:          map[string]any{"group": "国内"},
		SortOrder:        10,
	}

	got, err := service.BizTypeItemToProto(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != "feishu" {
		t.Errorf("Type = %q, want %q", got.Type, "feishu")
	}
	if got.Label != "飞书" {
		t.Errorf("Label = %q, want %q", got.Label, "飞书")
	}
	if got.Description != "Feishu/Lark Bot" {
		t.Errorf("Description = %q, want %q", got.Description, "Feishu/Lark Bot")
	}
	if got.Group != "国内" {
		t.Errorf("Group = %q, want %q", got.Group, "国内")
	}
	if len(got.ReceiveModes) != 1 || got.ReceiveModes[0] != "webhook" {
		t.Errorf("ReceiveModes = %v, want [webhook]", got.ReceiveModes)
	}
	if got.Icon != "feishu" {
		t.Errorf("Icon = %q, want %q", got.Icon, "feishu")
	}
	if got.Bundled != true {
		t.Errorf("Bundled = %v, want true", got.Bundled)
	}
	if got.SupportsTest != true {
		t.Errorf("SupportsTest = %v, want true", got.SupportsTest)
	}
	if got.SupportsWebhook != true {
		t.Errorf("SupportsWebhook = %v, want true", got.SupportsWebhook)
	}
	if got.SortOrder != 10 {
		t.Errorf("SortOrder = %d, want 10", got.SortOrder)
	}

	var cfgSchema map[string]any
	if err := json.Unmarshal([]byte(got.ConfigSchemaJson), &cfgSchema); err != nil {
		t.Fatalf("ConfigSchemaJson not valid JSON: %q", got.ConfigSchemaJson)
	}
	if cfgSchema["type"] != "object" {
		t.Errorf("ConfigSchemaJson.type = %v, want object", cfgSchema["type"])
	}
}

func TestBizCredToProto(t *testing.T) {
	cred := biz.ChannelCredential{
		ID:            "cred-1",
		ChannelID:     "ch-1",
		CredentialKey: "app_secret",
		Status:        "active",
		MetadataJSON:  `{"note":"test"}`,
		Configured:    true,
		MaskedPreview: "en***xyz",
		CreatedAt:     "2025-01-01T00:00:00Z",
		UpdatedAt:     "2025-01-02T00:00:00Z",
	}

	got := service.BizCredToProto(cred)
	if got.Id != "cred-1" {
		t.Errorf("Id = %q, want %q", got.Id, "cred-1")
	}
	if got.ChannelId != "ch-1" {
		t.Errorf("ChannelId = %q, want %q", got.ChannelId, "ch-1")
	}
	if got.CredentialKey != "app_secret" {
		t.Errorf("CredentialKey = %q, want %q", got.CredentialKey, "app_secret")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.MetadataJson != `{"note":"test"}` {
		t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, `{"note":"test"}`)
	}
	if got.Configured != true {
		t.Errorf("Configured = %v, want true", got.Configured)
	}
	if got.MaskedPreview != "en***xyz" {
		t.Errorf("MaskedPreview = %q, want %q", got.MaskedPreview, "en***xyz")
	}
	if got.CreatedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, "2025-01-01T00:00:00Z")
	}
	if got.UpdatedAt != "2025-01-02T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, "2025-01-02T00:00:00Z")
	}
}

func TestBizDeliveryToProto(t *testing.T) {
	d := biz.ChannelDelivery{
		ID:           "del-1",
		ChannelID:    "ch-1",
		AgentID:      "agent-1",
		Status:       "delivered",
		PayloadJSON:  `{"text":"hello"}`,
		ErrorMessage: "",
		CreatedAt:    "2025-01-01T00:00:00Z",
		UpdatedAt:    "2025-01-02T00:00:00Z",
	}

	got := service.BizDeliveryToProto(d)
	if got.Id != "del-1" {
		t.Errorf("Id = %q, want %q", got.Id, "del-1")
	}
	if got.ChannelId != "ch-1" {
		t.Errorf("ChannelId = %q, want %q", got.ChannelId, "ch-1")
	}
	if got.AgentId != "agent-1" {
		t.Errorf("AgentId = %q, want %q", got.AgentId, "agent-1")
	}
	if got.Status != "delivered" {
		t.Errorf("Status = %q, want %q", got.Status, "delivered")
	}
	if got.PayloadJson != `{"text":"hello"}` {
		t.Errorf("PayloadJson = %q, want %q", got.PayloadJson, `{"text":"hello"}`)
	}
	if got.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", got.ErrorMessage)
	}
}

func TestBizTurnJobToProto(t *testing.T) {
	job := biz.ChannelTurnJob{
		ID:               "job-1",
		ChannelID:        "ch-1",
		SessionID:        "sess-1",
		PeerID:           "peer-1",
		PeerKey:          "peer-key-1",
		IdempotencyKey:   "idem-1",
		Status:           "running",
		PreviewMessageID: "msg-1",
		ContentPreview:   "hello",
		AsyncTargetType:  "graph",
		AsyncTargetID:    "graph-1",
		ErrorMessage:     "",
		StartedAt:        "2025-01-01T00:00:00Z",
		FinishedAt:       "",
		CreatedAt:        "2025-01-01T00:00:00Z",
		UpdatedAt:        "2025-01-02T00:00:00Z",
	}

	got := service.BizTurnJobToProto(job)
	if got.Id != "job-1" {
		t.Errorf("Id = %q, want %q", got.Id, "job-1")
	}
	if got.ChannelId != "ch-1" {
		t.Errorf("ChannelId = %q, want %q", got.ChannelId, "ch-1")
	}
	if got.SessionId != "sess-1" {
		t.Errorf("SessionId = %q, want %q", got.SessionId, "sess-1")
	}
	if got.PeerId != "peer-1" {
		t.Errorf("PeerId = %q, want %q", got.PeerId, "peer-1")
	}
	if got.PeerKey != "peer-key-1" {
		t.Errorf("PeerKey = %q, want %q", got.PeerKey, "peer-key-1")
	}
	if got.IdempotencyKey != "idem-1" {
		t.Errorf("IdempotencyKey = %q, want %q", got.IdempotencyKey, "idem-1")
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want %q", got.Status, "running")
	}
	if got.PreviewMessageId != "msg-1" {
		t.Errorf("PreviewMessageId = %q, want %q", got.PreviewMessageId, "msg-1")
	}
	if got.ContentPreview != "hello" {
		t.Errorf("ContentPreview = %q, want %q", got.ContentPreview, "hello")
	}
	if got.AsyncTargetType != "graph" {
		t.Errorf("AsyncTargetType = %q, want %q", got.AsyncTargetType, "graph")
	}
	if got.AsyncTargetId != "graph-1" {
		t.Errorf("AsyncTargetId = %q, want %q", got.AsyncTargetId, "graph-1")
	}
	if got.StartedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("StartedAt = %q, want %q", got.StartedAt, "2025-01-01T00:00:00Z")
	}
}

func TestBizTurnJobToProto_UTF8Sanitization(t *testing.T) {
	job := biz.ChannelTurnJob{
		ID:     "job-utf8",
		Status: "running\xed\xa0\x80",
	}

	got := service.BizTurnJobToProto(job)
	if got.Id != "job-utf8" {
		t.Errorf("Id = %q, want %q", got.Id, "job-utf8")
	}
}

func TestBizTestToProto(t *testing.T) {
	tests := []struct {
		name    string
		input   biz.ChannelTestResult
		wantOK  bool
		wantErr bool
	}{
		{
			name: "success_no_details",
			input: biz.ChannelTestResult{
				OK:      true,
				Status:  "ok",
				Message: "test passed",
			},
			wantOK:  true,
			wantErr: false,
		},
		{
			name: "failure_with_details",
			input: biz.ChannelTestResult{
				OK:      false,
				Status:  "error",
				Message: "connection failed",
				Details: map[string]any{"code": 10003, "msg": "invalid app_id"},
			},
			wantOK:  false,
			wantErr: false,
		},
		{
			name: "success_empty_details",
			input: biz.ChannelTestResult{
				OK:      true,
				Status:  "ok",
				Message: "ok",
				Details: nil,
			},
			wantOK:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.BizTestToProto(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got.Ok != tt.wantOK {
				t.Errorf("Ok = %v, want %v", got.Ok, tt.wantOK)
			}
			if got.Status != tt.input.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.input.Status)
			}
			if got.Message != tt.input.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.input.Message)
			}
			if tt.input.Details != nil {
				if got.DetailsJson == "" {
					t.Error("DetailsJson is empty, want non-empty")
				}
			} else {
				if got.DetailsJson != "" {
					t.Errorf("DetailsJson = %q, want empty", got.DetailsJson)
				}
			}
		})
	}
}

func TestProtoCredInputs(t *testing.T) {
	tests := []struct {
		name  string
		input []*v1.ChannelCredentialInput
		want  int
	}{
		{
			name:  "nil_input",
			input: nil,
			want:  0,
		},
		{
			name:  "empty_input",
			input: []*v1.ChannelCredentialInput{},
			want:  0,
		},
		{
			name: "single_input",
			input: []*v1.ChannelCredentialInput{
				{CredentialKey: "app_secret", Secret: "abc123", Status: "active"},
			},
			want: 1,
		},
		{
			name: "multiple_inputs",
			input: []*v1.ChannelCredentialInput{
				{CredentialKey: "app_secret", Secret: "abc123", SecretRef: "enc:xyz", Status: "active", MetadataJson: `{"note":"test"}`},
				{CredentialKey: "bot_token", Secret: "tok456", Status: "active"},
			},
			want: 2,
		},
		{
			name: "nil_element_skipped",
			input: []*v1.ChannelCredentialInput{
				{CredentialKey: "app_secret", Secret: "abc"},
				nil,
				{CredentialKey: "bot_token", Secret: "tok"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ProtoCredInputs(tt.input)
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			if tt.name == "multiple_inputs" {
				if got[0].CredentialKey != "app_secret" {
					t.Errorf("CredentialKey = %q, want %q", got[0].CredentialKey, "app_secret")
				}
				if got[0].SecretRef != "enc:xyz" {
					t.Errorf("SecretRef = %q, want %q", got[0].SecretRef, "enc:xyz")
				}
				if got[0].MetadataJSON != `{"note":"test"}` {
					t.Errorf("MetadataJSON = %q, want %q", got[0].MetadataJSON, `{"note":"test"}`)
				}
				if got[1].CredentialKey != "bot_token" {
					t.Errorf("CredentialKey = %q, want %q", got[1].CredentialKey, "bot_token")
				}
			}
		})
	}
}

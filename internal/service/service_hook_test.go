package service_test

import (
	"testing"

	hookv1 "aranea-agents/api/kratos/hook/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/hook"
	"aranea-agents/internal/service"
)

func TestToProtoHook(t *testing.T) {
	h := biz.Hook{
		ID:           "hook-1",
		Key:          "on_run_complete",
		Name:         "Run Complete Hook",
		Description:  "Fires when a run completes",
		Status:       "active",
		Enabled:      true,
		SortOrder:    1,
		ConfigJSON:   `{"url":"https://example.com"}`,
		MetadataJSON: `{"source":"system"}`,
		CreatedAt:    "2024-01-01",
		UpdatedAt:    "2024-06-01",
		DeletedAt:    "",
	}
	pb := service.ToProtoHook(h)
	if pb.GetId() != "hook-1" || pb.GetKey() != "on_run_complete" {
		t.Fatalf("id/key mismatch: id=%q key=%q", pb.GetId(), pb.GetKey())
	}
	if pb.GetName() != "Run Complete Hook" || pb.GetDescription() != "Fires when a run completes" {
		t.Fatalf("name/desc mismatch: name=%q desc=%q", pb.GetName(), pb.GetDescription())
	}
	if pb.GetStatus() != "active" || !pb.GetEnabled() {
		t.Fatalf("status/enabled mismatch: status=%q enabled=%v", pb.GetStatus(), pb.GetEnabled())
	}
	if pb.GetSortOrder() != 1 {
		t.Fatalf("sort_order mismatch: %d", pb.GetSortOrder())
	}
	if pb.GetConfigJson() != `{"url":"https://example.com"}` {
		t.Fatalf("config mismatch: %q", pb.GetConfigJson())
	}
	if pb.GetMetadataJson() != `{"source":"system"}` {
		t.Fatalf("metadata mismatch: %q", pb.GetMetadataJson())
	}
	if pb.GetCreatedAt() != "2024-01-01" || pb.GetUpdatedAt() != "2024-06-01" {
		t.Fatalf("timestamps mismatch: created=%q updated=%q", pb.GetCreatedAt(), pb.GetUpdatedAt())
	}
}

func TestToProtoHook_DeletedAt(t *testing.T) {
	h := biz.Hook{
		ID:        "hook-2",
		Key:       "deleted_hook",
		DeletedAt: "2024-03-01",
	}
	pb := service.ToProtoHook(h)
	if pb.GetDeletedAt() != "2024-03-01" {
		t.Fatalf("deleted_at mismatch: %q", pb.GetDeletedAt())
	}
}

func TestPatchFromProtoHook_Nil(t *testing.T) {
	got := service.PatchFromProtoHook(nil)
	if got.Key != "" || got.Name != "" {
		t.Fatalf("expected zero-value Hook, got %+v", got)
	}
}

func TestPatchFromProtoHook(t *testing.T) {
	pb := &hookv1.Hook{
		Key:          "on_error",
		Name:         "Error Hook",
		Description:  "Fires on error",
		Status:       "active",
		Enabled:      true,
		SortOrder:    5,
		ConfigJson:   `{"url":"https://err.example.com"}`,
		MetadataJson: `{"source":"user"}`,
	}
	h := service.PatchFromProtoHook(pb)
	if h.Key != "on_error" || h.Name != "Error Hook" {
		t.Fatalf("key/name mismatch: key=%q name=%q", h.Key, h.Name)
	}
	if h.Description != "Fires on error" || h.Status != "active" {
		t.Fatalf("desc/status mismatch: desc=%q status=%q", h.Description, h.Status)
	}
	if !h.Enabled || h.SortOrder != 5 {
		t.Fatalf("enabled/sort mismatch: enabled=%v sort=%d", h.Enabled, h.SortOrder)
	}
	if h.ConfigJSON != `{"url":"https://err.example.com"}` {
		t.Fatalf("config mismatch: %q", h.ConfigJSON)
	}
	if h.MetadataJSON != `{"source":"user"}` {
		t.Fatalf("metadata mismatch: %q", h.MetadataJSON)
	}
}

func TestPatchFromProtoHook_NoID(t *testing.T) {
	pb := &hookv1.Hook{
		Key:  "test",
		Name: "Test",
	}
	h := service.PatchFromProtoHook(pb)
	if h.ID != "" {
		t.Fatalf("patch should not set ID, got %q", h.ID)
	}
	if h.CreatedAt != "" || h.UpdatedAt != "" || h.DeletedAt != "" {
		t.Fatalf("patch should not set timestamps: created=%q updated=%q deleted=%q", h.CreatedAt, h.UpdatedAt, h.DeletedAt)
	}
}

func TestToProtoHookDelivery(t *testing.T) {
	d := biz.HookDelivery{
		ID:            "del-1",
		HookKey:       "on_run_complete",
		HookID:        "hook-1",
		WebhookURL:    "https://example.com/webhook",
		PayloadJSON:   `{"event":"run.completed"}`,
		Status:        hook.DeliverySuccess,
		AttemptCount:  1,
		MaxAttempts:   3,
		LastError:     "",
		CreatedAt:     "2024-01-01T10:00:00Z",
		UpdatedAt:     "2024-01-01T10:00:01Z",
	}
	pb := service.ToProtoHookDelivery(d)
	if pb.GetId() != "del-1" || pb.GetHookKey() != "on_run_complete" {
		t.Fatalf("id/hook_key mismatch: id=%q key=%q", pb.GetId(), pb.GetHookKey())
	}
	if pb.GetHookId() != "hook-1" {
		t.Fatalf("hook_id mismatch: %q", pb.GetHookId())
	}
	if pb.GetWebhookUrl() != "https://example.com/webhook" {
		t.Fatalf("webhook_url mismatch: %q", pb.GetWebhookUrl())
	}
	if pb.GetPayloadJson() != `{"event":"run.completed"}` {
		t.Fatalf("payload mismatch: %q", pb.GetPayloadJson())
	}
	if pb.GetStatus() != "success" {
		t.Fatalf("status mismatch: %q", pb.GetStatus())
	}
	if pb.GetAttemptCount() != 1 || pb.GetMaxAttempts() != 3 {
		t.Fatalf("attempt mismatch: current=%d max=%d", pb.GetAttemptCount(), pb.GetMaxAttempts())
	}
	if pb.GetLastError() != "" {
		t.Fatalf("last_error should be empty: %q", pb.GetLastError())
	}
}

func TestToProtoHookDelivery_FailedStatus(t *testing.T) {
	d := biz.HookDelivery{
		ID:           "del-2",
		HookKey:      "on_error",
		Status:       hook.DeliveryFailed,
		AttemptCount: 3,
		MaxAttempts:  3,
		LastError:    "connection refused",
	}
	pb := service.ToProtoHookDelivery(d)
	if pb.GetStatus() != "failed" {
		t.Fatalf("status mismatch: %q", pb.GetStatus())
	}
	if pb.GetLastError() != "connection refused" {
		t.Fatalf("last_error mismatch: %q", pb.GetLastError())
	}
}

func TestToProtoHookDelivery_PendingStatus(t *testing.T) {
	d := biz.HookDelivery{
		ID:           "del-3",
		HookKey:      "pending_hook",
		Status:       hook.DeliveryPending,
		AttemptCount: 0,
		MaxAttempts:  3,
	}
	pb := service.ToProtoHookDelivery(d)
	if pb.GetStatus() != "pending" {
		t.Fatalf("status mismatch: %q", pb.GetStatus())
	}
	if pb.GetAttemptCount() != 0 {
		t.Fatalf("attempt_count mismatch: %d", pb.GetAttemptCount())
	}
}

func TestToProtoHook_FieldsComplete(t *testing.T) {
	h := biz.Hook{
		ID:           "h1",
		Key:          "k1",
		Name:         "N1",
		Description:  "D1",
		Status:       "S1",
		Enabled:      false,
		SortOrder:    10,
		ConfigJSON:   "{}",
		MetadataJSON: "{}",
		CreatedAt:    "c",
		UpdatedAt:    "u",
		DeletedAt:    "d",
	}
	pb := service.ToProtoHook(h)
	if pb.GetId() != "h1" || pb.GetKey() != "k1" || pb.GetName() != "N1" {
		t.Fatalf("basic fields mismatch")
	}
	if pb.GetDescription() != "D1" || pb.GetStatus() != "S1" || pb.GetEnabled() {
		t.Fatalf("desc/status/enabled mismatch")
	}
	if pb.GetSortOrder() != 10 || pb.GetCreatedAt() != "c" || pb.GetUpdatedAt() != "u" || pb.GetDeletedAt() != "d" {
		t.Fatalf("sort/timestamps mismatch")
	}
}

func TestPatchFromProtoHook_Disabled(t *testing.T) {
	pb := &hookv1.Hook{
		Key:     "disabled_hook",
		Enabled: false,
	}
	h := service.PatchFromProtoHook(pb)
	if h.Enabled {
		t.Fatal("expected enabled=false")
	}
}

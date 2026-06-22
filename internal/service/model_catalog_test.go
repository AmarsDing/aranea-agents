package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/model_catalog/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubRootResolver struct {
	dir string
	err error
}

func (s *stubRootResolver) GetRootDirectory(_ context.Context) (string, error) {
	return s.dir, s.err
}

func newTestUsecase(t *testing.T) (*biz.ModelRegistryUsecase, string) {
	t.Helper()
	dir := t.TempDir()
	roots := &stubRootResolver{dir: dir}
	uc := biz.NewModelRegistryUsecase(roots, nil, loggateway.NewNoop())
	return uc, dir
}

func seedStore(t *testing.T, dir string, cat modelregistry.Directory, meta modelregistry.Meta, policy modelregistry.Policy) {
	t.Helper()
	st := modelregistry.NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}
}

func defaultTestPolicy() modelregistry.Policy {
	return modelregistry.Policy{
		SourceURL:         "https://models.dev/api.json",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 24,
		AutoApply:         "metadata_and_pricing",
	}
}

func defaultTestMeta() modelregistry.Meta {
	return modelregistry.Meta{
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
		ETag:          `"test-etag"`,
		SHA256:        "abc123",
		SourceURL:     "https://models.dev/api.json",
		ProviderCount: 2,
		ModelCount:    5,
		Bytes:         4096,
	}
}

func defaultTestDirectory() modelregistry.Directory {
	return modelregistry.Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Doc:  "https://openai.com",
			Npm:  "@openai/sdk",
			API:  "https://api.openai.com",
			Env:  []string{"OPENAI_API_KEY"},
			Models: map[string]modelregistry.Model{
				"gpt-4o": {
					ID:        "gpt-4o",
					Name:      "GPT-4o",
					Status:    "active",
					Reasoning: true,
					ToolCall:  true,
					Limit:     modelregistry.ModelLimit{Context: 128000, Output: 16384},
					Modalities: modelregistry.Modalities{
						Input:  []string{"text", "image"},
						Output: []string{"text"},
					},
					Family:      "gpt-4o",
					ReleaseDate: "2024-05-13",
					LastUpdated: "2024-06-01",
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Doc:  "https://anthropic.com",
			Env:  []string{"ANTHROPIC_API_KEY"},
			Models: map[string]modelregistry.Model{
				"claude-3-opus": {
					ID:        "claude-3-opus",
					Name:      "Claude 3 Opus",
					Status:    "active",
					Reasoning: true,
					ToolCall:  true,
					Limit:     modelregistry.ModelLimit{Context: 200000, Output: 4096},
					Modalities: modelregistry.Modalities{
						Input:  []string{"text", "image"},
						Output: []string{"text"},
					},
					Family:      "claude-3",
					ReleaseDate: "2024-03-04",
					LastUpdated: "2024-04-01",
				},
			},
		},
	}
}

func TestModelCatalogService_GetModelCatalogStatus(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	meta := defaultTestMeta()
	cat := defaultTestDirectory()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetModelCatalogStatus(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetCatalogBytes() <= 0 {
		t.Errorf("CatalogBytes: got %d, want > 0", resp.GetCatalogBytes())
	}
	if !resp.GetCatalogLoaded() {
		t.Error("CatalogLoaded: expected true")
	}
	if resp.GetProviderCount() != int32(meta.ProviderCount) {
		t.Errorf("ProviderCount: got %d, want %d", resp.GetProviderCount(), meta.ProviderCount)
	}
	if resp.GetModelCount() != int32(meta.ModelCount) {
		t.Errorf("ModelCount: got %d, want %d", resp.GetModelCount(), meta.ModelCount)
	}
	if resp.GetEtag() != meta.ETag {
		t.Errorf("Etag: got %q, want %q", resp.GetEtag(), meta.ETag)
	}
	if resp.GetPolicy() == nil {
		t.Fatal("Policy should not be nil")
	}
	if resp.GetPolicy().GetSourceUrl() != policy.SourceURL {
		t.Errorf("Policy.SourceUrl: got %q, want %q", resp.GetPolicy().GetSourceUrl(), policy.SourceURL)
	}
	if resp.GetPolicy().GetSyncPolicy() != policy.SyncPolicy {
		t.Errorf("Policy.SyncPolicy: got %q, want %q", resp.GetPolicy().GetSyncPolicy(), policy.SyncPolicy)
	}
	if resp.GetPolicy().GetSyncIntervalHours() != int32(policy.SyncIntervalHours) {
		t.Errorf("Policy.SyncIntervalHours: got %d, want %d", resp.GetPolicy().GetSyncIntervalHours(), policy.SyncIntervalHours)
	}
	if resp.GetPolicy().GetAutoApply() != policy.AutoApply {
		t.Errorf("Policy.AutoApply: got %q, want %q", resp.GetPolicy().GetAutoApply(), policy.AutoApply)
	}
	if resp.GetLastSyncAt() == nil {
		t.Error("LastSyncAt should not be nil for valid RFC3339")
	}
}

func TestModelCatalogService_GetModelCatalogPolicy(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetModelCatalogPolicy(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSourceUrl() != policy.SourceURL {
		t.Errorf("SourceUrl: got %q, want %q", resp.GetSourceUrl(), policy.SourceURL)
	}
	if resp.GetSyncPolicy() != policy.SyncPolicy {
		t.Errorf("SyncPolicy: got %q, want %q", resp.GetSyncPolicy(), policy.SyncPolicy)
	}
	if resp.GetSyncIntervalHours() != int32(policy.SyncIntervalHours) {
		t.Errorf("SyncIntervalHours: got %d, want %d", resp.GetSyncIntervalHours(), policy.SyncIntervalHours)
	}
	if resp.GetAutoApply() != policy.AutoApply {
		t.Errorf("AutoApply: got %q, want %q", resp.GetAutoApply(), policy.AutoApply)
	}
}

func TestModelCatalogService_UpdateModelCatalogPolicy(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	req := &v1.UpdateModelCatalogPolicyRequest{
		SourceUrl:         "https://models.dev/api.json",
		SyncPolicy:        "off",
		SyncIntervalHours: 12,
		AutoApply:         "none",
	}
	resp, err := svc.UpdateModelCatalogPolicy(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSourceUrl() != req.GetSourceUrl() {
		t.Errorf("SourceUrl: got %q, want %q", resp.GetSourceUrl(), req.GetSourceUrl())
	}
	if resp.GetSyncPolicy() != req.GetSyncPolicy() {
		t.Errorf("SyncPolicy: got %q, want %q", resp.GetSyncPolicy(), req.GetSyncPolicy())
	}
	if resp.GetSyncIntervalHours() != req.GetSyncIntervalHours() {
		t.Errorf("SyncIntervalHours: got %d, want %d", resp.GetSyncIntervalHours(), req.GetSyncIntervalHours())
	}
	if resp.GetAutoApply() != req.GetAutoApply() {
		t.Errorf("AutoApply: got %q, want %q", resp.GetAutoApply(), req.GetAutoApply())
	}
}

func TestModelCatalogService_SyncModelCatalog_Success(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.SyncModelCatalog(context.Background(), &v1.SyncModelCatalogRequest{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetOk() {
		t.Errorf("Ok: got %v, want true", resp.GetOk())
	}
	if resp.GetLogId() == "" {
		t.Error("LogId should not be empty")
	}
	if resp.GetStatus() == nil {
		t.Error("Status should not be nil")
	}
}

func TestModelCatalogService_SyncModelCatalog_Error(t *testing.T) {
	roots := &stubRootResolver{err: errors.New("root directory unavailable")}
	uc := biz.NewModelRegistryUsecase(roots, nil, loggateway.NewNoop())
	svc := NewModelCatalogService(uc)

	resp, err := svc.SyncModelCatalog(context.Background(), &v1.SyncModelCatalogRequest{})
	if err != nil {
		t.Fatalf("SyncModelCatalog should not return error (it wraps in resp), got: %v", err)
	}
	if resp.GetOk() {
		t.Error("Ok should be false on error")
	}
	if resp.GetMessage() == "" {
		t.Error("Message should contain error info")
	}
}

func TestModelCatalogService_ListCatalogProviders(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListCatalogProviders(context.Background(), &v1.ListCatalogProvidersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTotal() != 2 {
		t.Errorf("Total: got %d, want 2", resp.GetTotal())
	}
	if len(resp.GetItems()) != 2 {
		t.Fatalf("Items count: got %d, want 2", len(resp.GetItems()))
	}
	var openaiItem *v1.CatalogProviderSummary
	for _, item := range resp.GetItems() {
		if item.GetId() == "openai" {
			openaiItem = item
			break
		}
	}
	if openaiItem == nil {
		t.Fatal("openai provider not found")
	}
	if openaiItem.GetName() != "OpenAI" {
		t.Errorf("Name: got %q, want %q", openaiItem.GetName(), "OpenAI")
	}
	if openaiItem.GetDoc() != "https://openai.com" {
		t.Errorf("Doc: got %q, want %q", openaiItem.GetDoc(), "https://openai.com")
	}
	if openaiItem.GetNpm() != "@openai/sdk" {
		t.Errorf("Npm: got %q, want %q", openaiItem.GetNpm(), "@openai/sdk")
	}
	if openaiItem.GetApi() != "https://api.openai.com" {
		t.Errorf("Api: got %q, want %q", openaiItem.GetApi(), "https://api.openai.com")
	}
	if openaiItem.GetModelCount() != 1 {
		t.Errorf("ModelCount: got %d, want 1", openaiItem.GetModelCount())
	}
	if len(openaiItem.GetEnv()) != 1 || openaiItem.GetEnv()[0] != "OPENAI_API_KEY" {
		t.Errorf("Env: got %v, want [OPENAI_API_KEY]", openaiItem.GetEnv())
	}
}

func TestModelCatalogService_ListCatalogModels(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListCatalogModels(context.Background(), &v1.ListCatalogModelsRequest{
		ProviderId: "openai",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTotal() != 1 {
		t.Errorf("Total: got %d, want 1", resp.GetTotal())
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("Items count: got %d, want 1", len(resp.GetItems()))
	}
	m := resp.GetItems()[0]
	if m.GetId() != "gpt-4o" {
		t.Errorf("Id: got %q, want %q", m.GetId(), "gpt-4o")
	}
	if m.GetName() != "GPT-4o" {
		t.Errorf("Name: got %q, want %q", m.GetName(), "GPT-4o")
	}
	if m.GetStatus() != "active" {
		t.Errorf("Status: got %q, want %q", m.GetStatus(), "active")
	}
	if !m.GetReasoning() {
		t.Error("Reasoning: expected true")
	}
	if !m.GetToolCall() {
		t.Error("ToolCall: expected true")
	}
	if m.GetContextTokens() != 128000 {
		t.Errorf("ContextTokens: got %d, want 128000", m.GetContextTokens())
	}
	if m.GetOutputTokens() != 16384 {
		t.Errorf("OutputTokens: got %d, want 16384", m.GetOutputTokens())
	}
	if m.GetFamily() != "gpt-4o" {
		t.Errorf("Family: got %q, want %q", m.GetFamily(), "gpt-4o")
	}
	if m.GetReleaseDate() != "2024-05-13" {
		t.Errorf("ReleaseDate: got %q, want %q", m.GetReleaseDate(), "2024-05-13")
	}
	if m.GetLastUpdated() != "2024-06-01" {
		t.Errorf("LastUpdated: got %q, want %q", m.GetLastUpdated(), "2024-06-01")
	}
	if len(m.GetModalityInput()) != 2 {
		t.Errorf("ModalityInput: got %v, want 2 items", m.GetModalityInput())
	}
	if len(m.GetModalityOutput()) != 1 {
		t.Errorf("ModalityOutput: got %v, want 1 item", m.GetModalityOutput())
	}
}

func TestModelCatalogService_ListCatalogModels_OptionalFields(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	meta := defaultTestMeta()

	structuredOut := true
	temperature := false
	cat := modelregistry.Directory{
		"testprov": {
			ID:   "testprov",
			Name: "Test",
			Models: map[string]modelregistry.Model{
				"model-a": {
					ID:               "model-a",
					Name:             "Model A",
					Status:           "active",
					StructuredOutput: &structuredOut,
					Temperature:      &temperature,
					OpenWeights:      true,
					Cost: &modelregistry.ModelCost{
						Input:      5.0,
						Output:     15.0,
						CacheRead:  1.0,
						CacheWrite: 2.5,
						Reasoning:  3.0,
					},
					Limit: modelregistry.ModelLimit{Context: 100000, Output: 8192},
					Modalities: modelregistry.Modalities{
						Input:  []string{"text"},
						Output: []string{"text"},
					},
				},
			},
		},
	}
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListCatalogModels(context.Background(), &v1.ListCatalogModelsRequest{
		ProviderId: "testprov",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("Items count: got %d, want 1", len(resp.GetItems()))
	}
	m := resp.GetItems()[0]
	if !m.GetStructuredOutput() {
		t.Error("StructuredOutput: expected true")
	}
	if m.GetTemperature() {
		t.Error("Temperature: expected false")
	}
	if !m.GetOpenWeights() {
		t.Error("OpenWeights: expected true")
	}
	if m.GetInputUsdPer_1M() != 5.0 {
		t.Errorf("InputUsdPer_1M: got %f, want 5.0", m.GetInputUsdPer_1M())
	}
	if m.GetOutputUsdPer_1M() != 15.0 {
		t.Errorf("OutputUsdPer_1M: got %f, want 15.0", m.GetOutputUsdPer_1M())
	}
	if m.GetCacheReadUsdPer_1M() != 1.0 {
		t.Errorf("CacheReadUsdPer_1M: got %f, want 1.0", m.GetCacheReadUsdPer_1M())
	}
	if m.GetCacheWriteUsdPer_1M() != 2.5 {
		t.Errorf("CacheWriteUsdPer_1M: got %f, want 2.5", m.GetCacheWriteUsdPer_1M())
	}
	if m.GetReasoningUsdPer_1M() != 3.0 {
		t.Errorf("ReasoningUsdPer_1M: got %f, want 3.0", m.GetReasoningUsdPer_1M())
	}
}

func TestModelCatalogService_ListCatalogModels_InterleavedJson(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	meta := defaultTestMeta()

	cat := modelregistry.Directory{
		"prov": {
			ID:   "prov",
			Name: "Prov",
			Models: map[string]modelregistry.Model{
				"m1": {
					ID:          "m1",
					Name:        "M1",
					Interleaved: json.RawMessage(`["text","image"]`),
					Limit:       modelregistry.ModelLimit{Context: 100, Output: 50},
					Modalities:  modelregistry.Modalities{Input: []string{"text"}, Output: []string{"text"}},
				},
				"m2": {
					ID:          "m2",
					Name:        "M2",
					Interleaved: json.RawMessage(`null`),
					Limit:       modelregistry.ModelLimit{Context: 100, Output: 50},
					Modalities:  modelregistry.Modalities{Input: []string{"text"}, Output: []string{"text"}},
				},
				"m3": {
					ID:         "m3",
					Name:       "M3",
					Limit:      modelregistry.ModelLimit{Context: 100, Output: 50},
					Modalities: modelregistry.Modalities{Input: []string{"text"}, Output: []string{"text"}},
				},
			},
		},
	}
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListCatalogModels(context.Background(), &v1.ListCatalogModelsRequest{
		ProviderId: "prov",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := resp.GetItems()
	if len(items) != 3 {
		t.Fatalf("Items count: got %d, want 3", len(items))
	}
	var m1, m2, m3 *v1.CatalogModelSummary
	for _, item := range items {
		switch item.GetId() {
		case "m1":
			m1 = item
		case "m2":
			m2 = item
		case "m3":
			m3 = item
		}
	}
	if m1.GetInterleavedJson() != `["text","image"]` {
		t.Errorf("m1 InterleavedJson: got %q, want %q", m1.GetInterleavedJson(), `["text","image"]`)
	}
	if m2.GetInterleavedJson() != "" {
		t.Errorf("m2 InterleavedJson: got %q, want empty (null JSON should be skipped)", m2.GetInterleavedJson())
	}
	if m3.GetInterleavedJson() != "" {
		t.Errorf("m3 InterleavedJson: got %q, want empty (no interleaved)", m3.GetInterleavedJson())
	}
}

func TestToProtoCatalogPolicy(t *testing.T) {
	p := modelregistry.Policy{
		SourceURL:         "https://example.com",
		SyncPolicy:        "manual",
		SyncIntervalHours: 6,
		AutoApply:         "full_spec",
	}
	got := toProtoCatalogPolicy(p)
	if got.GetSourceUrl() != p.SourceURL {
		t.Errorf("SourceUrl: got %q, want %q", got.GetSourceUrl(), p.SourceURL)
	}
	if got.GetSyncPolicy() != p.SyncPolicy {
		t.Errorf("SyncPolicy: got %q, want %q", got.GetSyncPolicy(), p.SyncPolicy)
	}
	if got.GetSyncIntervalHours() != int32(p.SyncIntervalHours) {
		t.Errorf("SyncIntervalHours: got %d, want %d", got.GetSyncIntervalHours(), p.SyncIntervalHours)
	}
	if got.GetAutoApply() != p.AutoApply {
		t.Errorf("AutoApply: got %q, want %q", got.GetAutoApply(), p.AutoApply)
	}
}

func TestToProtoCatalogStatus(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	view := biz.ModelRegistryStatusView{
		Policy: modelregistry.Policy{
			SourceURL:         "https://example.com",
			SyncPolicy:        "scheduled",
			SyncIntervalHours: 12,
			AutoApply:         "none",
		},
		Meta: modelregistry.Meta{
			SyncedAt:      now,
			ETag:          `"etag-123"`,
			ProviderCount: 3,
			ModelCount:    10,
			Bytes:         8192,
		},
		DirectoryLoaded: true,
		LocalPath:       "/data/model-catalog",
		LastSyncStatus:  "ok",
		LastSyncSummary: "synced 3 providers",
	}
	got := toProtoCatalogStatus(view)
	if got.GetCatalogBytes() != 8192 {
		t.Errorf("CatalogBytes: got %d, want 8192", got.GetCatalogBytes())
	}
	if !got.GetCatalogLoaded() {
		t.Error("CatalogLoaded: expected true")
	}
	if got.GetEtag() != `"etag-123"` {
		t.Errorf("Etag: got %q, want %q", got.GetEtag(), `"etag-123"`)
	}
	if got.GetProviderCount() != 3 {
		t.Errorf("ProviderCount: got %d, want 3", got.GetProviderCount())
	}
	if got.GetModelCount() != 10 {
		t.Errorf("ModelCount: got %d, want 10", got.GetModelCount())
	}
	if got.GetLocalPath() != "/data/model-catalog" {
		t.Errorf("LocalPath: got %q, want %q", got.GetLocalPath(), "/data/model-catalog")
	}
	if got.GetLastSyncStatus() != "ok" {
		t.Errorf("LastSyncStatus: got %q, want %q", got.GetLastSyncStatus(), "ok")
	}
	if got.GetLastSyncSummary() != "synced 3 providers" {
		t.Errorf("LastSyncSummary: got %q, want %q", got.GetLastSyncSummary(), "synced 3 providers")
	}
	if got.GetPolicy() == nil {
		t.Fatal("Policy should not be nil")
	}
	if got.GetPolicy().GetSourceUrl() != "https://example.com" {
		t.Errorf("Policy.SourceUrl: got %q", got.GetPolicy().GetSourceUrl())
	}
	if got.GetLastSyncAt() == nil {
		t.Error("LastSyncAt should not be nil for valid RFC3339")
	}
}

func TestToProtoCatalogStatus_EmptySyncedAt(t *testing.T) {
	view := biz.ModelRegistryStatusView{
		Policy:          modelregistry.Policy{},
		Meta:            modelregistry.Meta{SyncedAt: ""},
		DirectoryLoaded: false,
	}
	got := toProtoCatalogStatus(view)
	if got.GetLastSyncAt() != nil {
		t.Errorf("LastSyncAt should be nil for empty SyncedAt, got %v", got.GetLastSyncAt())
	}
}

func TestParseCatalogTS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"invalid format", "not-a-date", true},
		{"valid RFC3339", "2024-06-01T12:00:00Z", false},
		{"valid RFC3339 with timezone", "2024-06-01T12:00:00+08:00", false},
		{"whitespace padded valid", " 2024-06-01T12:00:00Z ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCatalogTS(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseCatalogTS(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseCatalogTS(%q) = nil, want non-nil", tt.input)
			}
			parsed, err := time.Parse(time.RFC3339, tt.input)
			if err != nil {
				parsed, _ = time.Parse(time.RFC3339, "2024-06-01T12:00:00Z")
			}
			want := timestamppb.New(parsed)
			if got.Seconds != want.Seconds {
				t.Errorf("Seconds: got %d, want %d", got.Seconds, want.Seconds)
			}
		})
	}
}

func TestModelCatalogService_GetModelCatalogStatus_RootError(t *testing.T) {
	roots := &stubRootResolver{err: errors.New("unavailable")}
	uc := biz.NewModelRegistryUsecase(roots, nil, loggateway.NewNoop())
	svc := NewModelCatalogService(uc)

	_, err := svc.GetModelCatalogStatus(context.Background(), &emptypb.Empty{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestModelCatalogService_ListCatalogModels_EmptyProvider(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	meta := defaultTestMeta()
	cat := defaultTestDirectory()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListCatalogModels(context.Background(), &v1.ListCatalogModelsRequest{
		ProviderId: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTotal() != 0 {
		t.Errorf("Total: got %d, want 0", resp.GetTotal())
	}
	if len(resp.GetItems()) != 0 {
		t.Errorf("Items: got %d, want 0", len(resp.GetItems()))
	}
}

func TestModelCatalogService_SyncModelCatalog_DryRunPath(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.SyncModelCatalog(context.Background(), &v1.SyncModelCatalogRequest{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetOk() {
		t.Errorf("Ok: got %v, want true for dry-run", resp.GetOk())
	}
	if resp.GetLogId() == "" {
		t.Error("LogId should not be empty")
	}
}

func TestModelCatalogService_GetCatalogProviderLogo_Empty(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetCatalogProviderLogo(context.Background(), &v1.GetCatalogProviderLogoRequest{
		ProviderId: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSvg() != "" {
		t.Errorf("Svg: got %q, want empty for nonexistent logo", resp.GetSvg())
	}
	if resp.GetCached() {
		t.Error("Cached: expected false for nonexistent logo")
	}
}

func TestModelCatalogService_GetCatalogProviderLogo_WithSVG(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	logosDir := filepath.Join(dir, "data", "model-catalog", "logos")
	if err := os.MkdirAll(logosDir, 0o755); err != nil {
		t.Fatal(err)
	}
	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`
	if err := os.WriteFile(filepath.Join(logosDir, "openai.svg"), []byte(svgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetCatalogProviderLogo(context.Background(), &v1.GetCatalogProviderLogoRequest{
		ProviderId: "openai",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSvg() != svgContent {
		t.Errorf("Svg: got %q, want %q", resp.GetSvg(), svgContent)
	}
	if !resp.GetCached() {
		t.Error("Cached: expected true for existing logo file")
	}
}

func TestModelCatalogService_GetModelCatalogRaw(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetModelCatalogRaw(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetJsonPretty() == "" {
		t.Error("JsonPretty should not be empty")
	}
	if resp.GetBytes() <= 0 {
		t.Errorf("Bytes: got %d, want > 0", resp.GetBytes())
	}
}

func TestModelCatalogService_ListModelCatalogSyncLogs(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	st := modelregistry.NewStore(dir, loggateway.NewNoop())
	entry := modelregistry.SyncLogEntry{
		ID:         "sync-test-001",
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		FinishedAt: time.Now().UTC().Add(5 * time.Second).Format(time.RFC3339),
		Status:     "ok",
		Message:    "synced 2 providers",
	}
	if err := modelregistry.AppendSyncLog(st, entry); err != nil {
		t.Fatal(err)
	}

	svc := NewModelCatalogService(uc)
	resp, err := svc.ListModelCatalogSyncLogs(context.Background(), &v1.ListModelCatalogSyncLogsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetItems()) == 0 {
		t.Fatal("expected at least one sync log entry")
	}
	latest := resp.GetItems()[0]
	if latest.GetId() != "sync-test-001" {
		t.Errorf("Id: got %q, want %q", latest.GetId(), "sync-test-001")
	}
	if latest.GetStatus() != "ok" {
		t.Errorf("Status: got %q, want %q", latest.GetStatus(), "ok")
	}
	if latest.GetStartedAt() == nil {
		t.Error("StartedAt should not be nil")
	}
	if latest.GetFinishedAt() == nil {
		t.Error("FinishedAt should not be nil")
	}
}

func TestModelCatalogService_SearchCatalogRaw(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.SearchCatalogRaw(context.Background(), &v1.SearchCatalogRawRequest{
		Q:     "openai",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTotal() == 0 {
		t.Error("expected at least one search result")
	}
}

func TestModelCatalogService_PreviewMigration(t *testing.T) {
	uc, _ := newTestUsecase(t)
	svc := NewModelCatalogService(uc)

	resp, err := svc.PreviewMigration(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("resp should not be nil")
	}
}

func TestModelCatalogService_GetProviderMigrationRules(t *testing.T) {
	uc, dir := newTestUsecase(t)
	policy := defaultTestPolicy()
	cat := defaultTestDirectory()
	meta := defaultTestMeta()
	seedStore(t, dir, cat, meta, policy)

	svc := NewModelCatalogService(uc)
	resp, err := svc.GetProviderMigrationRules(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetVersion() != modelregistry.ProviderMigrationVersion {
		t.Errorf("Version: got %q, want %q", resp.GetVersion(), modelregistry.ProviderMigrationVersion)
	}
}

func TestToProtoCatalogPolicy_ZeroValues(t *testing.T) {
	p := modelregistry.Policy{}
	got := toProtoCatalogPolicy(p)
	if got.GetSourceUrl() != "" {
		t.Errorf("SourceUrl: got %q, want empty", got.GetSourceUrl())
	}
	if got.GetSyncPolicy() != "" {
		t.Errorf("SyncPolicy: got %q, want empty", got.GetSyncPolicy())
	}
	if got.GetSyncIntervalHours() != 0 {
		t.Errorf("SyncIntervalHours: got %d, want 0", got.GetSyncIntervalHours())
	}
	if got.GetAutoApply() != "" {
		t.Errorf("AutoApply: got %q, want empty", got.GetAutoApply())
	}
}

func TestToProtoCatalogStatus_FullFieldCoverage(t *testing.T) {
	ts := "2024-01-15T10:30:00Z"
	view := biz.ModelRegistryStatusView{
		Policy: modelregistry.Policy{
			SourceURL:         "https://cdn.example.com/catalog.json",
			SyncPolicy:        "scheduled",
			SyncIntervalHours: 6,
			AutoApply:         "metadata_and_pricing",
		},
		Meta: modelregistry.Meta{
			SyncedAt:      ts,
			ETag:          `"w/abc"`,
			SHA256:        "sha256hash",
			SourceURL:     "https://cdn.example.com/catalog.json",
			ProviderCount: 7,
			ModelCount:    42,
			Bytes:         123456,
		},
		DirectoryLoaded: true,
		LocalPath:       "/opt/data/model-catalog",
		LastSyncStatus:  "failed",
		LastSyncSummary: "network timeout",
	}
	got := toProtoCatalogStatus(view)

	if got.GetPolicy().GetSourceUrl() != view.Policy.SourceURL {
		t.Errorf("Policy.SourceUrl mismatch")
	}
	if got.GetPolicy().GetSyncPolicy() != view.Policy.SyncPolicy {
		t.Errorf("Policy.SyncPolicy mismatch")
	}
	if got.GetPolicy().GetSyncIntervalHours() != int32(view.Policy.SyncIntervalHours) {
		t.Errorf("Policy.SyncIntervalHours mismatch")
	}
	if got.GetPolicy().GetAutoApply() != view.Policy.AutoApply {
		t.Errorf("Policy.AutoApply mismatch")
	}
	if got.GetLastSyncStatus() != "failed" {
		t.Errorf("LastSyncStatus: got %q, want %q", got.GetLastSyncStatus(), "failed")
	}
	if got.GetLastSyncSummary() != "network timeout" {
		t.Errorf("LastSyncSummary: got %q, want %q", got.GetLastSyncSummary(), "network timeout")
	}
	if got.GetEtag() != `"w/abc"` {
		t.Errorf("Etag: got %q, want %q", got.GetEtag(), `"w/abc"`)
	}
	if got.GetProviderCount() != 7 {
		t.Errorf("ProviderCount: got %d, want 7", got.GetProviderCount())
	}
	if got.GetModelCount() != 42 {
		t.Errorf("ModelCount: got %d, want 42", got.GetModelCount())
	}
	if got.GetCatalogBytes() != 123456 {
		t.Errorf("CatalogBytes: got %d, want 123456", got.GetCatalogBytes())
	}
	if !got.GetCatalogLoaded() {
		t.Error("CatalogLoaded: expected true")
	}
	if got.GetLocalPath() != "/opt/data/model-catalog" {
		t.Errorf("LocalPath: got %q, want %q", got.GetLocalPath(), "/opt/data/model-catalog")
	}
	if got.GetLastSyncAt() == nil {
		t.Fatal("LastSyncAt should not be nil")
	}
	expectedTS := timestamppb.New(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	if got.GetLastSyncAt().GetSeconds() != expectedTS.GetSeconds() {
		t.Errorf("LastSyncAt seconds: got %d, want %d", got.GetLastSyncAt().GetSeconds(), expectedTS.GetSeconds())
	}
}

func TestParseCatalogTS_ValidRFC3339WithOffset(t *testing.T) {
	input := "2024-06-15T08:00:00+08:00"
	got := parseCatalogTS(input)
	if got == nil {
		t.Fatal("expected non-nil for valid RFC3339 with offset")
	}
	parsed, err := time.Parse(time.RFC3339, input)
	if err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}
	want := timestamppb.New(parsed)
	if got.Seconds != want.Seconds {
		t.Errorf("Seconds: got %d, want %d", got.Seconds, want.Seconds)
	}
}

func TestParseCatalogTS_InvalidFormat(t *testing.T) {
	input := "2024/06/15 08:00:00"
	got := parseCatalogTS(input)
	if got != nil {
		t.Errorf("expected nil for invalid format, got %v", got)
	}
}

func TestParseCatalogTS_EmptyString(t *testing.T) {
	got := parseCatalogTS("")
	if got != nil {
		t.Errorf("expected nil for empty string, got %v", got)
	}
}

func TestModelCatalogService_SyncModelCatalog_ErrorPath_ErrorMessage(t *testing.T) {
	roots := &stubRootResolver{err: fmt.Errorf("root dir error")}
	uc := biz.NewModelRegistryUsecase(roots, nil, loggateway.NewNoop())
	svc := NewModelCatalogService(uc)

	resp, err := svc.SyncModelCatalog(context.Background(), &v1.SyncModelCatalogRequest{})
	if err != nil {
		t.Fatalf("should not return Go error, got: %v", err)
	}
	if resp.GetOk() {
		t.Error("Ok should be false")
	}
	if resp.GetMessage() == "" {
		t.Error("Message should not be empty on error")
	}
}

package monitor_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"aranea-agents/internal/biz/monitor"
)

func TestParseMetadataJSON(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantNil   bool
		wantKey   string
		wantValue string
	}{
		{"empty_string", "", true, "", ""},
		{"whitespace_only", "   ", true, "", ""},
		{"valid_json", `{"key":"value","num":42}`, false, "key", "value"},
		{"invalid_json", `{not json}`, true, "", ""},
		{"empty_object", `{}`, false, "", ""},
		{"nested_json", `{"outer":{"inner":"val"}}`, false, "outer", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.ParseMetadataJSON(tt.raw)
			if tt.wantNil {
				if got != nil {
					t.Errorf("ParseMetadataJSON(%q) = %v, want nil", tt.raw, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseMetadataJSON(%q) = nil, want non-nil", tt.raw)
			}
			if tt.wantKey != "" {
				if _, ok := got[tt.wantKey]; !ok {
					t.Errorf("key %q missing in result", tt.wantKey)
				}
			}
		})
	}
}

func TestNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no_args", nil, 0},
		{"all_empty", []string{"", "", ""}, 0},
		{"all_whitespace", []string{"  ", " ", "\t"}, 0},
		{"mixed", []string{"hello", "", "world"}, 2},
		{"all_nonempty", []string{"a", "b", "c"}, 3},
		{"single_nonempty", []string{"test"}, 1},
		{"single_empty", []string{""}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.NonEmpty(tt.args...)
			if len(got) != tt.want {
				t.Errorf("NonEmpty() returned %d items, want %d; got %v", len(got), tt.want, got)
			}
		})
	}
}

func TestNonEmpty_Values(t *testing.T) {
	got := monitor.NonEmpty("hello", "", "world", "  ")
	if len(got) != 2 {
		t.Fatalf("NonEmpty() = %v, want 2 items", got)
	}
	if got[0] != "hello" {
		t.Errorf("got[0] = %q, want %q", got[0], "hello")
	}
	if got[1] != "world" {
		t.Errorf("got[1] = %q, want %q", got[1], "world")
	}
}

func TestNewDiagBundleGenerator_NilRepo(t *testing.T) {
	g := monitor.NewDiagBundleGenerator(nil)
	if g != nil {
		t.Error("NewDiagBundleGenerator(nil) should return nil")
	}
}

func TestDiagBundleGenerator_Generate_NilGenerator(t *testing.T) {
	var g *monitor.DiagBundleGenerator
	_, err := g.Generate(context.Background(), "trace-1", "sess-1", "run-1", "step-1", "manual", 5)
	if err == nil {
		t.Error("nil generator should return error")
	}
}

func TestDiagBundleGenerator_Generate_DefaultContextMinutes(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "", "manual", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle should not be nil")
	}
	if bundle.BundleID == "" {
		t.Error("BundleID should be auto-generated")
	}
}

func TestDiagBundleGenerator_Generate_NegativeContextMinutes(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "", "manual", -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle should not be nil")
	}
}

func TestDiagBundleGenerator_Generate_WithSessionID(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.completion", Name: "test", Status: "success", MetadataJSON: `{"session_id":"sess-1"}`},
					{ID: "e2", Key: "alert.fired", Name: "alert", Status: "firing", MetadataJSON: `{"session_id":"sess-1"}`},
					{ID: "e3", Key: "other", Name: "no-match", Status: "ok", MetadataJSON: `{"session_id":"other"}`},
				},
				Total: 3,
			}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.Total != 2 {
		t.Errorf("Total = %d, want 2 (only matching events)", bundle.Total)
	}
	if bundle.FlowJSONL == "" {
		t.Error("FlowJSONL should not be empty")
	}
	if bundle.AlertsJSONL == "" {
		t.Error("AlertsJSONL should not be empty")
	}

	var flowEntries []map[string]any
	if err := json.Unmarshal([]byte(bundle.FlowJSONL), &flowEntries); err != nil {
		t.Fatalf("FlowJSONL unmarshal error: %v", err)
	}
	if len(flowEntries) != 2 {
		t.Errorf("flow entries = %d, want 2", len(flowEntries))
	}
}

func TestDiagBundleGenerator_Generate_WithTraceID(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.completion", Name: "test", Status: "success", MetadataJSON: `{"trace_id":"trace-1"}`},
				},
				Total: 1,
			}, nil
		},
		getMonitorTraceFn: func(_ context.Context, id string) (monitor.PlatformRow, error) {
			return monitor.PlatformRow{
				ID: "trace-1", Name: "test-trace", Status: "running",
				MetadataJSON: `{"span_count":5}`,
			}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "trace-1", "", "", "", "auto", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.Total < 2 {
		t.Errorf("Total = %d, want >= 2 (event + trace)", bundle.Total)
	}
	if bundle.TraceJSON == "" || bundle.TraceJSON == "null" {
		t.Error("TraceJSON should contain trace data")
	}
}

func TestDiagBundleGenerator_Generate_WithStepID(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "run-1", "tool-step-1", "error", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.RootCauses == nil {
		t.Error("RootCauses should be populated when stepID is provided")
	}
}

func TestDiagBundleGenerator_Generate_NoStepID_NoRootCauses(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.RootCauses != nil {
		t.Errorf("RootCauses should be nil when stepID is empty, got %v", bundle.RootCauses)
	}
}

func TestDiagBundleGenerator_Generate_ManifestStructure(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "trace-1", "sess-1", "run-1", "step-1", "manual", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifest := bundle.Manifest
	if manifest["schema_version"] != "diag_bundle/v1" {
		t.Errorf("schema_version = %v, want diag_bundle/v1", manifest["schema_version"])
	}
	if manifest["bundle_id"] != bundle.BundleID {
		t.Errorf("bundle_id mismatch: manifest=%v, bundle=%v", manifest["bundle_id"], bundle.BundleID)
	}

	trigger, ok := manifest["trigger"].(map[string]any)
	if !ok {
		t.Fatal("trigger should be a map")
	}
	if trigger["type"] != "manual" {
		t.Errorf("trigger.type = %v, want manual", trigger["type"])
	}
	if trigger["trace_id"] != "trace-1" {
		t.Errorf("trigger.trace_id = %v, want trace-1", trigger["trace_id"])
	}
	if trigger["session_id"] != "sess-1" {
		t.Errorf("trigger.session_id = %v, want sess-1", trigger["session_id"])
	}

	scope, ok := manifest["scope"].(map[string]any)
	if !ok {
		t.Fatal("scope should be a map")
	}
	timeRange, ok := scope["time_range"].([]string)
	if !ok || len(timeRange) != 2 {
		t.Fatalf("scope.time_range should have 2 elements, got %v", scope["time_range"])
	}

	files, ok := manifest["files"].(map[string]any)
	if !ok {
		t.Fatal("files should be a map")
	}
	for _, key := range []string{"flow.jsonl", "trace.json", "usage.json", "alerts.jsonl"} {
		if _, ok := files[key]; !ok {
			t.Errorf("files missing key %q", key)
		}
	}
}

func TestDiagBundleGenerator_Generate_UsageRecords(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			if query.Limit == 50 {
				return monitor.ListResult{
					Items: []monitor.PlatformRow{
						{ID: "u1", Key: "usage.tokens", Name: "usage-1", Status: "ok", MetadataJSON: `{"trace_id":"trace-1","session_id":"sess-1"}`},
						{ID: "u2", Key: "other.key", Name: "non-usage", Status: "ok", MetadataJSON: `{"session_id":"sess-1"}`},
					},
					Total: 2,
				}, nil
			}
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "trace-1", "sess-1", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var usageData map[string]any
	if err := json.Unmarshal([]byte(bundle.UsageJSON), &usageData); err != nil {
		t.Fatalf("UsageJSON unmarshal error: %v", err)
	}
	records, ok := usageData["records"].([]any)
	if !ok {
		t.Fatal("usage records should be a slice")
	}
	if len(records) != 1 {
		t.Errorf("usage records = %d, want 1 (only usage prefix)", len(records))
	}
}

func TestDiagBundleGenerator_Generate_ListEventsError(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, fmt.Errorf("db error")
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("should not return error on list failure, got: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle should not be nil even on list error")
	}
	if bundle.Total != 0 {
		t.Errorf("Total = %d, want 0 on list error", bundle.Total)
	}
}

func TestDiagBundleGenerator_Generate_TriggerMetadataExtraction(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step", Name: "step", Status: "error", MetadataJSON: `{"step_id":"tool-step-1","flow_phase":"error"}`},
				},
				Total: 1,
			}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "sess-1", "", "tool-step-1", "error", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.RootCauses == nil {
		t.Error("RootCauses should be populated for tool step with error phase")
	}
}

func TestDiagBundleGenerator_Generate_EmptySessionAndTrace(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	g := monitor.NewDiagBundleGenerator(repo)
	bundle, err := g.Generate(context.Background(), "", "", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.Total != 0 {
		t.Errorf("Total = %d, want 0 when no sessionID or traceID", bundle.Total)
	}
}

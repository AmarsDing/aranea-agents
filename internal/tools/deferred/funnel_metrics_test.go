package deferred

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// P1-4 漏斗度量：发现（tool_search）→ 激活（tool_load）→ 使用（复用
// aranea_tool_invocation_total）三段 counter，支撑 deferred 工具转化率分析。

func TestToolSearch_FunnelMetricEmitted(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "arxiv_search", Description: "Search academic papers", Category: "search"},
	}
	tool := NewToolSearchTool(catalog)

	hit := metrics.DeferredToolSearchTotal.WithLabelValues("true")
	miss := metrics.DeferredToolSearchTotal.WithLabelValues("false")
	hitBefore := testutil.ToFloat64(hit)
	missBefore := testutil.ToFloat64(miss)

	if _, err := tool.Call(context.Background(), []byte(`{"query": "arxiv"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := tool.Call(context.Background(), []byte(`{"query": "zzz"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if delta := testutil.ToFloat64(hit) - hitBefore; delta != 1 {
		t.Errorf("has_results=true counter delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(miss) - missBefore; delta != 1 {
		t.Errorf("has_results=false counter delta = %v, want 1", delta)
	}
}

func TestToolLoad_FunnelMetricEmitted(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)
	tool.Manager().RegisterTool("web_fetch", newWebFetchTool())

	success := metrics.DeferredToolActivationTotal.WithLabelValues("web_fetch", "success")
	notFound := metrics.DeferredToolActivationTotal.WithLabelValues("ghost_tool", "not_found")
	successBefore := testutil.ToFloat64(success)
	notFoundBefore := testutil.ToFloat64(notFound)

	ctx := withTestInvocation(context.Background())
	args, _ := json.Marshal(toolLoadInput{ToolName: "web_fetch"})
	if _, err := tool.Call(ctx, args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ghostArgs, _ := json.Marshal(toolLoadInput{ToolName: "ghost_tool"})
	if _, err := tool.Call(ctx, ghostArgs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if delta := testutil.ToFloat64(success) - successBefore; delta != 1 {
		t.Errorf("success counter delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(notFound) - notFoundBefore; delta != 1 {
		t.Errorf("not_found counter delta = %v, want 1", delta)
	}
}

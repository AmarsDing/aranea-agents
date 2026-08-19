package health

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	mcpmetadata "aranea-agents/internal/mcp/metadata"
	"aranea-agents/pkg/loggateway"
)

// --- minimal stubs to build a real *biz.MCPServerUsecase ---

type discStubRepo struct {
	server biz.MCPServer
}

func (r *discStubRepo) ListMCPServers(_ context.Context, _ biz.MCPListQuery) ([]biz.MCPServer, error) {
	return []biz.MCPServer{r.server}, nil
}
func (r *discStubRepo) ListMCPServersPaged(_ context.Context, q biz.MCPListQuery) (biz.MCPListResult, error) {
	return biz.MCPListResult{Items: []biz.MCPServer{r.server}, Total: 1, Limit: q.Limit, Offset: q.Offset}, nil
}
func (r *discStubRepo) GetMCPServer(_ context.Context, _ string) (biz.MCPServer, error) {
	return r.server, nil
}
func (r *discStubRepo) GetMCPServerByKey(_ context.Context, _ string) (biz.MCPServer, error) {
	return r.server, nil
}
func (r *discStubRepo) CreateMCPServer(_ context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	return m, nil
}
func (r *discStubRepo) UpdateMCPServer(_ context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	r.server = m
	return m, nil
}
func (r *discStubRepo) DeleteMCPServer(_ context.Context, _ string) error { return nil }
func (r *discStubRepo) UpdateMCPServerConfigJSON(_ context.Context, _ string, cfg string) error {
	r.server.ConfigJSON = cfg
	return nil
}
func (r *discStubRepo) UpdateMCPServerMetadata(_ context.Context, _ string, metadataJSON string, status string) error {
	r.server.MetadataJSON = metadataJSON
	if status != "" {
		r.server.Status = status
	}
	return nil
}

type discStubCredRepo struct{}

func (discStubCredRepo) ListMCPServerUserCredentials(_ context.Context, _, _ string) ([]biz.MCPServerUserCredential, error) {
	return nil, nil
}
func (discStubCredRepo) UpsertMCPServerUserCredential(_ context.Context, c biz.MCPServerUserCredential) (biz.MCPServerUserCredential, error) {
	return c, nil
}
func (discStubCredRepo) DeleteMCPServerUserCredential(_ context.Context, _, _, _ string) error {
	return nil
}

type discStubProber struct{ result biz.MCPTestResult }

func (p discStubProber) Evaluate(_ context.Context, _ bool, _ string) biz.MCPTestResult {
	return p.result
}

// discStubMetaEdit delegates to the real mcpmetadata implementation.
type discStubMetaEdit struct{}

func (discStubMetaEdit) Parse(raw string) map[string]any { return mcpmetadata.Parse(raw) }
func (discStubMetaEdit) Marshal(m map[string]any) (string, error) {
	return mcpmetadata.Marshal(m)
}
func (discStubMetaEdit) ApplyHealth(m map[string]any, s string, ok bool, errMsg string, at time.Time) (map[string]any, string) {
	return mcpmetadata.ApplyHealth(m, s, ok, errMsg, at)
}
func (discStubMetaEdit) ApplyReconnect(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.ApplyReconnect(m, at)
}
func (discStubMetaEdit) MarkHealthAlert(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.MarkHealthAlert(m, at)
}
func (discStubMetaEdit) ApplyToolDiscovery(m map[string]any, count int, names []string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscovery(m, count, names, at)
}
func (discStubMetaEdit) ApplyToolDiscoveryError(m map[string]any, errMsg string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscoveryError(m, errMsg, at)
}

type countingDiscoverer struct {
	calls int
	names []string
}

func (d *countingDiscoverer) DiscoverTools(_ context.Context, _ string, _ string) ([]string, error) {
	d.calls++
	return d.names, nil
}

func newDiscoveryRunner(repo *discStubRepo, prober biz.MCPProber, disc *countingDiscoverer) *Runner {
	uc := biz.NewMCPServerUsecase(repo, discStubCredRepo{}, prober, discStubMetaEdit{}, nil)
	if disc != nil {
		uc.SetToolDiscoverer(disc)
	}
	return NewRunner(Deps{MCP: repo, UC: uc}, loggateway.NewNoop())
}

// TestProbeOne_StaleDiscoveryTriggersFallback: probe OK + 无 tools_discovered_at
// → 兜底握手触发一次，tool_count 写入 metadata。
func TestProbeOne_StaleDiscoveryTriggersFallback(t *testing.T) {
	repo := &discStubRepo{server: biz.MCPServer{
		ID: "s1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON: `{"transport":"stdio","command":"x"}`,
	}}
	disc := &countingDiscoverer{names: []string{"a", "b"}}
	r := newDiscoveryRunner(repo, discStubProber{result: biz.MCPTestResult{OK: true, Status: "ok"}}, disc)

	r.probeOne(context.Background(), repo.server)

	if disc.calls != 1 {
		t.Fatalf("expected 1 discovery call, got %d", disc.calls)
	}
	meta := mcpmetadata.Parse(repo.server.MetadataJSON)
	if meta[mcpmetadata.KeyToolCount] != float64(2) {
		t.Fatalf("tool_count=%v", meta[mcpmetadata.KeyToolCount])
	}
}

// TestProbeOne_FreshDiscoverySkipped: tools_discovered_at 刚刚写入（如
// full_handshake 模式 persistHealth 合并）→ stale=false，不重复握手。
func TestProbeOne_FreshDiscoverySkipped(t *testing.T) {
	repo := &discStubRepo{server: biz.MCPServer{
		ID: "s1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON:  `{"transport":"stdio","command":"x"}`,
		MetadataJSON: `{"tools_discovered_at":"` + time.Now().UTC().Format(time.RFC3339) + `","tool_count":3}`,
	}}
	disc := &countingDiscoverer{names: []string{"a"}}
	r := newDiscoveryRunner(repo, discStubProber{result: biz.MCPTestResult{OK: true, Status: "ok"}}, disc)

	r.probeOne(context.Background(), repo.server)

	if disc.calls != 0 {
		t.Fatalf("fresh discovery should skip fallback, got %d calls", disc.calls)
	}
}

// TestProbeOne_UnhealthyServerSkipsDiscovery: probe 失败 → 不兜底（服务器不可达，
// 握手必然失败）。
func TestProbeOne_UnhealthyServerSkipsDiscovery(t *testing.T) {
	repo := &discStubRepo{server: biz.MCPServer{
		ID: "s1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON: `{"transport":"stdio","command":"x"}`,
	}}
	disc := &countingDiscoverer{names: []string{"a"}}
	r := newDiscoveryRunner(repo, discStubProber{result: biz.MCPTestResult{OK: false, Status: "error", Message: "boom"}}, disc)

	r.probeOne(context.Background(), repo.server)

	if disc.calls != 0 {
		t.Fatalf("unhealthy server should skip discovery, got %d calls", disc.calls)
	}
}

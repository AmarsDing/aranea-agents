package tools

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
	tmcp "trpc.group/trpc-go/trpc-mcp-go"
)

// startResourceTestServer 起一个真实 streamable MCP server（trpc-mcp-go 官方
// HTTPHandler + httptest 模式），注册 text/blob 两个资源，返回客户端接入 URL。
func startResourceTestServer(t *testing.T) string {
	t.Helper()
	server := tmcp.NewServer("resource-test-server", "1.0.0", tmcp.WithServerPath("/mcp"))

	textRes := &tmcp.Resource{
		Name:        "zeta_doc",
		URI:         "file://zeta/doc.txt",
		Description: "Zeta 文档（后排序到尾部）",
		MimeType:    "text/plain",
	}
	server.RegisterResources(textRes, func(ctx context.Context, req *tmcp.ReadResourceRequest) ([]tmcp.ResourceContents, error) {
		return []tmcp.ResourceContents{tmcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "text/plain",
			Text:     "zeta content",
		}}, nil
	})

	blobRes := &tmcp.Resource{
		Name:     "alpha_blob",
		URI:      "file://alpha/blob.bin",
		MimeType: "application/octet-stream",
	}
	server.RegisterResources(blobRes, func(ctx context.Context, req *tmcp.ReadResourceRequest) ([]tmcp.ResourceContents, error) {
		return []tmcp.ResourceContents{tmcp.BlobResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/octet-stream",
			Blob:     "aGVsbG8=",
		}}, nil
	})

	httpServer := httptest.NewServer(server.HTTPHandler())
	t.Cleanup(httpServer.Close)
	return httpServer.URL + "/mcp"
}

func resourceTestBrokerCfg(serverURL string) MCPBrokerConfig {
	return MCPBrokerConfig{
		Servers: []MCPServerConfig{{
			Name:       "res_srv",
			Transport:  "streamable",
			ServerURL:  serverURL,
			TimeoutSec: 10,
		}},
	}
}

func TestBuildMCPResourceTools_EmptyServers(t *testing.T) {
	if got := buildMCPResourceTools(MCPBrokerConfig{}); got != nil {
		t.Fatalf("no named servers must yield no resource tools; got %d", len(got))
	}
	// 仅有空名服务器同样不挂载。
	if got := buildMCPResourceTools(MCPBrokerConfig{Servers: []MCPServerConfig{{Name: "  "}}}); got != nil {
		t.Fatalf("blank server names must yield no resource tools; got %d", len(got))
	}
}

func TestBuildMCPBrokerTools_IncludesResourceTools(t *testing.T) {
	tools, err := buildMCPBrokerTools(resourceTestBrokerCfg("http://127.0.0.1:1/mcp"))
	if err != nil {
		t.Fatalf("buildMCPBrokerTools: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		d := tl.Declaration()
		if d == nil {
			t.Fatal("resource/broker tool with nil declaration")
		}
		names[d.Name] = true
	}
	for _, want := range []string{"mcp_list_servers", "mcp_list_tools", "mcp_inspect_tools", "mcp_call", mcpListResourcesToolName, mcpReadResourceToolName} {
		if !names[want] {
			t.Fatalf("broker tool family missing %q; got %v", want, names)
		}
	}
}

func TestMCPResourceResolver_UnknownSelector(t *testing.T) {
	r := newMCPResourceResolver(MCPBrokerConfig{Servers: []MCPServerConfig{
		{Name: "beta"}, {Name: "alpha"},
	}})
	_, _, err := r.lookup("gamma")
	if err == nil {
		t.Fatal("unknown selector must error")
	}
	// 错误信息按字母序列出可用服务器，便于模型自我纠正。
	if !strings.Contains(err.Error(), "alpha, beta") {
		t.Fatalf("error must list configured servers sorted; got %v", err)
	}
}

func TestMCPResourceResolver_ResolveHeaders(t *testing.T) {
	var gotReq *trpcmcpbroker.HeaderInjectRequest
	r := newMCPResourceResolver(MCPBrokerConfig{
		Servers: []MCPServerConfig{{Name: "srv", Headers: map[string]string{"X-Static": "s", "X-Auth": "static"}}},
		HeaderInjector: func(ctx context.Context, req *trpcmcpbroker.HeaderInjectRequest) (map[string]string, error) {
			gotReq = req
			return map[string]string{"X-Auth": "injected"}, nil
		},
	})
	headers, err := r.resolveHeaders(context.Background(), "srv", r.servers["srv"], "streamable")
	if err != nil {
		t.Fatalf("resolveHeaders: %v", err)
	}
	if headers["X-Static"] != "s" || headers["X-Auth"] != "injected" {
		t.Fatalf("injector must override static on conflict; got %v", headers)
	}
	if gotReq == nil || gotReq.Selector != "srv" || gotReq.Phase != "resources" || gotReq.IsAdHoc {
		t.Fatalf("injector request mismatch: %#v", gotReq)
	}
}

func TestMCPResources_EndToEnd(t *testing.T) {
	serverURL := startResourceTestServer(t)
	r := newMCPResourceResolver(resourceTestBrokerCfg(serverURL))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// list：按 URI 排序，alpha 在前。
	list, err := r.listResources(ctx, mcpListResourcesInput{Selector: "res_srv"})
	if err != nil {
		t.Fatalf("listResources: %v", err)
	}
	if len(list.Resources) != 2 || list.Truncated {
		t.Fatalf("expect 2 resources without truncation; got %+v", list)
	}
	if list.Resources[0].URI != "file://alpha/blob.bin" || list.Resources[1].URI != "file://zeta/doc.txt" {
		t.Fatalf("resources must sort by URI; got %+v", list.Resources)
	}
	if list.Resources[1].Name != "zeta_doc" || list.Resources[1].MimeType != "text/plain" {
		t.Fatalf("summary fields lost; got %+v", list.Resources[1])
	}

	// read text：内容透传。
	textOut, err := r.readResource(ctx, mcpReadResourceInput{Selector: "res_srv", URI: "file://zeta/doc.txt"})
	if err != nil {
		t.Fatalf("readResource text: %v", err)
	}
	if len(textOut.Contents) != 1 || textOut.Contents[0].Text != "zeta content" || textOut.Contents[0].Truncated {
		t.Fatalf("text content mismatch; got %+v", textOut.Contents)
	}

	// read blob：base64 透传。
	blobOut, err := r.readResource(ctx, mcpReadResourceInput{Selector: "res_srv", URI: "file://alpha/blob.bin"})
	if err != nil {
		t.Fatalf("readResource blob: %v", err)
	}
	if len(blobOut.Contents) != 1 || blobOut.Contents[0].BlobBase64 != "aGVsbG8=" {
		t.Fatalf("blob content mismatch; got %+v", blobOut.Contents)
	}

	// 空 URI 前置校验。
	if _, err := r.readResource(ctx, mcpReadResourceInput{Selector: "res_srv", URI: "  "}); err == nil {
		t.Fatal("empty uri must error before connecting")
	}
}

func TestTruncateMCPContent(t *testing.T) {
	if got, truncated := truncateMCPContent("abc", 3); got != "abc" || truncated {
		t.Fatalf("exact-fit must not truncate; got %q truncated=%v", got, truncated)
	}
	got, truncated := truncateMCPContent("你好世界x", 4)
	if !truncated || !strings.HasPrefix(got, "你好世界") || !strings.Contains(got, "[truncated]") {
		t.Fatalf("rune-boundary truncation broken; got %q truncated=%v", got, truncated)
	}
}

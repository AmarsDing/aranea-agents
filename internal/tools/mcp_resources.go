package tools

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
	tmcp "trpc.group/trpc-go/trpc-mcp-go"
)

// MCP resources（resources/list、resources/read）业务层暴露。
//
// 框架 mcpbroker 元工具只覆盖 tools 面（list/inspect/call）；resources 能力由
// trpc-mcp-go v0.0.10 客户端原生提供（普通外部依赖，非 vendored 框架），此处按
// broker 同一 selector 语义在业务层补齐，避免修改 vendored 框架（FW-R1 红线）。
//
// 装配入口为 buildMCPBrokerTools：broker 工具族 = 4 个框架元工具 + 本文件 2 个
// resources 工具，共用 MCPBrokerConfig.Servers 与 HeaderInjector（用户凭证注入
// 语义与 broker 调用一致）。v1 仅支持命名服务器 selector，不支持 ad-hoc URL
// （resources 是读面，收敛攻击面；broker tools 面的 ad-hoc 语义不变）。
const (
	mcpListResourcesToolName = "mcp_list_resources"
	mcpReadResourceToolName  = "mcp_read_resource"

	// mcpResourceListCap 限制单次 list 返回条目数，防止大型资源库灌爆上下文。
	mcpResourceListCap = 200
	// mcpResourceContentMaxRunes 限制单项资源内容符文数（read 结果直进工具结果）。
	mcpResourceContentMaxRunes = 100_000
)

// mcpResourcesClientInfo 标识一次性 resources 会话的客户端身份。
var mcpResourcesClientInfo = tmcp.Implementation{Name: "aranea-mcp-resources", Version: "1.0.0"}

type mcpListResourcesInput struct {
	Selector string `json:"selector" jsonschema:"required,description=Named MCP server selector (e.g. local_stdio_code). Use mcp_list_servers to discover configured servers."`
}

type mcpResourceSummary struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type mcpListResourcesOutput struct {
	Resources []mcpResourceSummary `json:"resources"`
	Truncated bool                 `json:"truncated,omitempty"`
}

type mcpReadResourceInput struct {
	Selector string `json:"selector" jsonschema:"required,description=Named MCP server selector (e.g. local_stdio_code)."`
	URI      string `json:"uri" jsonschema:"required,description=Resource URI exactly as returned by mcp_list_resources."`
}

type mcpResourceContent struct {
	URI        string `json:"uri"`
	MimeType   string `json:"mime_type,omitempty"`
	Text       string `json:"text,omitempty"`
	BlobBase64 string `json:"blob_base64,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type mcpReadResourceOutput struct {
	Contents []mcpResourceContent `json:"contents"`
}

// mcpResourceResolver 从 MCPBrokerConfig 解析命名服务器与按调用注入的头，
// selector 语义与框架 broker 元工具一致（仅命名服务器）。
type mcpResourceResolver struct {
	servers  map[string]MCPServerConfig
	injector func(context.Context, *trpcmcpbroker.HeaderInjectRequest) (map[string]string, error)
}

func newMCPResourceResolver(cfg MCPBrokerConfig) *mcpResourceResolver {
	r := &mcpResourceResolver{
		servers:  make(map[string]MCPServerConfig, len(cfg.Servers)),
		injector: cfg.HeaderInjector,
	}
	for _, s := range cfg.Servers {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		r.servers[name] = s
	}
	return r
}

func (r *mcpResourceResolver) lookup(selector string) (string, MCPServerConfig, error) {
	name := strings.TrimSpace(selector)
	srv, ok := r.servers[name]
	if !ok {
		names := make([]string, 0, len(r.servers))
		for n := range r.servers {
			names = append(names, n)
		}
		sort.Strings(names)
		return "", MCPServerConfig{}, fmt.Errorf("unknown MCP server selector %q; configured servers: %s", selector, strings.Join(names, ", "))
	}
	return name, srv, nil
}

// resolveHeaders 合并静态配置头与按调用注入的头（RequireUserCredentials 服务器）。
// 注入头冲突时覆盖静态头，与 broker 的优先级一致。
func (r *mcpResourceResolver) resolveHeaders(ctx context.Context, name string, srv MCPServerConfig, transport string) (map[string]string, error) {
	headers := make(map[string]string, len(srv.Headers))
	for k, v := range srv.Headers {
		headers[k] = v
	}
	if r.injector == nil {
		return headers, nil
	}
	injected, err := r.injector(ctx, &trpcmcpbroker.HeaderInjectRequest{
		Selector:  name,
		BaseURL:   strings.TrimSpace(srv.ServerURL),
		Phase:     "resources",
		Transport: transport,
		IsAdHoc:   false,
	})
	if err != nil {
		return nil, err
	}
	for k, v := range injected {
		headers[k] = v
	}
	return headers, nil
}

// newOneShotMCPClient 按连接配置建一次性客户端，镜像框架 mcpSessionManager
// createClient 的传输分支（stdio/sse/streamable），HTTP 头在创建时静态合入
// （注入头已在调用方按 ctx 解析，无需 WithHTTPBeforeRequest 钩子）。
func newOneShotMCPClient(connCfg trpcmcp.ConnectionConfig, headers map[string]string) (tmcp.Connector, error) {
	httpOpts := make([]tmcp.ClientOption, 0, 1)
	if len(headers) > 0 {
		h := http.Header{}
		for k, v := range headers {
			h.Set(k, v)
		}
		httpOpts = append(httpOpts, tmcp.WithHTTPHeaders(h))
	}

	switch connCfg.Transport {
	case "", "stdio":
		return tmcp.NewStdioClient(tmcp.StdioTransportConfig{
			ServerParams: tmcp.StdioServerParameters{
				Command: connCfg.Command,
				Args:    connCfg.Args,
				Env:     connCfg.Env,
			},
			Timeout: connCfg.Timeout,
		}, mcpResourcesClientInfo)
	case "sse":
		return tmcp.NewSSEClient(connCfg.ServerURL, mcpResourcesClientInfo, httpOpts...)
	default: // streamable（含 NormalizeTransport 归一后的全部 HTTP 形态）
		return tmcp.NewClient(connCfg.ServerURL, mcpResourcesClientInfo, httpOpts...)
	}
}

// connect 完成「建客户端 + Initialize」并返回可用连接；调用方负责 Close 与 cancel。
func (r *mcpResourceResolver) connect(ctx context.Context, selector string) (tmcp.Connector, context.CancelFunc, error) {
	name, srv, err := r.lookup(selector)
	if err != nil {
		return nil, nil, err
	}
	connCfg := srv.ToConnectionConfig()
	headers, err := r.resolveHeaders(ctx, name, srv, connCfg.Transport)
	if err != nil {
		return nil, nil, err
	}
	client, err := newOneShotMCPClient(connCfg, headers)
	if err != nil {
		return nil, nil, fmt.Errorf("create MCP client for %q: %w", name, err)
	}

	callCtx := ctx
	cancel := context.CancelFunc(func() {})
	if connCfg.Timeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			callCtx, cancel = context.WithTimeout(ctx, connCfg.Timeout)
		}
	}
	if _, err := client.Initialize(callCtx, &tmcp.InitializeRequest{}); err != nil {
		_ = client.Close()
		cancel()
		return nil, nil, fmt.Errorf("initialize MCP session with %q: %w", name, err)
	}
	return client, cancel, nil
}

func (r *mcpResourceResolver) listResources(ctx context.Context, in mcpListResourcesInput) (mcpListResourcesOutput, error) {
	client, cancel, err := r.connect(ctx, in.Selector)
	if err != nil {
		return mcpListResourcesOutput{}, err
	}
	defer func() {
		_ = client.Close()
		cancel()
	}()

	result, err := client.ListResources(ctx, &tmcp.ListResourcesRequest{})
	if err != nil {
		return mcpListResourcesOutput{}, fmt.Errorf("list MCP resources: %w", err)
	}

	resources := result.Resources
	sort.Slice(resources, func(i, j int) bool { return resources[i].URI < resources[j].URI })

	out := mcpListResourcesOutput{Resources: make([]mcpResourceSummary, 0, min(len(resources), mcpResourceListCap))}
	for i := range resources {
		if len(out.Resources) >= mcpResourceListCap {
			out.Truncated = true
			break
		}
		res := resources[i]
		out.Resources = append(out.Resources, mcpResourceSummary{
			Name:        res.Name,
			URI:         res.URI,
			Description: res.Description,
			MimeType:    res.MimeType,
			Size:        res.Size,
		})
	}
	return out, nil
}

func (r *mcpResourceResolver) readResource(ctx context.Context, in mcpReadResourceInput) (mcpReadResourceOutput, error) {
	uri := strings.TrimSpace(in.URI)
	if uri == "" {
		return mcpReadResourceOutput{}, fmt.Errorf("uri is required; pass the URI exactly as returned by mcp_list_resources")
	}
	client, cancel, err := r.connect(ctx, in.Selector)
	if err != nil {
		return mcpReadResourceOutput{}, err
	}
	defer func() {
		_ = client.Close()
		cancel()
	}()

	req := &tmcp.ReadResourceRequest{}
	req.Params.URI = uri
	req.Params.Arguments = map[string]interface{}{}
	result, err := client.ReadResource(ctx, req)
	if err != nil {
		return mcpReadResourceOutput{}, fmt.Errorf("read MCP resource %q: %w", uri, err)
	}

	out := mcpReadResourceOutput{Contents: make([]mcpResourceContent, 0, len(result.Contents))}
	for _, c := range result.Contents {
		switch typed := c.(type) {
		case tmcp.TextResourceContents:
			text, truncated := truncateMCPContent(typed.Text, mcpResourceContentMaxRunes)
			out.Contents = append(out.Contents, mcpResourceContent{URI: typed.URI, MimeType: typed.MIMEType, Text: text, Truncated: truncated})
		case *tmcp.TextResourceContents:
			if typed != nil {
				text, truncated := truncateMCPContent(typed.Text, mcpResourceContentMaxRunes)
				out.Contents = append(out.Contents, mcpResourceContent{URI: typed.URI, MimeType: typed.MIMEType, Text: text, Truncated: truncated})
			}
		case tmcp.BlobResourceContents:
			blob, truncated := truncateMCPContent(typed.Blob, mcpResourceContentMaxRunes)
			out.Contents = append(out.Contents, mcpResourceContent{URI: typed.URI, MimeType: typed.MIMEType, BlobBase64: blob, Truncated: truncated})
		case *tmcp.BlobResourceContents:
			if typed != nil {
				blob, truncated := truncateMCPContent(typed.Blob, mcpResourceContentMaxRunes)
				out.Contents = append(out.Contents, mcpResourceContent{URI: typed.URI, MimeType: typed.MIMEType, BlobBase64: blob, Truncated: truncated})
			}
		}
	}
	return out, nil
}

// truncateMCPContent 按符文截断并报告是否发生截断；不截断时原样返回（确定性输出）。
// 与 toolresult_size_limiter.go 的 truncateRunes 区分：此处需要截断标志回传给模型。
func truncateMCPContent(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s, false
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…[truncated]", true
}

// buildMCPResourceTools 构建 broker 工具族的 resources 面。命名服务器为空时
// 返回 nil——selector 无可解析目标，挂载只会产生永远报错的噪音工具。
func buildMCPResourceTools(cfg MCPBrokerConfig) []Tool {
	r := newMCPResourceResolver(cfg)
	if len(r.servers) == 0 {
		return nil
	}
	return []Tool{
		function.NewFunctionTool(
			r.listResources,
			function.WithName(mcpListResourcesToolName),
			function.WithDescription("List resources exposed by a named MCP server (resources/list). Use a server selector from mcp_list_servers. Resources are server-published data entries (files, records, pages) addressable by URI; read one with mcp_read_resource."),
		),
		function.NewFunctionTool(
			r.readResource,
			function.WithName(mcpReadResourceToolName),
			function.WithDescription("Read one MCP resource by URI (resources/read) from a named MCP server. Call mcp_list_resources first and pass the URI exactly as returned. Large contents are truncated with a [truncated] marker."),
		),
	}
}

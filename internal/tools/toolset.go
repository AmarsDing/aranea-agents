package tools

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"

	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/tools/browser"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/internal/tools/mcpobserve"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcagenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpcarxivsearch "trpc.group/trpc-go/trpc-agent-go/tool/arxivsearch"
	trpcawaitreply "trpc.group/trpc-go/trpc-agent-go/tool/awaitreply"
	trpcduckduckgo "trpc.group/trpc-go/trpc-agent-go/tool/duckduckgo"
	trpcemail "trpc.group/trpc-go/trpc-agent-go/tool/email"

	deliverabletools "aranea-agents/internal/tools/deliverable"
	subagenttool "aranea-agents/internal/tools/subagent"
	workingmemory "aranea-agents/internal/tools/working_memory"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
	trpctodo "trpc.group/trpc-go/trpc-agent-go/tool/todo"
	trpchttpfetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch"
	trpcwikipedia "trpc.group/trpc-go/trpc-agent-go/tool/wikipedia"
	tmcp "trpc.group/trpc-go/trpc-mcp-go"
)

type filesystemDirKey struct{}

// FilesystemDirWithDir is DEPRECATED. Use AssemblyConfig.ClaudeCodeDir or
// AssemblyConfig.FilesystemDir instead. Kept only for backward compatibility;
// will be removed in a future release.
func FilesystemDirWithDir(dir string) context.Context {
	return context.WithValue(context.Background(), filesystemDirKey{}, dir)
}

// FilesystemDirFromContext is DEPRECATED. Use AssemblyConfig fields instead.
func FilesystemDirFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(filesystemDirKey{}).(string); ok {
		return v
	}
	return ""
}

var (
	registryOnce sync.Once
	registry     []*ToolRegistration
)

func Registry() []*ToolRegistration {
	registryOnce.Do(func() {
		registry = []*ToolRegistration{
			{
				Name:        "file",
				Description: "File operation ToolSet (read, write, search, replace, list)",
				Category:    "filesystem",
				Tags:        []string{"filesystem", "read", "write", "search"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					// Placeholder: actual assembly happens in assembleBuiltinToolsets()
					// which applies FilesystemDir config. Returning nil,nil signals
					// assembleFromRegistry to skip this entry.
					return nil, nil
				},
				EnabledByDefault:    true,
				RiskLevel:           "low",
				SupportsConcurrency: true,
				Group:               "file_edit",
				Examples: []ToolUseExample{
					{UserQuery: "read the contents of config.yaml", ToolName: "file", Explanation: "basic file read/write operations"},
					{UserQuery: "search for TODO comments in my project", ToolName: "file", Explanation: "file search within workspace"},
				},
			},
			{
				Name:        "hostexec",
				Description: "Host command execution ToolSet (shell, bash, powershell)",
				Category:    "execution",
				Tags:        []string{"shell", "exec", "command"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					// Placeholder: actual assembly happens in assembleBuiltinToolsets().
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "httpfetch",
				Description: "HTTP web page fetch tool",
				Category:    "web",
				Tags:        []string{"web", "fetch", "http"},
				Factory: func(ctx context.Context) (Tool, error) {
					return trpchttpfetch.NewTool(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
				Group:            "web_search",
				Examples: []ToolUseExample{
					{UserQuery: "fetch the content of this URL", ToolName: "httpfetch", Explanation: "direct HTTP page retrieval, not a search engine"},
				},
			},
			{
				Name:        "geminifetch",
				Description: "Gemini web fetch tool (Gemini-powered page extraction)",
				Category:    "web",
				Tags:        []string{"web", "fetch", "gemini"},
				Factory: func(ctx context.Context) (Tool, error) {
					return nil, nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "duckduckgo",
				Description: "DuckDuckGo web search tool",
				Category:    "search",
				Tags:        []string{"search", "web"},
				Factory: func(ctx context.Context) (Tool, error) {
					return trpcduckduckgo.NewTool(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
				Group:            "web_search",
				Examples: []ToolUseExample{
					{UserQuery: "search the web for recent news about AI", ToolName: "duckduckgo", Explanation: "general web search via DuckDuckGo"},
					{UserQuery: "find information about Go programming language", ToolName: "duckduckgo", Explanation: "broad web search query"},
				},
			},
			{
				Name:        "google_search",
				Description: "Google Custom Search ToolSet",
				Category:    "search",
				Tags:        []string{"search", "web", "google"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return nil, nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
				Group:            "web_search",
				Examples: []ToolUseExample{
					{UserQuery: "search Google for latest research papers on transformers", ToolName: "google_search", Explanation: "Google Custom Search with API key"},
				},
			},
			{
				Name:        "arxiv_search",
				Description: "ArXiv paper search ToolSet",
				Category:    "search",
				Tags:        []string{"search", "academic", "paper"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcarxivsearch.NewToolSet()
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "wikipedia",
				Description: "Wikipedia search ToolSet",
				Category:    "search",
				Tags:        []string{"search", "encyclopedia"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcwikipedia.NewToolSet()
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "email",
				Description: "Email sending ToolSet",
				Category:    "communication",
				Tags:        []string{"email", "send", "smtp"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcemail.NewToolSet()
				},
				EnabledByDefault:     false,
				RiskLevel:            "high",
				RequiresConfirmation: true,
			},
			{
				Name:        "message",
				Description: "Send text and optional files through registered channels (outbound messaging)",
				Category:    "communication",
				Tags:        []string{"communication", "outbound", "channel", "message"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "high",
				RequiresConfirmation: true,
			},
			{
				Name:        "todo",
				Description: "Todo management tool",
				Category:    "productivity",
				Tags:        []string{"todo", "task", "manage"},
				Factory: func(ctx context.Context) (Tool, error) {
					return trpctodo.New(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "await_user_reply",
				Description: "Mark agent as waiting for user reply",
				Category:    "interaction",
				Tags:        []string{"interaction", "reply", "await"},
				Factory: func(ctx context.Context) (Tool, error) {
					return trpcawaitreply.New(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "claudecode",
				Description: "Claude Code ToolSet (bash, edit, read, write, glob, grep, etc.)",
				Category:    "coding",
				Tags:        []string{"coding", "ide", "claude"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					// Actual assembly happens in assembleClaudeCodeToolset() which
					// applies config overrides (base dir, readonly, sandbox, etc.).
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
				Group:                "file_edit",
				Examples: []ToolUseExample{
					{UserQuery: "edit line 42 in main.go to fix the bug", ToolName: "claudecode", Explanation: "full IDE-like coding toolset with bash, edit, grep"},
				},
			},
			{
				Name:        "workspace_exec",
				Description: "Workspace execution tools (exec, write_stdin, kill_session) — NOT YET IMPLEMENTED",
				Category:    "execution",
				Tags:        []string{"exec", "workspace", "code"},
				Factory: func(ctx context.Context) (Tool, error) {
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "openapi",
				Description: "OpenAPI spec ToolSet (dynamic REST API tools from spec)",
				Category:    "integration",
				Tags:        []string{"api", "rest", "openapi"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return nil, apierror.BadRequest(apierror.DomainTool, "openapi requires spec configuration")
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "agent",
				Description:      "Agent-as-Tool: wrap an Agent as a callable Tool for delegation/composition",
				Category:         "composition",
				Tags:             []string{"agent", "delegation", "composition"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "mcp",
				Description:      "MCP ToolSet: connect to MCP servers (stdio/sse/streamable_http) and expose their tools",
				Category:         "integration",
				Tags:             []string{"mcp", "integration", "protocol"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "mcpbroker",
				Description:      "MCP Broker: runtime MCP discovery tools (mcp_list_servers, mcp_list_tools, mcp_inspect_tools, mcp_call)",
				Category:         "integration",
				Tags:             []string{"mcp", "broker", "discovery"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "model_registry_sync",
				Description:      "Model registry sync tools (fetch_model_directory, migrate_provider_bindings, apply_model_directory, sync_provider_logos)",
				Category:         "system",
				Tags:             []string{"model", "registry", "sync", "system"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "subagents_spawn",
				Description:      "Spawn a background subagent for the current session",
				Category:         "composition",
				Tags:             []string{"subagent", "background", "spawn"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "subagents_list",
				Description:      "List background subagents for the current session",
				Category:         "composition",
				Tags:             []string{"subagent", "background", "list"},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:             "subagents_get",
				Description:      "Get status and result of a background subagent run",
				Category:         "composition",
				Tags:             []string{"subagent", "background", "get"},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:             "subagents_cancel",
				Description:      "Cancel a background subagent run (best-effort)",
				Category:         "composition",
				Tags:             []string{"subagent", "background", "cancel"},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "browser",
				Description: "Browser automation tool (navigate, snapshot, screenshot, click, type, etc.) via Playwright MCP",
				Category:    "browser",
				Tags:        []string{"browser", "web", "automation", "playwright"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:                "read_document",
				Description:         "Read a document from a local path (PDF, DOCX, plain text). Use instead of exec_command to inspect documents.",
				Category:            "media",
				Tags:                []string{"document", "pdf", "docx", "read"},
				EnabledByDefault:    true,
				RiskLevel:           "medium",
				SupportsConcurrency: true,
			},
			{
				Name:                "read_spreadsheet",
				Description:         "Read tabular files (XLSX, CSV). Use instead of exec_command when the user asks for rows, sheets, or table excerpts.",
				Category:            "media",
				Tags:                []string{"spreadsheet", "xlsx", "csv", "read"},
				EnabledByDefault:    true,
				RiskLevel:           "medium",
				SupportsConcurrency: true,
			},
			{
				Name:        "read_tool_result",
				Description: "Retrieve the full content of a previously persisted tool result by its blob_id",
				Category:    "system",
				Tags:        []string{"system", "tool-result", "retrieval"},
				Factory: func(ctx context.Context) (Tool, error) {
					return nil, nil
				},
				EnabledByDefault: true,
				RiskLevel:        "low",
				Deferred:         true,
			},
			{
				Name:             "working_memory",
				Description:      "Working memory tools for reading/writing task-scoped fields (read, list, write, patch, delete)",
				Category:         "memory",
				Tags:             []string{"memory", "working-memory", "l1"},
				ToolSetFactory:   func(ctx context.Context) (ToolSet, error) { return workingmemory.ToolSet{}, nil },
				EnabledByDefault: true,
				RiskLevel:        "low",
			},
			{
				Name:             "deliverable",
				Description:      "Cross-agent deliverable handoff tools (set_deliverable, get_deliverable) for structured output passing via graph state",
				Category:         "team",
				Tags:             []string{"team", "deliverable", "handoff", "a2a"},
				ToolSetFactory:   func(ctx context.Context) (ToolSet, error) { return deliverabletools.ToolSet{}, nil },
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "datetime",
				Description: "Returns current date, time and timezone information",
				Category:    "system",
				Tags:        []string{"system", "datetime", "time"},
				Factory: func(ctx context.Context) (Tool, error) {
					return newDatetimeTool(), nil
				},
				EnabledByDefault: true,
				RiskLevel:        "low",
			},
			{
				Name:        "media",
				Description: "Media generation tools (text-to-image, text-to-video, image-to-video)",
				Category:    "media",
				Tags:        []string{"media", "image", "video", "generation"},
				Factory: func(ctx context.Context) (Tool, error) {
					// Media tools require MediaProvider which is not available in the
					// global tool factory context. They are assembled separately via
					// media.NewGenerateImageTool etc. and injected at the Agent level.
					return nil, nil
				},
				EnabledByDefault: true,
				RiskLevel:        "medium",
			},
		}
	})
	// Return a defensive copy so callers cannot mutate the global registry.
	out := make([]*ToolRegistration, len(registry))
	copy(out, registry)
	return out
}

type AgentToolConfig struct {
	Agent             trpcagent.Agent
	Name              string
	Description       string
	SkipSummarization bool
	StreamInner       bool
	HistoryScope      trpcagenttool.HistoryScope
	ResponseMode      trpcagenttool.ResponseMode
}

type MCPServerConfig struct {
	Name                string
	Transport           string
	ServerURL           string
	Command             string
	Args                []string
	Env                 map[string]string
	Headers             map[string]string
	TimeoutSec          int
	ToolPrefix          string
	SessionReconnectMax int
	AllowAdHocHTTP      bool
	AdHocTimeoutSec     int
	// RequireUserCredentials defers auth header resolution to tool-call time
	// via HeaderInjector / MCPBrokerConfig.HeaderInjector (E2E-P1-08).
	RequireUserCredentials bool
	// HeaderInjector resolves per-request HTTP headers from Invocation context
	// for this named server (MCP ToolSet path). Prefer over build-time Headers
	// when RequireUserCredentials is set.
	HeaderInjector func(ctx context.Context) (map[string]string, error)
}

type MCPBrokerConfig struct {
	Servers         []MCPServerConfig
	AllowAdHocHTTP  bool
	AdHocTimeoutSec int
	// HeaderInjector resolves per-request HTTP headers from Invocation context.
	// Used for RequireUserCredentials MCP servers so user identity is available
	// at tool-call time (not Agent build time). E2E-P1-08.
	HeaderInjector func(ctx context.Context, req *trpcmcpbroker.HeaderInjectRequest) (map[string]string, error)
}

// ToConnectionConfig is the SINGLE mapping from MCPServerConfig to the framework
// ConnectionConfig. All runtime code paths (buildMCPToolSet, buildMCPBrokerTools)
// must call this instead of constructing trpcmcp.ConnectionConfig manually so
// transport normalization, timeout defaults, and field mapping stay aligned.
// TPM-P1-12.
func (c MCPServerConfig) ToConnectionConfig() trpcmcp.ConnectionConfig {
	transport := mcpconfig.NormalizeTransport(c.Transport)
	if transport == "" {
		transport = string(mcpconfig.TransportStdio)
	}
	return trpcmcp.ConnectionConfig{
		Transport: transport,
		ServerURL: strings.TrimSpace(c.ServerURL),
		Headers:   c.Headers,
		Command:   strings.TrimSpace(c.Command),
		Args:      c.Args,
		Env:       c.Env,
		Timeout:   mcpTimeoutDuration(c.TimeoutSec),
	}
}

// ShellExecConfig holds configuration for shell execution tools.
type ShellExecConfig struct {
	Dir string
	Env map[string]string
}

// SearchConfig holds configuration for search-related tools (geminifetch, google_search).
type SearchConfig struct {
	GeminiModel  string
	GoogleAPIKey string
	GoogleCX     string
}

// ClaudeCodeConfig holds configuration for the claudecode toolset.
type ClaudeCodeConfig struct {
	Dir              string
	ReadOnly         bool
	MaxFileSize      int64
	WebFetch         *WebFetchConfig
	WebSearch        *WebSearchConfig
	CommandAllowList []string
}

// MCPConfig holds configuration for MCP server and broker tools.
type MCPConfig struct {
	Servers []MCPServerConfig
	Broker  *MCPBrokerConfig
}

// SessionConfig holds configuration for session-scoped tools (memory, custom, message, subagent, blob).
type SessionConfig struct {
	MemoryEnabled   bool
	MemoryTools     []Tool
	CustomTools     []Tool
	OutboundRouter  *outbound.Router
	SubAgentService *subagenttool.Service
	BlobReader      biz.ToolResultBlobReader
}

// AssemblyConfig holds all configuration for tool assembly.
// Sub-configs group related fields to keep the top-level field count within
// the AS-COG-01 limit of 15.
type AssemblyConfig struct {
	EnabledTools  []string
	DeferredTools []string
	FilesystemDir string
	ShellExec     ShellExecConfig
	Search        SearchConfig
	ClaudeCode    ClaudeCodeConfig
	OpenAPISpecs  []OpenAPISpecConfig
	AgentTools    []AgentToolConfig
	MCP           MCPConfig
	Session       SessionConfig
	Browser       *browser.PlaywrightMCPConfig
	Lg            loggateway.Logger
}

type OpenAPISpecConfig struct {
	Name     string
	SpecURL  string
	SpecData []byte
}

type WebFetchConfig struct {
	AllowAll         bool
	AllowedDomains   []string
	BlockedDomains   []string
	Timeout          time.Duration
	MaxContentLength int
}

type WebSearchConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	EngineID string
}

type AssembledToolsets struct {
	ToolSets        []ToolSet
	Tools           []Tool
	DeferredManager *deferred.DeferredToolManager
}

func Assemble(ctx context.Context, cfg AssemblyConfig) (*AssembledToolsets, error) {
	lg := cfg.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	started := time.Now()
	lg.Info("tools.Assemble started",
		loggateway.StepID("tool.assemble.start"),
		loggateway.Int("enabled_tools", len(cfg.EnabledTools)),
		loggateway.Int("deferred_tools", len(cfg.DeferredTools)),
	)
	if err := ValidateRuntimeAliasesAgainstPolicy(); err != nil {
		return nil, apierror.Internal(apierror.DomainTool, "alias validation: "+err.Error())
	}

	enabled := make(map[string]bool, len(cfg.EnabledTools))
	for _, name := range cfg.EnabledTools {
		enabled[name] = true
	}
	deferredSet := make(map[string]bool, len(cfg.DeferredTools))
	for _, name := range cfg.DeferredTools {
		deferredSet[name] = true
	}

	ac := &assembleContext{
		ctx:         ctx,
		cfg:         cfg,
		out:         &AssembledToolsets{},
		enabled:     enabled,
		deferredSet: deferredSet,
		lg:          lg,
	}

	// Phase 1: registry factories
	if err := ac.assembleFromRegistry(); err != nil {
		return nil, err
	}
	// Phase 2: builtin toolsets (file, hostexec, document, spreadsheet)
	if err := ac.assembleBuiltinToolsets(); err != nil {
		return nil, err
	}
	// Phase 3: search tools (geminifetch, google_search)
	if err := ac.assembleSearchTools(); err != nil {
		return nil, err
	}
	// Phase 4: claudecode with sandbox
	if err := ac.assembleClaudeCodeToolset(); err != nil {
		return nil, err
	}
	// Phase 5: OpenAPI toolsets
	if err := ac.assembleOpenAPIToolsets(); err != nil {
		return nil, err
	}
	// Phase 6: agent-as-tool
	ac.assembleAgentTools()
	// Phase 7: MCP server and broker
	if err := ac.assembleMCPTools(); err != nil {
		return nil, err
	}
	// Phase 8: session-scoped tools (memory, custom, message, subagent)
	ac.assembleSessionTools()
	// Phase 9: browser
	if err := ac.assembleBrowserToolset(); err != nil {
		return nil, err
	}
	// Phase 10: blob reader and deferred tools
	if err := ac.assembleBlobAndResultTools(); err != nil {
		return nil, err
	}

	ApplyDisambiguationHints(ac.out.Tools)
	for _, ts := range ac.out.ToolSets {
		ApplyDisambiguationHints(ts.Tools(ctx))
	}

	// K1 出口摘要：toolset 数 + 工具 key 列表 + 耗时。仅枚举独立工具与
	// toolset 名（不调用 ts.Tools(ctx) 展开成员，避免 MCP 等 toolset 触发额外 I/O）。
	toolKeys := make([]string, 0, len(ac.out.Tools))
	for _, t := range ac.out.Tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		toolKeys = append(toolKeys, t.Declaration().Name)
	}
	toolsetNames := make([]string, 0, len(ac.out.ToolSets))
	for _, ts := range ac.out.ToolSets {
		if ts == nil {
			continue
		}
		toolsetNames = append(toolsetNames, ts.Name())
	}
	lg.Info("tools.Assemble completed",
		loggateway.StepID("tool.assemble.complete"),
		loggateway.Int("toolsets", len(ac.out.ToolSets)),
		loggateway.Int("tools", len(ac.out.Tools)),
		loggateway.Str("tool_keys", strings.Join(toolKeys, ",")),
		loggateway.Str("toolset_names", strings.Join(toolsetNames, ",")),
		loggateway.Duration(time.Since(started).Milliseconds()),
	)

	return ac.out, nil
}

// DefaultMCPServerTimeoutSec is applied when config_json.timeout_sec is unset.
const DefaultMCPServerTimeoutSec = mcpdefaults.DefaultRuntimeTimeoutSec

// DefaultMCPToolsCacheTTL is the default TTL for caching an MCP server's tool
// list. llmagent calls ToolSet.Tools(ctx) on every run when
// RefreshToolSetsOnRun is enabled; without caching each run pays a tools/list
// roundtrip (plus session reconnect when the transport expired), which
// dominated the pre-orchestration latency in the 2026-08-06 20:45 session.
const DefaultMCPToolsCacheTTL = 5 * time.Minute

func mcpTimeoutDuration(timeoutSec int) time.Duration {
	if timeoutSec <= 0 {
		timeoutSec = DefaultMCPServerTimeoutSec
	}
	return parseDurationSec(timeoutSec)
}

func buildMCPToolSet(cfg MCPServerConfig) (ToolSet, error) {
	connCfg := cfg.ToConnectionConfig()

	opts := []trpcmcp.ToolSetOption{
		trpcmcp.WithName(cfg.Name),
		trpcmcp.WithToolsCacheTTL(DefaultMCPToolsCacheTTL),
	}
	if pred := ToolFilterForPrefix(cfg.ToolPrefix); pred != nil {
		opts = append(opts, trpcmcp.WithToolFilterFunc(pred))
	}
	reconnectMax := mcpobserve.EffectiveSessionReconnectMax(connCfg.Transport, cfg.SessionReconnectMax)
	if reconnectMax > 0 {
		opts = append(opts, trpcmcp.WithSessionReconnect(reconnectMax))
	}
	if cfg.HeaderInjector != nil {
		injector := cfg.HeaderInjector
		opts = append(opts, trpcmcp.WithMCPOptions(tmcp.WithHTTPBeforeRequest(
			func(ctx context.Context, req *http.Request) error {
				if req == nil {
					return nil
				}
				headers, err := injector(ctx)
				if err != nil {
					return err
				}
				for k, v := range headers {
					req.Header.Set(k, v)
				}
				return nil
			},
		)))
	}

	return trpcmcp.NewMCPToolSet(connCfg, opts...), nil
}

func buildMCPBrokerTools(cfg MCPBrokerConfig) ([]Tool, error) {
	servers := make(map[string]trpcmcp.ConnectionConfig, len(cfg.Servers))
	for _, s := range cfg.Servers {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		servers[name] = s.ToConnectionConfig()
	}

	brokerOpts := []trpcmcpbroker.Option{}
	if len(servers) > 0 {
		brokerOpts = append(brokerOpts, trpcmcpbroker.WithServers(servers))
	}
	if cfg.AllowAdHocHTTP {
		brokerOpts = append(brokerOpts, trpcmcpbroker.WithAllowAdHocHTTP(true))
	}
	brokerOpts = append(brokerOpts, trpcmcpbroker.WithAdHocHTTPTimeout(mcpTimeoutDuration(cfg.AdHocTimeoutSec)))
	if cfg.HeaderInjector != nil {
		brokerOpts = append(brokerOpts, trpcmcpbroker.WithHTTPHeaderInjector(cfg.HeaderInjector))
	}

	broker := trpcmcpbroker.New(brokerOpts...)
	tools := broker.Tools()
	result := make([]Tool, len(tools))
	for i, t := range tools {
		result[i] = t
	}
	return result, nil
}

func ToolFilterForPrefix(prefix string) ToolFilterFunc {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	return func(_ context.Context, t Tool) bool {
		if t == nil || t.Declaration() == nil {
			return false
		}
		return strings.HasPrefix(t.Declaration().Name, prefix)
	}
}

type ToolFilterFunc = func(context.Context, Tool) bool

func parseDurationSec(sec int) time.Duration {
	return time.Duration(sec) * time.Second
}

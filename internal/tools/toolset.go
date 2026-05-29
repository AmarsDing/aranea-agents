package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"

	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/tools/hostexecnorm"
	"aranea-agents/internal/tools/mcpobserve"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcagenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpcarxivsearch "trpc.group/trpc-go/trpc-agent-go/tool/arxivsearch"
	trpcawaitreply "trpc.group/trpc-go/trpc-agent-go/tool/awaitreply"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/tool/claudecode"
	trpcduckduckgo "trpc.group/trpc-go/trpc-agent-go/tool/duckduckgo"
	trpcemail "trpc.group/trpc-go/trpc-agent-go/tool/email"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
	trpcgooglesearch "trpc.group/trpc-go/trpc-agent-go/tool/google/search"

	memorytool "aranea-agents/internal/tools/memory"

	trpchostexec "trpc.group/trpc-go/trpc-agent-go/tool/hostexec"
	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
	trpcopenapi "trpc.group/trpc-go/trpc-agent-go/tool/openapi"
	trpctodo "trpc.group/trpc-go/trpc-agent-go/tool/todo"
	trpcgeminifetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch"
	trpchttpfetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch"
	trpcwikipedia "trpc.group/trpc-go/trpc-agent-go/tool/wikipedia"
)

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
					return nil, nil
				},
				EnabledByDefault:    true,
				RiskLevel:           "low",
				SupportsConcurrency: true,
			},
			{
				Name:        "hostexec",
				Description: "Host command execution ToolSet (shell, bash, powershell)",
				Category:    "execution",
				Tags:        []string{"shell", "exec", "command"},
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
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
			},
			{
				Name:        "claudefetch",
				Description: "Claude web fetch tool (Claude-powered page extraction) — framework stub, not yet implemented",
				Category:    "web",
				Tags:        []string{"web", "fetch", "claude"},
				Factory: func(ctx context.Context) (Tool, error) {
					return nil, nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
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
					return nil, nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "workspace_exec",
				Description: "Workspace execution tools (exec, write_stdin, kill_session)",
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
					return nil, fmt.Errorf("openapi requires spec configuration")
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
		}
	})
	return registry
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
}

type MCPBrokerConfig struct {
	Servers         []MCPServerConfig
	AllowAdHocHTTP  bool
	AdHocTimeoutSec int
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
		Timeout:   mcpTimeoutDuration(c.TimeoutSec),
	}
}

type AssemblyConfig struct {
	EnabledTools  []string
	FilesystemDir string
	ShellExecDir  string
	GeminiModel   string
	GoogleAPIKey  string
	GoogleCX      string
	ClaudeCodeDir string
	OpenAPISpecs  []OpenAPISpecConfig
	AgentTools    []AgentToolConfig
	MCPServers    []MCPServerConfig
	MCPBroker     *MCPBrokerConfig
	MemoryEnabled bool
	CustomTools   []Tool
}

type OpenAPISpecConfig struct {
	Name     string
	SpecURL  string
	SpecData []byte
}

type AssembledToolsets struct {
	ToolSets []ToolSet
	Tools    []Tool
}

func Assemble(ctx context.Context, cfg AssemblyConfig) (*AssembledToolsets, error) {
	// TPM-P1-01: fail fast if runtime/policy alias maps disagree on canonical target.
	if err := ValidateRuntimeAliasesAgainstPolicy(); err != nil {
		return nil, fmt.Errorf("tools.Assemble: %w", err)
	}
	out := &AssembledToolsets{}
	enabled := make(map[string]bool, len(cfg.EnabledTools))
	for _, name := range cfg.EnabledTools {
		enabled[name] = true
	}

	for _, reg := range Registry() {
		if !enabled[reg.Name] {
			continue
		}
		if reg.ToolSetFactory != nil {
			ts, err := reg.ToolSetFactory(ctx)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", reg.Name, err)
			}
			if ts != nil {
				out.ToolSets = append(out.ToolSets, ts)
			} else {
				event.SysLogWarn("system.tool_assembly_skip", "tools.assemble.factory_nil",
					event.P("tool", reg.Name),
					event.P("reason", "factory returned nil without error"))
			}
		} else if reg.Factory != nil {
			t, err := reg.Factory(ctx)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", reg.Name, err)
			}
			if t != nil {
				out.Tools = append(out.Tools, t)
			} else {
				event.SysLogWarn("system.tool_assembly_skip", "tools.assemble.factory_nil",
					event.P("tool", reg.Name),
					event.P("reason", "factory returned nil without error"))
			}
		}
	}

	if enabled["file"] {
		var opts []trpcfile.Option
		if cfg.FilesystemDir != "" {
			opts = append(opts, trpcfile.WithBaseDir(cfg.FilesystemDir))
		}
		ts, err := trpcfile.NewToolSet(opts...)
		if err != nil {
			return nil, fmt.Errorf("file toolset: %w", err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	if enabled["hostexec"] {
		var opts []trpchostexec.Option
		if cfg.ShellExecDir != "" {
			opts = append(opts, trpchostexec.WithBaseDir(cfg.ShellExecDir))
		}
		ts, err := trpchostexec.NewToolSet(opts...)
		if err != nil {
			return nil, fmt.Errorf("hostexec toolset: %w", err)
		}
		out.ToolSets = append(out.ToolSets, hostexecnorm.WrapToolSet(ts))
	}

	if enabled["geminifetch"] {
		if model := strings.TrimSpace(cfg.GeminiModel); model != "" {
			t, err := trpcgeminifetch.NewTool(model)
			if err != nil {
				return nil, fmt.Errorf("geminifetch: %w", err)
			}
			out.Tools = append(out.Tools, t)
		} else {
			event.SysLogWarn("system.tool_assembly_skip", "tools.assemble.geminifetch_no_model",
				event.P("reason", "gemini_model config is empty"))
		}
	}

	if enabled["google_search"] {
		apiKey := strings.TrimSpace(cfg.GoogleAPIKey)
		cx := strings.TrimSpace(cfg.GoogleCX)
		if apiKey != "" && cx != "" {
			ts, err := trpcgooglesearch.NewToolSet(ctx,
				trpcgooglesearch.WithAPIKey(apiKey),
				trpcgooglesearch.WithEngineID(cx),
			)
			if err != nil {
				return nil, fmt.Errorf("google search: %w", err)
			}
			out.ToolSets = append(out.ToolSets, ts)
		} else {
			event.SysLogWarn("system.tool_assembly_skip", "tools.assemble.google_search_no_config",
				event.P("reason", "api_key or cx is empty"))
		}
	}

	if enabled["claudecode"] {
		var opts []trpcclaudecode.Option
		if cfg.ClaudeCodeDir != "" {
			opts = append(opts, trpcclaudecode.WithBaseDir(cfg.ClaudeCodeDir))
		}
		ts, err := trpcclaudecode.NewToolSet(opts...)
		if err != nil {
			return nil, fmt.Errorf("claudecode: %w", err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	for _, spec := range cfg.OpenAPISpecs {
		var specLoader trpcopenapi.Loader
		var err error
		if len(spec.SpecData) > 0 {
			specLoader, err = trpcopenapi.NewDataLoader(spec.SpecData)
		} else if spec.SpecURL != "" {
			specLoader, err = trpcopenapi.NewURILoader(spec.SpecURL)
		}
		// TPM-P2-02: log loader failures so misconfigurations are visible instead
		// of silently disappearing. We continue rather than aborting all assembly
		// so a bad spec doesn't block unrelated toolsets.
		if err != nil {
			event.SysLogWarn("system.builtin_tools_sync_fail", "tools.assemble.openapi_loader_failed",
				event.P("spec_name", spec.Name),
				event.P("spec_url", spec.SpecURL),
				event.P("error", err.Error()))
			continue
		}
		if specLoader == nil {
			continue
		}
		ts, err := trpcopenapi.NewToolSet(ctx,
			trpcopenapi.WithSpecLoader(specLoader),
			trpcopenapi.WithName(spec.Name),
		)
		if err != nil {
			return nil, fmt.Errorf("openapi %s: %w", spec.Name, err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	for _, atCfg := range cfg.AgentTools {
		opts := []trpcagenttool.Option{
			trpcagenttool.WithSkipSummarization(atCfg.SkipSummarization),
			trpcagenttool.WithStreamInner(atCfg.StreamInner),
		}
		if atCfg.Description != "" {
			opts = append(opts, trpcagenttool.WithDescription(atCfg.Description))
		}
		if atCfg.HistoryScope > 0 {
			opts = append(opts, trpcagenttool.WithHistoryScope(atCfg.HistoryScope))
		}
		if atCfg.ResponseMode > 0 {
			opts = append(opts, trpcagenttool.WithResponseMode(atCfg.ResponseMode))
		}
		t := trpcagenttool.NewTool(atCfg.Agent, opts...)
		out.Tools = append(out.Tools, t)
	}

	for _, mcpCfg := range cfg.MCPServers {
		ts, err := buildMCPToolSet(mcpCfg)
		if err != nil {
			return nil, fmt.Errorf("mcp %s: %w", mcpCfg.Name, err)
		}
		if ts != nil {
			out.ToolSets = append(out.ToolSets, ts)
		}
	}

	if enabled["mcpbroker"] && cfg.MCPBroker != nil {
		brokerTools, err := buildMCPBrokerTools(*cfg.MCPBroker)
		if err != nil {
			return nil, fmt.Errorf("mcpbroker: %w", err)
		}
		out.Tools = append(out.Tools, brokerTools...)
	}

	if cfg.MemoryEnabled {
		out.Tools = append(out.Tools, memorytool.DefaultTools()...)
	}

	out.Tools = append(out.Tools, cfg.CustomTools...)

	return out, nil
}

// DefaultMCPServerTimeoutSec is applied when config_json.timeout_sec is unset.
const DefaultMCPServerTimeoutSec = mcpdefaults.DefaultRuntimeTimeoutSec

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
		trpcmcp.WithReconnectObserver(mcpobserve.ObserverForServer(cfg.Name)),
	}
	if pred := ToolFilterForPrefix(cfg.ToolPrefix); pred != nil {
		opts = append(opts, trpcmcp.WithToolFilterFunc(pred))
	}
	reconnectMax := mcpobserve.EffectiveSessionReconnectMax(connCfg.Transport, cfg.SessionReconnectMax)
	if reconnectMax > 0 {
		opts = append(opts, trpcmcp.WithSessionReconnect(reconnectMax))
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

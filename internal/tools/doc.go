package tools

// Package tools provides tool implementations for the aranea-agents platform.
//
// Directory structure:
//
//	tools/
//	├── tool.go        — Project-level tool interface definitions (type aliases to trpc-agent-go/tool)
//	├── toolset.go     — Central tool registry + Assemble() entry point
//	│                    Registry() lists all available tools with metadata and factory functions.
//	│                    Assemble() builds ToolSets and Tools based on an AssemblyConfig.
//	├── doc.go         — This documentation file
//	├── trpc/          — Backward-compatible adapter: ToolsetConfig → AssemblyConfig → tools.Assemble()
//	│                    Existing callers (trpc_build.go) continue to use BuildToolsets().
//	├── custom/        — Custom tool implementations (user-defined tools)
//	│                    Add your own tools here following the demo pattern.
// MCP 连接配置见 internal/mcp/config；运行时装配见 internal/agent/tool_assembly.go + toolset.go
//	├── skillrouter/   — Skill detection and taxonomy
//	└── skillruntime/  — Skill-based toolset resolution
//
// Framework interfaces (pkg/trpc-agent-go/tool):
//
//	tool.Tool           — base interface: Declaration() *tool.Declaration
//	tool.CallableTool   — Tool + Call(ctx, jsonArgs) (any, error)
//	tool.StreamableTool — Tool + StreamableCall(ctx, jsonArgs) (*StreamReader, error)
//	tool.ToolSet        — Tools(ctx) []Tool + Close() error + Name() string
//
// Project-level types (this package):
//
//	Tool, CallableTool, StreamableTool, ToolSet, Declaration, Schema
//	  — Type aliases to trpc-agent-go/tool equivalents for convenience
//	ToolRegistration — Describes a tool's metadata, factory, category, and default state
//	AssemblyConfig   — Input to Assemble(): which tools to enable + configuration
//	AssembledToolsets — Output of Assemble(): assembled ToolSets and Tools
//
// Framework capabilities (pkg/trpc-agent-go/tool):
//
//	1. Callbacks — Tool execution lifecycle hooks
//	   tool.Callbacks manages BeforeTool/AfterTool callback chains:
//	     - BeforeToolCallbackStructured: pre-execution hook (skip execution, modify args, inject context)
//	     - AfterToolCallbackStructured: post-execution hook (replace result, skip summarization)
//	     - ToolResultMessagesFunc: custom tool result → model message conversion
//	     - WithContinueOnError / WithContinueOnResponse: callback chain control
//	   Usage: tool.NewCallbacks().RegisterBeforeTool(cb).RegisterAfterTool(cb)
//	   Agent integration: llmagent.WithToolCallbacks(callbacks)
//
//	2. Filtering — Dynamic tool visibility and execution control
//	   tool.FilterFunc: func(ctx context.Context, tool Tool) bool
//	   Built-in filters:
//	     - tool.NewIncludeToolNamesFilter(names...): whitelist filter
//	     - tool.NewExcludeToolNamesFilter(names...): blacklist filter
//	     - tool.FilterTools(ctx, tools, filter): filter a tool slice
//	     - tool.FilterToolSet(toolset, filter): wrap a ToolSet with filtering
//	   Two-layer filtering in llmagent:
//	     - WithToolFilter: controls which tools are visible to the model
//	     - WithToolExecutionFilter: controls which tool calls are auto-executed
//	   Project integration: ToolsDenyJSON → buildToolFilter() → WithToolFilter
//
//	3. Retry — Automatic retry for transient tool failures
//	   tool.RetryPolicy:
//	     - MaxAttempts, InitialInterval, BackoffFactor, MaxInterval, Jitter
//	     - RetryOn(ctx, *RetryInfo) (bool, error): custom retry decision
//	   tool.DefaultRetryOn: retries on EOF, unexpected EOF, network timeout/temporary errors
//	   Agent integration: llmagent.WithToolCallRetryPolicy(policy)
//	   Project integration: buildToolRetryPolicy() → SelectiveRetryOn → WithToolCallRetryPolicy
//
//	4. Merge — Result aggregation for streaming and multi-tool outputs
//	   tool.Merge[T](ts []T) T: generic merge function
//	   Supports: string concatenation, number sum, slice concat, map merge,
//	             struct field-by-field merge, custom Mergeable interface
//
//	5. Streaming — Bidirectional streaming infrastructure
//	   tool.NewStream(bufferSize) → Stream{Reader, Writer}
//	   tool.StreamChunk{Content, Metadata}: streaming data unit
//	   tool.FinalResultChunk: marks final structured result of a streamable tool
//	   tool.FinalResultStateChunk: final result + state delta
//	   StreamableFunctionTool[I,O]: wraps a function returning (*StreamReader, error)
//	   Context markers:
//	     - WithStructuredStreamErrors / StructuredStreamErrorsFromContext
//	     - WithFinalResultChunks / FinalResultChunksFromContext
//
//	6. Context utilities
//	   tool.ToolCallIDFromContext(ctx): retrieve model-assigned tool call ID
//	   tool.ContextKeyToolCallID: context key for tool call ID injection
//
//	7. Inner text mode (stream preferences)
//	   tool.InnerTextMode: controls inner assistant text visibility in parent flow
//	     - InnerTextModeDefault / InnerTextModeInclude / InnerTextModeExclude
//	   Used by agent.Tool (WithInnerTextMode) for agent-as-tool composition
//
// Tool registry (Registry()):
//
//	All framework tools are registered with name, description, category, factory, and default state.
//	To add a new tool:
//	  1. Add a ToolRegistration entry in toolset.go Registry()
//	  2. If it needs configuration, add fields to AssemblyConfig
//	  3. Add a seed entry in internal/data/builtin_tools_seed.go with matching key
//
//	Registered tools:
//	  file             — File operation ToolSet (filesystem)
//	  hostexec         — Host command execution ToolSet (execution)
//	  httpfetch        — HTTP web page fetch tool (web)
//	  geminifetch      — Gemini web fetch tool (web)
//	  duckduckgo       — DuckDuckGo web search tool (search)
//	  google_search    — Google Custom Search ToolSet (search)
//	  arxiv_search     — ArXiv paper search ToolSet (search)
//	  wikipedia        — Wikipedia search ToolSet (search)
//	  email            — Email sending ToolSet (communication)
//	  message          — Outbound messaging ToolSet (communication)
//	  todo             — Todo management tool (productivity)
//	  await_user_reply — Mark agent as waiting for user reply (interaction)
//	  claudecode       — Claude Code ToolSet (coding)
//	  workspace_exec   — Workspace execution tools (execution, NOT YET IMPLEMENTED)
//	  openapi          — OpenAPI spec ToolSet (integration)
//	  agent            — Agent-as-Tool for delegation/composition (composition)
//	  mcp              — MCP ToolSet: connect to MCP servers (integration)
//	  mcpbroker        — MCP Broker: runtime MCP discovery tools (integration)
//	  model_registry_sync — Model registry sync tools (system)
//	  subagents_spawn  — Spawn a background subagent (composition)
//	  subagents_list   — List background subagents (composition)
//	  subagents_get    — Get status/result of a background subagent (composition)
//	  subagents_cancel — Cancel a background subagent run (composition)
//	  browser          — Browser automation tool via Playwright MCP (browser)
//	  read_document    — Read a document (PDF, DOCX, text) (media)
//	  read_spreadsheet — Read tabular files (XLSX, CSV) (media)
//	  read_tool_result — Retrieve persisted tool result by blob_id (system)
//	  working_memory   — Working memory tools for task-scoped fields (memory)
//	  deliverable      — Cross-agent deliverable handoff tools (team)
//	  client           — Client tool bridge ToolSet (interaction)
//	  datetime         — Current date/time/timezone tool (system)
//	  media            — Media generation tools (media)
//
// Framework tool sub-packages not in registry (integrated differently):
//	  function/   — FunctionTool[I,O] — used by custom/ tools
//	  agent/      — Agent-as-Tool — used by Team编排 via llmagent options
//	  transfer/   — TransferTool — injected by Team编排 automatically
//	  codeexec/   — CodeExecutionTool — injected via llmagent.WithCodeExecutor
//	  skill/      — Skill tools — injected via llmagent.WithSkills
//	  mcp/        — MCP ToolSet — internal/tools/toolset.go buildMCPToolSet
//	  mcpbroker/  — MCP Broker — internal/tools/toolset.go buildMCPBrokerTools
//
// Custom tool guide:
//
//  1. Create a new file in custom/ (e.g. custom/my_tool.go)
//  2. Define input/output structs with jsonschema tags
//  3. Implement an execute function: func(ctx context.Context, input MyInput) (MyOutput, error)
//  4. Wrap with function.NewFunctionTool(fn, WithName("..."), WithDescription("..."))
//  5. Register it in ToolsetConfig.CustomTools or via ToolsetConfig flag
//  6. Add a seed entry in internal/data/builtin_tools_seed.go with matching key
//
// Tool key naming convention:
//
//	Tool keys in builtin_tools_seed.go MUST match the Declaration().Name returned by the
//	framework tool. This ensures the effective-tool policy (allow/deny/profile) and the
//	runtime toolset builder (trpc_build.go) use consistent identifiers.
//
// See custom/demo.go for a complete working example.

package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizmedia "aranea-agents/internal/biz/media"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/browser"
	"aranea-agents/internal/tools/deferred"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/tools/memory"
	"aranea-agents/internal/tools/officecli"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/internal/tools/twinops"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// shard_plan.go — P0-2 阶段A：分片构建计划（分片定义 + 规范化指纹 + computeShardPlan）。
//
// 分片边界（report-05 FR-2.1 装配组粒度，按构建输入亲缘性切分）：
//
//	core        注册表/内建/搜索/claudecode/browser/message/subagent/kanban/
//	            computeruse/webresearch/callagent/await + 运行时配置 + 工作区目录。
//	            指纹含 AgentID（保守：工作区目录按 agent 解析，放弃跨 agent 共享）。
//	mcp:<key>   每个直连 MCP server 一片；指纹 = 解密+鉴权头解析后的
//	            MCPServerConfig（不含 HeaderInjector）。HeaderInjector != nil
//	            （按用户注入凭证）→ cacheable=false，绝不跨 agent 共享（红线）。
//	mcp_broker  broker 工具集；HeaderInjector != nil → cacheable=false。
//	            BrokerFallback 不建片：它是治理降级的备用输入，由合并期消费。
//	memory      记忆工具（service 工具/默认工具）+ working_memory + compact。
//	knowledge   knowledge_search/reflect + knowledge_write。
//	media       generate_image/generate_video/image_to_video（提供方配置进指纹）。
//	twinops     twin_*/gns3_* 白名单子集 + env 连接配置。
//	officecli   officecli_read/write/render + env 配置 + agent 工作区根。
//	custom      deps.CustomTools（调用方注入，身份每构建不同）→ cacheable=false。
//
// 不建片的横切处理（合并期对并集统一重放，均为非变异包装器，共享安全）：
// MCP schema 治理、延迟工具目录/包装、flat 名去重、消歧提示、运行时别名、
// 确认门、装饰器。策略类字段（超时等）只影响合并期重放，不进任何分片指纹——
// 策略变更时所有分片命中，仅重放阶段重跑（report-05 风险④的阶段A 形态）。
//
// 指纹规范：显式投影结构体 + canonical JSON（encoding/json map 键排序）+
// sha256。凡构建产物消费的输入必须进指纹；拿不准的一律进指纹（FR-2.3 保守
// 默认）。序列化失败 → 随机指纹（恒未命中，退化为每构建新建，绝不错误共享）。

// 分片组名（用于指标标签 / 日志 / hold 占位符命名）。
const (
	shardGroupCore      = "core"
	shardGroupMCP       = "mcp"
	shardGroupMCPBroker = "mcp_broker"
	shardGroupMemory    = "memory"
	shardGroupKnowledge = "knowledge"
	shardGroupMedia     = "media"
	shardGroupTwinOps   = "twinops"
	shardGroupOfficeCLI = "officecli"
	shardGroupCustom    = "custom"
)

// shardPlan 是一次 agent 构建的分片计划：待获取分片（固定确定性顺序）+
// 合并期元数据（治理范围 / broker 降级配置 / 延迟工具清单）。
type shardPlan struct {
	specs []shardSpec
	// mcpIdx 是直连 MCP 分片在 specs 中的下标（合并期治理范围）。
	mcpIdx []int
	// brokerIdx 是 broker 分片在 specs 中的下标；-1 表示未启用 broker。
	brokerIdx int
	// brokerFallback 是治理降级备用 broker 配置（P1-2 语义）：直连 declaration
	// 总量超预算且无 broker 分片时，合并期用它现场构建 broker 工具。
	brokerFallback *tooltrpc.MCPBrokerConfig
	// deferredTools 是合并期 FinalizeDeferredTools 的输入（registry 名）。
	deferredTools []string
}

// shardFingerprint 计算规范化指纹：投影结构体 → canonical JSON → sha256。
// group 前缀隔离跨组命名空间。序列化失败返回随机指纹（crypto/rand；恒未命中，
// 安全降级——不依赖 time.Now()，Windows 时钟粒度下相邻调用可能同值）。
func shardFingerprint(group string, v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		var nonce [16]byte
		if _, rerr := rand.Read(nonce[:]); rerr != nil {
			// 熵源不可用的极端兜底：纳秒时间戳（此时仅保证非空指纹）。
			raw = []byte(fmt.Sprintf("%s:marshal-error:%d", group, time.Now().UnixNano()))
		} else {
			raw = []byte(fmt.Sprintf("%s:marshal-error:%x", group, nonce[:]))
		}
	}
	sum := sha256.Sum256(raw)
	return group + ":" + hex.EncodeToString(sum[:])
}

// ---------- 指纹投影结构体（仅含影响构建产物的 JSON 安全字段） ----------

// coreShardFP 是 core 分片的指纹投影：core 受限 ToolsetConfig 的全部
// 可序列化字段 + 不可序列化依赖（hook/桥/单例服务）的存在性布尔。
type coreShardFP struct {
	AgentID string // 保守：工作区目录按 agent 解析，core 放弃跨 agent 共享

	Filesystem    bool
	FilesystemDir string
	ShellExec     bool
	ShellExecDir  string
	ShellExecEnv  map[string]string
	WebFetch      bool
	WebSearch     bool
	WebResearch   bool
	WebResearchCfg webresearchpkg.Config
	GeminiFetch   bool
	GeminiModel   string
	GoogleSearch  bool
	GoogleAPIKey  string
	GoogleCX      string
	ArxivSearch   bool
	Wikipedia     bool
	Email         bool
	Todo          bool
	AwaitReply    bool
	HasAwaitHook  bool
	ClaudeCode    bool
	ClaudeCodeDir string
	// OpenAPISpecs / AgentTools 当前构建路径恒空；保留投影防未来接线漂移
	// （一旦接线，值变化自动反映进指纹）。
	OpenAPISpecs []tooltrpc.OpenAPISpecConfig
	NumAgentTools int
	WorkspaceExec bool
	CallAgent     bool
	Kanban        bool
	HasKanbanBridge bool
	ReadDocument    bool
	ReadSpreadsheet bool
	Datetime        bool
	Message         bool
	HasOutboundRouter bool
	SubAgent          bool
	HasSubAgentService bool
	ClientBridge       bool
	HasClientBridgeSvc bool
	CodingBridge       bool
	HasCodingBridgeSvc bool
	BrowserEnabled     bool
	Browser            *browser.PlaywrightMCPConfig
	ComputerUse        bool
	HasComputerUseUC   bool
	HasBlobReader      bool
}

// mcpServerShardFP 是单个直连 MCP server 分片的指纹投影。
// HeaderInjector 不进指纹：其存在即 cacheable=false（指纹仅用于缓存键，
// 不可缓存分片的指纹仅作日志/占位符命名用途）。
type mcpServerShardFP struct {
	Name                   string
	Transport              string
	ServerURL              string
	Command                string
	Args                   []string
	Env                    map[string]string
	Headers                map[string]string
	TimeoutSec             int
	ToolPrefix             string
	SessionReconnectMax    int
	AllowAdHocHTTP         bool
	AdHocTimeoutSec        int
	RequireUserCredentials bool
	AuthHeaderName         string
}

func mcpServerFPFromConfig(c tooltrpc.MCPServerConfig) mcpServerShardFP {
	return mcpServerShardFP{
		Name:                   c.Name,
		Transport:              c.Transport,
		ServerURL:              c.ServerURL,
		Command:                c.Command,
		Args:                   c.Args,
		Env:                    c.Env,
		Headers:                c.Headers,
		TimeoutSec:             c.TimeoutSec,
		ToolPrefix:             c.ToolPrefix,
		SessionReconnectMax:    c.SessionReconnectMax,
		AllowAdHocHTTP:         c.AllowAdHocHTTP,
		AdHocTimeoutSec:        c.AdHocTimeoutSec,
		RequireUserCredentials: c.RequireUserCredentials,
		AuthHeaderName:         c.AuthHeaderName,
	}
}

// mcpBrokerShardFP 是 broker 分片的指纹投影。
type mcpBrokerShardFP struct {
	Servers         []mcpServerShardFP
	AllowAdHocHTTP  bool
	AdHocTimeoutSec int
}

// ---------- computeShardPlan ----------

// computeShardPlan 计算一次 agent 构建的分片计划。配置计算路径完整复刻
// buildToolsetsForAgent 的原始步骤（行为等价是硬性要求），仅在「组装」处
// 改道：完整 cfg 算好后按组切成受限 cfg，逐组计算指纹并生成构建闭包。
//
// 返回 nil 表示无任何工具可构建（等价于原 ToolsetConfigHasAny=false 路径）。
func computeShardPlan(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, plan *toolBuildPlan) (*shardPlan, error) {
	lg := deps.Logger()
	eff := plan.eff
	var cfg tooltrpc.ToolsetConfig

	// 各分片的计划期解析结果（指纹输入 + 构建闭包捕获）。
	var knowledgeReady, knowledgeWriteEnabled bool
	var mediaResolved []resolvedMediaTool
	var deferredTools []string

	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		cfg = tooltrpc.ToolsetConfigFromEffectiveKeys(eff)

		mcpServers, mcpErr := resolveMCPServers(ctx, deps, ag.ID, lg)
		if mcpErr != nil {
			lg.Warn("MCP 服务器解析失败，部分工具可能不可用",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Err(mcpErr))
		}
		platformAllowAdHoc := platformMCPAllowAdHocHTTP(ctx, deps)
		if eff[biz.ToolKeyMCPToolSet] && len(mcpServers) > 0 {
			cfg.MCPServers = mcpServers
		}
		if eff[biz.ToolKeyMCPBroker] {
			if len(mcpServers) > 0 {
				if brokerCfg := buildMCPBrokerFromServers(mcpServers, platformAllowAdHoc); brokerCfg != nil {
					brokerCfg.HeaderInjector = mcpUserCredentialInjector(deps, mcpServers)
					cfg.MCPBroker = brokerCfg
				}
			} else {
				mcpBrokerCfg, err := resolveMCPBrokerConfig(ctx, deps, ag.ID)
				if err == nil && mcpBrokerCfg != nil {
					cfg.MCPBroker = mcpBrokerCfg
				}
			}
		}
		// P1-2: 直连模式挂载 server 但未显式启用 broker 时，预建一份备用
		// broker 配置——合并期治理发现 declaration 总量超预算时用它降级。
		if len(cfg.MCPServers) > 0 && cfg.MCPBroker == nil {
			if fallback := buildMCPBrokerFromServers(cfg.MCPServers, platformAllowAdHoc); fallback != nil {
				fallback.HeaderInjector = mcpUserCredentialInjector(deps, cfg.MCPServers)
				cfg.MCPBrokerFallback = fallback
			}
		}

		// Knowledge tools require both the Usecase (for availability check) and
		// the Retriever (for actual search). Without a Retriever, the tools would
		// be registered but fail at runtime with "retriever not configured in context".
		knowledgeReady = deps.KnowledgeUsecase != nil && !deps.KnowledgeUsecase.IsUnavailable() && deps.KnowledgeRetriever != nil
		cfg.KnowledgeSearch = eff[biz.ToolKeyKnowledgeSearch] && knowledgeReady
		cfg.KnowledgeReflect = eff[biz.ToolKeyKnowledgeReflect] && knowledgeReady
		// knowledge_write（P1）：只需 Usecase（写路径不依赖 Retriever）。
		knowledgeWriteEnabled = eff[biz.ToolKeyKnowledgeWrite] && deps.KnowledgeUsecase != nil
		// CallAgent requires the A2A invoker to be injected at runtime. When A2A
		// is not configured (a2aEnabled=false), prune the flag to avoid registering
		// a tool that always fails with "invoker not configured".
		cfg.CallAgent = eff[biz.ToolKeyCallAgent] && deps.A2AEnabled
		cfg.AwaitHook = deps.AwaitHook

		// Media 两阶段：计划期仅解析提供方配置（进指纹），实例化推迟到分片构建。
		mediaResolved = resolveMediaToolConfigs(ctx, eff, deps)

		deferredTools = resolveDeferredToolNames(ag, eff, lg)
	}

	// 以下块在原路径中位于 ToolsEnabled 门外侧，无条件参与计算。
	if kanbanpkg.Enabled() {
		if deps.KanbanBridge != nil {
			cfg.Kanban = true
			cfg.KanbanBridge = deps.KanbanBridge
		} else {
			lg.Warn("kanban 已启用但 KanbanBridge 未注入，跳过看板工具",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("agent_id", ag.ID))
		}
	}

	memoryMaster := deps.HasMemory && biz.ResolveMemoryRuntimePolicy(ag.Settings).MasterEnabled
	if memoryMaster {
		cfg.MemoryEnabled = true
		if deps.MemoryService != nil {
			cfg.MemoryTools = filterMemoryTools(deps.MemoryService.Tools())
		}
		// Auto-enable working_memory tools when L1 write/read ports are wired
		if deps.HasWorkingMemory() {
			cfg.WorkingMemory = true
		}
	}
	compactEnabled := memoryMaster && deps.ManualCompressor != nil

	if deps.ToolResultGate != nil {
		cfg.BlobReader = deps.ToolResultGate.BlobReader()
	}

	cfg.OutboundRouter = deps.OutboundRouter
	cfg.SubAgentService = deps.SubAgentService
	cfg.ClientBridgeSvc = deps.ClientBridge
	cfg.ComputerUseUC = deps.ComputerUseUC
	cfg.CodingBridgeSvc = deps.CodingBridgeSvc

	lg.Info("工具构建：SubAgentService 检查",
		loggateway.StepID("agent.subagent_check"),
		loggateway.Bool("subagent_service_nil", deps.SubAgentService == nil))

	// 与原路径同序：runtime config → 平台默认 → gemini 模型解析 → prune → 工作区目录。
	applyRuntimeToolConfigs(&cfg, eff, plan.catalog)
	applyWebResearchPlatformDefaults(ctx, deps, &cfg)
	tooltrpc.ResolveGeminiFetchModel(&cfg, ag.Provider, ag.Model)
	if skipped := tooltrpc.PruneUnconfiguredToolFlags(&cfg); len(skipped) > 0 {
		lg.Warn("已跳过未配置凭证的工具，避免构建失败",
			loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Str("skipped_tools", fmt.Sprintf("%v", skipped)))
	}
	if err := applyToolWorkspaceDirs(ctx, ag, deps, &cfg); err != nil {
		lg.Error("工具构建失败", loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, err
	}

	// twinops / officecli / custom 的计划期输入。
	twinopsCfg := twinops.ConfigFromEnv()
	twinopsKeys := twinops.EnabledKeys(eff)
	var officecliCfg officecli.Config
	var officecliKeys []string
	var officecliRoot string
	if officecli.AnyEnabled(eff) {
		officecliCfg = officecli.ConfigFromEnv()
		officecliKeys = officecliEnabledKeys(eff)
		// 工作区解析失败时 fail-closed 跳过挂载（无围栏根目录不放行文件操作），
		// 与原 resolveOfficeCLITools 语义一致。
		if root, err := resolveAgentFilesystemDir(ctx, ag, deps, ""); err != nil {
			lg.Warn("officecli 工作区解析失败，跳过 Office 工具挂载",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Err(err))
			officecliKeys = nil
		} else {
			officecliRoot = root
		}
	}

	// ---------- 切分 ----------

	p := &shardPlan{brokerIdx: -1, brokerFallback: cfg.MCPBrokerFallback, deferredTools: deferredTools}

	// core：完整 cfg 减去被抽出的组。
	coreCfg := cfg
	coreCfg.MCPServers = nil
	coreCfg.MCPBroker = nil
	coreCfg.MCPBrokerFallback = nil
	coreCfg.KnowledgeSearch = false
	coreCfg.KnowledgeReflect = false
	coreCfg.MemoryEnabled = false
	coreCfg.MemoryTools = nil
	coreCfg.WorkingMemory = false
	coreCfg.CustomTools = nil
	coreCfg.DeferredTools = nil
	if coreShardActive(coreCfg) {
		fp := shardFingerprint(shardGroupCore, coreShardFP{
			AgentID: ag.ID,

			Filesystem:    coreCfg.Filesystem,
			FilesystemDir: coreCfg.FilesystemDir,
			ShellExec:     coreCfg.ShellExec,
			ShellExecDir:  coreCfg.ShellExecDir,
			ShellExecEnv:  coreCfg.ShellExecEnv,
			WebFetch:      coreCfg.WebFetch,
			WebSearch:     coreCfg.WebSearch,
			WebResearch:   coreCfg.WebResearch,
			WebResearchCfg: coreCfg.WebResearchCfg,
			GeminiFetch:   coreCfg.GeminiFetch,
			GeminiModel:   coreCfg.GeminiModel,
			GoogleSearch:  coreCfg.GoogleSearch,
			GoogleAPIKey:  coreCfg.GoogleAPIKey,
			GoogleCX:      coreCfg.GoogleCX,
			ArxivSearch:   coreCfg.ArxivSearch,
			Wikipedia:     coreCfg.Wikipedia,
			Email:         coreCfg.Email,
			Todo:          coreCfg.Todo,
			AwaitReply:    coreCfg.AwaitReply,
			HasAwaitHook:  coreCfg.AwaitHook != nil,
			ClaudeCode:    coreCfg.ClaudeCode,
			ClaudeCodeDir: coreCfg.ClaudeCodeDir,
			OpenAPISpecs:  coreCfg.OpenAPISpecs,
			NumAgentTools: len(coreCfg.AgentTools),
			WorkspaceExec: coreCfg.WorkspaceExec,
			CallAgent:     coreCfg.CallAgent,
			Kanban:        coreCfg.Kanban,
			HasKanbanBridge: coreCfg.KanbanBridge != nil,
			ReadDocument:    coreCfg.ReadDocument,
			ReadSpreadsheet: coreCfg.ReadSpreadsheet,
			Datetime:        coreCfg.Datetime,
			Message:         coreCfg.Message,
			HasOutboundRouter:  coreCfg.OutboundRouter != nil,
			SubAgent:           coreCfg.SubAgent,
			HasSubAgentService: coreCfg.SubAgentService != nil,
			ClientBridge:       coreCfg.ClientBridge,
			HasClientBridgeSvc: coreCfg.ClientBridgeSvc != nil,
			CodingBridge:       coreCfg.CodingBridge,
			HasCodingBridgeSvc: coreCfg.CodingBridgeSvc != nil,
			BrowserEnabled:     coreCfg.BrowserEnabled,
			Browser:            coreCfg.Browser,
			ComputerUse:        coreCfg.ComputerUse,
			HasComputerUseUC:   coreCfg.ComputerUseUC != nil,
			HasBlobReader:      coreCfg.BlobReader != nil,
		})
		buildCfg := coreCfg
		buildCfg.SkipMCPGovernance = true
		buildCfg.SkipPostProcess = true
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupCore + ":" + ag.ID,
			group:     shardGroupCore,
			fp:        fp,
			cacheable: true,
			build:     buildShardViaToolsets(buildCfg, lg),
		})
	}

	// mcp:<server>：每 server 一片，保持解析顺序（与现状的挂载顺序一致）。
	for _, srv := range cfg.MCPServers {
		srv := srv
		fp := shardFingerprint(shardGroupMCP, mcpServerFPFromConfig(srv))
		buildCfg := tooltrpc.ToolsetConfig{
			MCPServers:        []tooltrpc.MCPServerConfig{srv},
			SkipMCPGovernance: true,
			SkipPostProcess:   true,
		}
		p.mcpIdx = append(p.mcpIdx, len(p.specs))
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupMCP + ":" + srv.Name,
			group:     shardGroupMCP,
			fp:        fp,
			cacheable: tools.MCPServerConfigShareable(srv),
			build:     buildShardViaToolsets(buildCfg, lg),
		})
	}

	// mcp_broker。
	if cfg.MCPBroker != nil {
		brokerCfg := *cfg.MCPBroker
		serverFPs := make([]mcpServerShardFP, 0, len(brokerCfg.Servers))
		for _, s := range brokerCfg.Servers {
			serverFPs = append(serverFPs, mcpServerFPFromConfig(s))
		}
		fp := shardFingerprint(shardGroupMCPBroker, mcpBrokerShardFP{
			Servers:         serverFPs,
			AllowAdHocHTTP:  brokerCfg.AllowAdHocHTTP,
			AdHocTimeoutSec: brokerCfg.AdHocTimeoutSec,
		})
		buildCfg := tooltrpc.ToolsetConfig{
			MCPBroker:         &brokerCfg,
			SkipMCPGovernance: true,
			SkipPostProcess:   true,
		}
		p.brokerIdx = len(p.specs)
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupMCPBroker,
			group:     shardGroupMCPBroker,
			fp:        fp,
			cacheable: brokerCfg.HeaderInjector == nil,
			build:     buildShardViaToolsets(buildCfg, lg),
		})
	}

	// memory。
	if cfg.MemoryEnabled || cfg.WorkingMemory {
		var toolNames []string
		for _, t := range cfg.MemoryTools {
			if t == nil || t.Declaration() == nil {
				continue
			}
			toolNames = append(toolNames, t.Declaration().Name)
		}
		fp := shardFingerprint(shardGroupMemory, struct {
			MemoryEnabled    bool
			WorkingMemory    bool
			ServiceToolNames []string
			HasCompressor    bool
		}{cfg.MemoryEnabled, cfg.WorkingMemory, toolNames, compactEnabled})
		buildCfg := tooltrpc.ToolsetConfig{
			MemoryEnabled:   cfg.MemoryEnabled,
			MemoryTools:     cfg.MemoryTools,
			WorkingMemory:   cfg.WorkingMemory,
			SkipPostProcess: true,
		}
		if compactEnabled {
			buildCfg.CustomTools = []trpctool.Tool{memory.NewCompactTool()}
		}
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupMemory,
			group:     shardGroupMemory,
			fp:        fp,
			cacheable: true,
			build:     buildShardViaToolsets(buildCfg, lg),
		})
	}

	// knowledge。
	if cfg.KnowledgeSearch || cfg.KnowledgeReflect || knowledgeWriteEnabled {
		fp := shardFingerprint(shardGroupKnowledge, struct {
			Search bool
			Reflect bool
			Write  bool
			Ready  bool
		}{cfg.KnowledgeSearch, cfg.KnowledgeReflect, knowledgeWriteEnabled, knowledgeReady})
		uc := deps.KnowledgeUsecase
		buildCfg := tooltrpc.ToolsetConfig{
			KnowledgeSearch:  cfg.KnowledgeSearch,
			KnowledgeReflect: cfg.KnowledgeReflect,
			SkipPostProcess:  true,
		}
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupKnowledge,
			group:     shardGroupKnowledge,
			fp:        fp,
			cacheable: true,
			build: func(ctx context.Context) (*shardProduct, error) {
				c := buildCfg
				if knowledgeWriteEnabled {
					if t := knowledgetool.NewWriteTool(uc); t != nil {
						c.CustomTools = []trpctool.Tool{t}
					}
				}
				return buildShardViaToolsets(c, lg)(ctx)
			},
		})
	}

	// media。
	if len(mediaResolved) > 0 {
		fpItems := make([]bizmedia.ProviderConfig, 0, len(mediaResolved))
		for _, r := range mediaResolved {
			fpItems = append(fpItems, r.cfg)
		}
		fp := shardFingerprint(shardGroupMedia, struct {
			Providers        []bizmedia.ProviderConfig
			HasArtifactWriter bool
		}{fpItems, deps.ArtifactWriter != nil})
		mediaDeps := deps
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupMedia,
			group:     shardGroupMedia,
			fp:        fp,
			cacheable: true,
			build: func(ctx context.Context) (*shardProduct, error) {
				c := tooltrpc.ToolsetConfig{SkipPostProcess: true}
				c.CustomTools = buildMediaTools(mediaResolved, mediaDeps)
				if len(c.CustomTools) == 0 {
					return &shardProduct{}, nil
				}
				return buildShardViaToolsets(c, lg)(ctx)
			},
		})
	}

	// twinops。
	if len(twinopsKeys) > 0 {
		fp := shardFingerprint(shardGroupTwinOps, struct {
			Keys           []string
			GatewayBaseURL string
			APIKey         string
			GNS3BaseURL    string
			TimeoutSec     int64
		}{twinopsKeys, twinopsCfg.GatewayBaseURL, twinopsCfg.APIKey, twinopsCfg.GNS3BaseURL, int64(twinopsCfg.Timeout / time.Second)})
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupTwinOps,
			group:     shardGroupTwinOps,
			fp:        fp,
			cacheable: true,
			build: func(ctx context.Context) (*shardProduct, error) {
				tools := twinops.EnabledTools(eff, twinopsCfg)
				if len(tools) == 0 {
					return &shardProduct{}, nil
				}
				return buildShardViaToolsets(tooltrpc.ToolsetConfig{
					CustomTools:     tools,
					SkipPostProcess: true,
				}, lg)(ctx)
			},
		})
	}

	// officecli。
	if len(officecliKeys) > 0 {
		fp := shardFingerprint(shardGroupOfficeCLI, struct {
			Keys              []string
			Bin               string
			TimeoutSec        int64
			Root              string
			HasArtifactWriter bool
		}{officecliKeys, officecliCfg.Bin, int64(officecliCfg.Timeout / time.Second), officecliRoot, deps.ArtifactWriter != nil})
		officeDeps := deps
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupOfficeCLI,
			group:     shardGroupOfficeCLI,
			fp:        fp,
			cacheable: true,
			build: func(ctx context.Context) (*shardProduct, error) {
				tools := officecli.EnabledTools(eff, officecliCfg, officecliRoot, officeDeps.ArtifactWriter)
				if len(tools) == 0 {
					return &shardProduct{}, nil
				}
				return buildShardViaToolsets(tooltrpc.ToolsetConfig{
					CustomTools:     tools,
					SkipPostProcess: true,
				}, lg)(ctx)
			},
		})
	}

	// custom：调用方注入工具，身份每构建不同，不缓存（构建闭包仅包装引用，
	// 零成本；释放不落 Close——flat 工具无 Close 且所有权在调用方）。
	if len(deps.CustomTools) > 0 {
		customTools := deps.CustomTools
		p.specs = append(p.specs, shardSpec{
			id:        shardGroupCustom,
			group:     shardGroupCustom,
			fp:        shardFingerprint(shardGroupCustom, customToolNames(customTools)),
			cacheable: false,
			build: func(ctx context.Context) (*shardProduct, error) {
				return &shardProduct{tools: customTools}, nil
			},
		})
	}

	if len(p.specs) == 0 {
		return nil, nil
	}
	return p, nil
}

// buildShardViaToolsets 生成分片构建闭包：以受限 ToolsetConfig 走完整
// BuildToolsets 装配（SkipPostProcess/SkipMCPGovernance 由调用方预设），
// 产物收敛为原始 shardProduct（未过治理/去重/消歧/别名/延迟包装/门/装饰器）。
func buildShardViaToolsets(cfg tooltrpc.ToolsetConfig, lg loggateway.Logger) func(ctx context.Context) (*shardProduct, error) {
	return func(ctx context.Context) (*shardProduct, error) {
		ts, err := tooltrpc.BuildToolsets(ctx, cfg, lg)
		if err != nil {
			return nil, err
		}
		if ts == nil {
			return &shardProduct{}, nil
		}
		return &shardProduct{toolSets: ts.ToolSets, tools: ts.Tools}, nil
	}
}

// coreShardActive 报告 core 受限 cfg 是否有任何可构建内容。
// 口径与 tooltrpc.ToolsetConfigHasAny 对齐，仅剔除已抽出的组字段
// （knowledge/memory/working_memory/mcp/broker/custom）。
func coreShardActive(cfg tooltrpc.ToolsetConfig) bool {
	return cfg.Filesystem || cfg.ShellExec || cfg.WebFetch || cfg.WebSearch || cfg.WebResearch ||
		cfg.GeminiFetch || cfg.GoogleSearch || cfg.ArxivSearch || cfg.Wikipedia ||
		cfg.Email || cfg.Todo || cfg.AwaitReply || cfg.ClaudeCode || cfg.WorkspaceExec ||
		cfg.CallAgent || cfg.Kanban ||
		cfg.ReadDocument || cfg.ReadSpreadsheet || cfg.Datetime || cfg.Message || cfg.BrowserEnabled || cfg.SubAgent ||
		cfg.ClientBridge || cfg.ComputerUse || cfg.CodingBridge ||
		len(cfg.AgentTools) > 0 || len(cfg.OpenAPISpecs) > 0
}

// officecliEnabledKeys 返回 eff 中启用的 officecli 工具键（固定顺序，确定性）。
func officecliEnabledKeys(eff map[string]bool) []string {
	var out []string
	for _, k := range []string{officecli.ToolRead, officecli.ToolWrite, officecli.ToolRender} {
		if eff[k] {
			out = append(out, k)
		}
	}
	return out
}

// resolveDeferredToolNames 计算延迟工具清单（registry 名），与原
// buildToolsetsForAgent 的两段式分离逻辑一致：手动 ToolsDeferredJSON 优先，
// 否则按 profile 自动分离核心常驻/延迟加载集。结果为合并期
// FinalizeDeferredTools 的输入（分片构建自身不携带延迟处理）。
func resolveDeferredToolNames(ag biz.Agent, eff map[string]bool, lg loggateway.Logger) []string {
	if ag.Settings == nil {
		return nil
	}
	if ag.Settings.ToolsDeferredJSON != "" {
		// 手动配置优先：ToolsDeferredJSON 显式指定延迟工具列表（biz key 粒度），
		// 需转换为 registry 名称供装配层匹配 ToolSet/工具。
		var deferredKeys []string
		if err := json.Unmarshal([]byte(ag.Settings.ToolsDeferredJSON), &deferredKeys); err == nil {
			return deferred.RegistryNamesForBizKeys(deferredKeys)
		}
		return nil
	}
	// 自动两段式分离（WP-4）：基于 profile 把有效工具分为核心常驻集和延迟加载集。
	profile := strings.TrimSpace(ag.Settings.ToolsProfile)
	effKeys := effKeysList(eff)
	_, deferredKeys := deferred.SplitCoreResidentTools(effKeys, profile)
	out := deferred.RegistryNamesForBizKeys(deferredKeys)
	if len(out) > 0 {
		lg.Info("两段式工具加载：自动分离核心/延迟",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("profile", profile),
			loggateway.Int("total_tools", len(effKeys)),
			loggateway.Int("deferred_tools", len(out)))
	}
	return out
}

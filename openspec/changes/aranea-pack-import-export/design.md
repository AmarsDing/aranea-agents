## Context

Aranea-Agents 当前的种子/导入机制存在严重分裂：

1. **内置种子**：`internal/data/seed_*.go` 通过 RawSQL 硬编码 80+ 列写入数据库，schema 变更时极易遗漏
2. **CLI 导入**（orgimport）：通过 HTTP API 导入，但 AgentSpec 功能远少于内置种子（无 variant/tools/skills/subagents），TeamSpec 仅支持 member 列表（无 graph/critic_loop/intent_anchor）
3. **Graph 模板**：6 个内置模板 Go 硬编码在 `templates.go`，无法扩展
4. **两套类型体系**：`loader.Spec` 和 `orgimport.Spec` 概念重复但能力不对等，同一概念用不同字段表达（`position_key` vs `category_position`）
5. **无导出能力**：Agent 和 Team 没有完整的 JSON 导出 API，用户无法自助分享场景

实体依赖链：Taxonomy(industry→department→position) → Agent(引用 position) → AgentFiles + RuntimeSettings(1:1) → Graph(节点引用 Agent) → Team(成员引用 Agent.ID，可能引用 Graph.ID)

关键约束：Team 成员通过 `agent_id`（UUID）引用 Agent，导出时需转换为 `agent_key`；`taxonomy_position_id` 需转换为 taxonomy_key 路径（如 `finance/quant_trading/quant_researcher`）。

## Goals / Non-Goals

**Goals:**

- 定义统一的 `.arpack` 场景包格式，覆盖 Agent、Team、Taxonomy、Graph 四类实体
- 实现三种粒度的导出：单 Agent、单 Team（含成员 Agent）、整行业
- 实现导入引擎：校验→冲突策略→按依赖顺序四阶段写入
- 提供完整的 ID→Key 映射机制，确保跨实例可移植
- 将内置种子迁移到 .arpack 格式，消除 RawSQL 硬编码
- 提供 HTTP API 和 CLI 两种操作入口
- 为商城分享预留 manifest 元数据扩展空间

**Non-Goals:**

- 不做商城前端 UI
- 不做 Pack 数字签名和加密
- 不做增量 Pack 更新（首版仅全量导入）
- 不做跨 Pack 依赖解析
- 不做 Pack 版本自动升级
- 不做前端导入/导出 UI

## Decisions

### D1: Pack 物理格式 — tar.gz + YAML 目录结构

**选择**：`.arpack` = tar.gz 压缩包，内部为 YAML 文件 + Agent 文件目录

**替代方案**：
- (B) 纯 YAML + base64 内嵌文件：文件不可读不可编辑，被否决
- (C) ZIP 格式：Go 标准库 `archive/tar` 更轻量，无需引入额外依赖

**理由**：tar.gz 是 Go 生态最自然的打包格式，`archive/tar` + `compress/gzip` 为标准库；YAML 目录结构可读性好，Agent prompt 文件可独立编辑，Git diff 友好。

**Pack 目录结构**：
```
<name>.arpack (tar.gz)
├── manifest.yaml              # 包元数据
├── taxonomy.yaml              # 行业分类树（可选）
├── agents/
│   ├── <agent-key>.yaml       # Agent 完整定义
│   └── <agent-key>/           # Agent 文件目录
│       ├── IDENTITY.md
│       └── SOUL.md
├── teams/
│   └── <team-key>.yaml        # Team 定义
└── graphs/
    └── <graph-id>.yaml        # Graph 模板定义
```

### D2: 实体引用方式 — agent_key / taxonomy_key 路径

**选择**：Pack 内所有实体间引用使用业务 key 而非数据库 UUID

- Agent 引用 Taxonomy：`position_key: finance/quant_trading/quant_researcher`（路径格式）
- Team 引用 Agent：`members[].agent_key: go-senior-general`
- Team 引用 Graph：`graph.linked_id: pipeline`（Graph 使用 template ID）
- Graph 节点引用 Agent：`nodes[].agent_key: go-senior-architect`

**替代方案**：导出时保留 UUID，导入时做 UUID 映射表——但 UUID 在目标系统无意义，且不同 Pack 间 UUID 会冲突。

**理由**：业务 key 天然可读、可移植、跨实例一致。导入时统一做 key→ID 映射转换。

### D3: 导入写入路径 — ImporterRepo 直接写入

**选择**：导入通过 `ImporterRepo` 接口直接调用 data 层 Repo 写入，`ImporterRepo` 由 `PackRepoAdapter`（`internal/data/pack_repo.go`）组合适配

**替代方案**：通过 `biz.AgentUsecase.Create/Update` 等 Usecase 路径写入——但 Usecase 方法包含额外的业务校验和事件触发逻辑，Pack 导入引擎需要更细粒度的控制（如单独写入 Files、RuntimeSettings、按 key 查询等），直接使用 Repo 接口更灵活。

**理由**：ImporterRepo 接口提供了 Pack 导入所需的精确方法集（GetAgentByAgentKey、CreateAgent、UpdateAgent、ReplaceAgentPromptFiles、UpsertAgentRuntimeSettings 等），由 `PackRepoAdapter` 组合适配现有的 `biz.AgentRepository`、`biz.TeamRepository`、`biz.TaxonomyRepo`、`biz.GraphRepo`、`biz.SkillLookupReader` 五个依赖。幂等通过 `agent_key` 唯一约束 + `GetAgentByAgentKey` 查询实现。

**注意**：实际代码中 Pack 导入**不经过** biz 层 Usecase（AgentUsecase/TeamUsecase），而是通过 `ImporterRepo` → `PackRepoAdapter` → 底层 Repo 直接写入。这意味着 Usecase 层的业务校验和事件触发逻辑不会在 Pack 导入时执行。这是有意为之的设计——Pack 导入引擎需要更细粒度的控制（如单独写入 Files、RuntimeSettings、按 key 查询等），直接使用 Repo 接口更灵活。

### D4: Agent RuntimeSettings 导出粒度 — 可移植配置子集

**选择**：只导出"可移植配置"（约 30 个字段），排除实例绑定字段

**可移植字段**（导出）：
- Memory 域：enabled、L0-L4 各开关和窗口参数（含 L0SnapshotMode、L0InjectL1/L3/L4、L1BudgetTokens、L2EpisodeMinImportance、L2RecallMax/RetentionDays、L3RecallTopK/MinScore、L4GraphInjectNeighbors/MaxNeighbors/IdentityInject）
- Tools 域：retry_enabled、retry_max_attempts、retry_initial_interval_ms、streaming_enabled、circuit_breaker_enabled、command_safety_enabled、profile、allow/deny、parallel
- Skills 域：runtime_json（allowed_slugs/denied_slugs）、load_mode
- Evolution 域：self_evolve、skill_evolve、metrics_enabled、suggestions_enabled
- Reasoning 域：mode、level
- RalphLoop 域：max_iterations、completion_promise、verify_command、verify_timeout_seconds
- Context 域：compaction_enabled、session_summary_enabled、intent_pass_enabled

**实例绑定字段**（不导出）：
- Identity 域：channel_id、chat_id、workspace、variables_json
- CodeExecutor 域：code_executor_type（依赖部署环境）
- ModelInstructionsJSON（可能含实例特定指令）

**理由**：实例绑定字段在目标环境无意义，导入后应使用目标环境默认值。

### D5: 冲突策略 — 三选一

**选择**：支持 skip / overwrite / duplicate 三种策略

- **skip**：已存在则跳过，不修改
- **overwrite**：ON CONFLICT DO UPDATE（upsert），保留原 ID
- **duplicate**：生成新 key（如 `go-senior-general-copy`），创建新实体

**默认策略**：skip（最安全）

**理由**：不同场景需要不同策略——开发迭代用 overwrite，生产导入用 skip，试用场景用 duplicate。

### D6: 内置种子迁移 — embed .arpack + 统一引擎

**选择**：将现有 YAML 数据源转为 .arpack 目录结构，通过 `go:embed` 嵌入二进制，启动时用统一 Pack 导入引擎加载

**迁移映射**：
```
internal/scenario/taxonomy.yaml          → packs/builtin-templates/taxonomy.yaml
internal/scenario/agent_templates.yaml   → packs/builtin-templates/agents/*.yaml
internal/scenario/finance/agents.yaml    → packs/finance/ (拆分为独立 agent/team YAML)
internal/scenario/selfmedia/agents.yaml  → packs/selfmedia/
internal/scenario/softwaredev/agents.yaml→ packs/softwaredev/
internal/graph/trpc/templates.go         → packs/builtin-templates/graphs/*.yaml
```

**启动时序**：
1. P1 阶段：加载 `builtin-templates.arpack`（taxonomy + agent templates + graph templates）
2. Lazy 阶段：加载 `finance.arpack`、`selfmedia.arpack`、`softwaredev.arpack`
3. 版本门控：沿用 `schema_migrations` 表，版本号递增

**替代方案**：保留 RawSQL 但改为自动生成——增加构建复杂度，且仍绕过 biz 层。

### D7: Pack 包结构定义位置 — internal/biz/pack/

**选择**：在 `internal/biz/pack/` 下新建 Pack 引擎包

```
internal/biz/pack/
├── spec.go          # Pack YAML 类型定义（ManifestSpec, AgentSpec, TeamSpec, GraphSpec）
├── reader.go        # Pack 读取（解压 tar.gz → 解析 YAML → 构建内存模型）+ ReadPackFromFS（从 embed.FS 读取）
├── writer.go        # Pack 写出（内存模型 → YAML → 打包 tar.gz）
├── exporter.go      # 导出引擎（从 DB 读取 → ID→Key 转换 → 构建 Pack 模型）
├── importer.go      # 导入引擎（校验 → 冲突检测 → 四阶段写入 → Key→ID 映射）
├── validator.go     # 校验引擎（依赖检查、schema 校验、冲突预检）
├── mapper.go        # ID↔Key 映射工具（KeyMapper、BuildTaxonomyKey、ParseTaxonomyKeyPath）
└── convert.go       # 旧格式转换器（loader.Spec → Pack、GraphTemplate → GraphPackSpec、MergePacks）
```

**理由**：Pack 引擎是 biz 层业务逻辑，不依赖 trpc-agent-go 或 api proto，符合分层规范。

### D8: API 设计 — 新增 pack.proto（unary RPC + bytes 传输）

**选择**：新增 `api/kratos/pack/v1/pack.proto`，使用 unary RPC + bytes 字段传输 .arpack 数据

```protobuf
service PackService {
  rpc ExportPack(ExportPackRequest) returns (ExportPackResponse);    // unary，bytes 返回
  rpc ImportPack(ImportPackRequest) returns (ImportPackResponse);    // unary，bytes 上传
  rpc ValidatePack(ValidatePackRequest) returns (ValidatePackResponse); // unary，bytes 上传
}
```

- `ExportPackRequest`：`kind`（agent/team/industry）+ `ref`（ID 或 key）
- `ExportPackResponse`：`data`（bytes，.arpack tar.gz 内容）+ `name` + `kind`
- `ImportPackRequest`：`data`（bytes，.arpack tar.gz 内容）+ `conflict_strategy`
- `ImportPackResponse`：各实体创建/更新/跳过计数 + `conflict_strategy` + `failures[]`
- `ValidatePackRequest`：`data`（bytes）
- `ValidatePackResponse`：`valid` + `errors[]` + `warnings[]` + `missing_skills[]` + `missing_func_refs[]` + `conflicts[]`

**注意**：实际代码中 `ValidatePackResponse` proto 包含 `warnings` 字段（repeated string），但 `ValidationResult` Go 结构体当前未使用 `warnings` 字段（校验逻辑将所有问题放入 `errors`）。`ImportResult` Go 结构体包含 `warnings` 字段（Taxonomy 导入时的部分失败会记录为 warning），但 proto `ImportPackResponse` 未映射此字段。

HTTP 映射：
- `POST /v1/pack/export` → ExportPack（body: JSON）
- `POST /v1/pack/import` → ImportPack（body: JSON）
- `POST /v1/pack/validate` → ValidatePack（body: JSON）

**替代方案**：
- (A) 流式 RPC（stream ExportPackResponse / stream ImportPackRequest）——首版采用 unary + bytes 更简单，protobuf bytes 字段天然支持 base64 JSON 传输，避免流式 HTTP 的复杂度
- (B) 复用现有 Agent/Team/Graph API 逐个导入——无法保证事务性和依赖顺序

### D9: Pack 安全防护 — 解压炸弹与路径遍历防护

**选择**：在 `ReadPack` 中实现多层安全防护

- **大小限制**：`MaxPackSize`（200MB，原始文件）、`MaxTotalSize`（200MB，解压后总大小）、`MaxEntrySize`（10MB，单条目）、`MaxTarEntries`（1000，条目数上限）
- **路径遍历防护**：清洗路径后检查 `..` 和绝对路径，拒绝包含路径遍历的条目
- **符号链接跳过**：跳过 `TypeSymlink` 和 `TypeLink` 类型的 tar 条目
- **LimitReader**：使用 `io.LimitReader` 限制 gzip 解压大小，防止 gzip 炸弹

**理由**：Pack 文件可能来自不可信来源（用户上传），需要防止解压炸弹和路径遍历攻击。

### D10: 旧格式转换器 — convert.go

**选择**：在 `internal/biz/pack/convert.go` 中提供旧格式到 Pack 格式的转换函数，用于内置种子迁移的过渡期

- `ConvertIndustrySpecToPack`：将 `loader.IndustrySpec`（agents.yaml 格式）转换为 Pack
- `ConvertAgentTemplatesToPack`：将 `loader.AgentTemplatesSpec` 转换为 Pack
- `ConvertTaxonomySpecToPack`：将 `loader.TaxonomySpec` 转换为 TaxonomyPackSpec
- `ConvertGraphTemplates`：将 `GraphTemplateSource`（Go 硬编码模板）转换为 GraphPackSpec 列表
- `MergePacks`：合并多个 Pack（用于将 taxonomy + agent templates + graph templates 合并为 builtin-templates Pack）

**理由**：内置种子迁移是渐进式的，需要兼容现有 YAML 数据源。行业数据（finance/selfmedia/softwaredev）当前通过 `loader.LoadIndustrySpec` + `ConvertIndustrySpecToPack` 动态转换，无需预先创建 .arpack 目录。

### D11: ReadPackFromFS — 从 embed.FS 读取 Pack

**选择**：在 `reader.go` 中额外提供 `ReadPackFromFS(fsys fs.FS, root string)` 函数，从 `embed.FS` 读取 .arpack 目录结构

**理由**：内置模板 Pack 通过 `go:embed` 嵌入二进制，以目录形式存储（非 tar.gz），需要独立的读取路径。`SeedPackBuiltinTemplates` 使用 `ReadPackFromFS(builtinTemplatesFS, "scenario/packs/builtin-templates")` 加载内置模板。

### D12: Agent 类型扩展 — A2A Proxy 支持

**选择**：在 `AgentPackSpec` 中增加 `kind`（llm | a2a_proxy）和 `a2a_proxy` 字段，支持 A2A 代理类型的 Agent 导出/导入

- `A2AProxyPackSpec` 包含：`remote_url`、`agent_card_url`、`enable_streaming`、`auth_type`、`timeout_seconds`
- 导出时从 `biz.Agent.A2AProxy` 转换，导入时写入 `biz.Agent.A2AProxy`
- Agent 的 `Kind` 字段默认为 `"llm"`（导入时通过 `firstNonEmpty` 函数）

**理由**：系统已支持 A2A 代理类型的 Agent，Pack 格式需要完整覆盖。

### D13: Team 完整配置导出/导入

**选择**：Team 导出/导入覆盖完整的 OrchestrationSpec 配置，包括：

- **超时配置**：`run_timeout_sec`、`turn_timeout_sec`、`first_byte_timeout_sec`
- **运行时引擎**：`runtime_engine`（默认 "graph"）、`team_graph_runtime`
- **失败策略**：`failure_policy`（含 `default`、`retry`（含 max_attempts/initial_interval_ms/backoff_factor）、`node_overrides`（map[string]TeamNodeFailureOverride）、`circuit_breaker`（含 failure_threshold/recovery_timeout_ms）、`parallel_fail`、`on_error`）
- **Critic Loop**：`critic_loop`（含 `max_iterations`、`score_threshold`）
- **意图锚定与合成器**：`intent_anchor_key`、`synthesizer_key`（通过 agent_key 引用）
- **Graph 完整导出**：支持 `conditional_edges` 和 `subgraphs`
- **Team 成员扩展字段**：`enabled`（*bool）、`sort_order`（int）
- **Team Graph 扩展**：`layout`（string）、节点扩展字段 `interrupt_before`/`interrupt_after`/`destinations`/`retry_max_attempts`/`fallback_agent`、边扩展字段 `condition`

**理由**：Team 的完整配置远超最初设计文档中描述的 `max_concurrency`/`timeout_seconds`/`loop_max_iter`/`enable_checkpoint` 四个字段，需要完整覆盖以确保导出/导入的一致性。

### D14: 导入回滚机制

**选择**：Agent 导入时，若 Files 或 RuntimeSettings 写入失败，尝试通过 `DeleteAgent` 回滚已创建的 Agent 记录

**理由**：Pack 导入不使用数据库事务（每个实体独立写入），需要在失败时清理已创建的记录，避免残留不完整数据。回滚失败时在错误信息中同时报告原始错误和回滚错误。

### D15: Graph 节点扩展字段导出/导入

**选择**：Graph 节点（GraphNodePackSpec）导出/导入覆盖完整的 NodeDef 字段，包括：

- **基础字段**：`id`、`type`、`label`、`description`、`func_ref`、`instruction`、`model_name`、`tool_names`、`agent_key`
- **中断控制**：`interrupt_before`、`interrupt_after`
- **路由字段**：`destinations`
- **失败处理**：`retry_max_attempts`、`failure_action`、`fallback_agent`
- **数据映射**：`input_mapper_json`、`output_mapper_json`
- **消息隔离**：`isolated_messages`、`input_from_last_response`
- **缓存**：`cache_enabled`、`cache_ttl_seconds`

**理由**：Graph 节点的完整配置包含路由、失败处理、数据映射等高级功能，导出/导入需要完整覆盖以确保 Graph 模板的一致性。

### D16: overwrite 策略保留原始元数据

**选择**：Agent 和 Team 在 overwrite 策略下，保留原始记录的 `Status`、`Readonly`、`Source` 字段，而非用 Pack 中的默认值覆盖

**理由**：overwrite 策略的语义是"更新可修改字段，保留元数据"。如果用 Pack 中的默认值（如 `Source: "imported"`）覆盖原始值（如 `Source: "builtin"`），会导致已有 Agent 的来源信息丢失。同样，`Status` 和 `Readonly` 是运维属性，不应被导入操作改变。

### D17: duplicate 策略下原始 key 映射

**选择**：Agent 在 duplicate 策略下，除了注册新 key → 新 ID 映射外，同时注册原始 key → 新 ID 映射

**理由**：后续 Team/Graph 引用 Agent 时使用的是 Pack 中的原始 agent_key，如果 duplicate 策略只注册新 key 映射，Team 成员引用会解析失败。同时注册原始 key 映射确保引用链不断裂。

### D18: PackRepoAdapter 接受 SkillLookupReader 依赖

**选择**：`PackRepoAdapter` 构造函数接受 5 个参数：`biz.AgentRepository`、`biz.TeamRepository`、`biz.TaxonomyRepo`、`biz.GraphRepo`、`biz.SkillLookupReader`

**理由**：`ValidatorRepo.SkillExists` 方法需要通过 `SkillLookupReader` 查询 Skill 是否存在，`PackRepoAdapter` 需要组合此依赖。`FuncRefExists` 通过 `tools.Registry()` 全局函数查询，不需要额外依赖。

### D19: Graph 边扩展字段

**选择**：Graph 普通边（GraphEdgePackSpec）包含 `from`、`to`、`kind` 字段；Team 内嵌 Graph 边（TeamGraphEdgeSpec）包含 `id`、`source`、`target`、`label`、`condition` 字段

**理由**：两种边的使用场景不同——Graph 模板边需要 `kind` 字段区分边类型，Team 内嵌 Graph 边需要 `condition` 字段支持条件路由和 `label` 字段支持可视化标注。

### D20: seed_pack.go 使用独立 packImporterRepo

**选择**：`seed_pack.go` 中 `SeedPackBuiltinTemplates` 和 `SeedPackIndustry` 使用独立的 `packImporterRepo` 结构体（通过 `newPackImporter` 创建），而非 `PackRepoAdapter`

**理由**：种子加载发生在 DI 容器完全初始化之前（P1/Lazy 阶段），此时 `PackRepoAdapter` 所需的完整依赖（`biz.AgentRepository`、`biz.TeamRepository` 等）尚不可用。`packImporterRepo` 直接持有 `ent.Client`，通过 data 层的 Repo 实现来桥接。

**注意**：当前 `packImporterRepo` 的方法尚未实现（注释说"在 seed_pack_adapter.go 中实现"，但该文件不存在），导致 `SeedPackBuiltinTemplates`/`SeedPackIndustry` 若被调用则编译失败。由于 `data.go` 中仍调用旧种子函数，这些函数当前未被调用，编译不受影响。

### D21: CLI 命令参数名

**选择**：CLI 命令参数名与原始设计文档略有调整

- 导出命令使用 `--ref`（而非 `--id`），与 proto 的 `ref` 字段一致，因为 ref 可以是 ID 也可以是 key
- 导入命令使用 `--strategy`（而非 `--conflict-strategy`），更简洁

**实际命令**：
- `aranea pack export --kind agent --ref <id_or_key> -o output.arpack`
- `aranea pack import <file.arpack> --strategy overwrite`
- `aranea pack validate <file.arpack>`

### D22: CircuitBreakerPolicySpec.HalfOpenMaxCalls 未映射

**选择**：`CircuitBreakerPolicySpec` 包含 `HalfOpenMaxCalls` 字段，但导出/导入引擎未映射此字段

**理由**：导出时 `buildFailurePolicySpec` 从 `biz.TeamFailurePolicy.CircuitBreaker` 转换时只映射了 `FailureThreshold` 和 `RecoveryTimeoutMs`（由 `ResetTimeoutSeconds * 1000` 计算），未映射 `HalfOpenMaxCalls`。导入时同样未设置此字段。该字段当前为预留字段，后续如需支持可在导出/导入逻辑中补充。

### D23: Usecase 扩展方法已实现但 Pack 导入未使用

**选择**：`AgentUsecase.UpsertByKey`、`AgentUsecase.CreateWithFilesAndSettings`、`TeamUsecase.SaveTeamWithGraph` 已实现，但 Pack 导入引擎通过 `ImporterRepo` 直接写入，不调用这些 Usecase 方法

**理由**：如 D3 所述，Pack 导入需要更细粒度的控制（如单独写入 Files、RuntimeSettings、按 key 查询等），直接使用 Repo 接口更灵活。这些 Usecase 方法为其他场景（如未来前端 API）预留，当前 Pack 导入路径不经过它们。

### D24: ValidatePackResponse.warnings 未映射

**选择**：proto `ValidatePackResponse` 定义了 `warnings` 字段（`repeated string`），但 `ValidationResult` Go 结构体没有 `Warnings` 字段，service 层也未映射此字段

**理由**：当前校验逻辑将所有问题放入 `errors` 列表，未区分阻断性错误和非阻断性警告。后续如需区分，需在 `ValidationResult` 中添加 `Warnings` 字段并在 service 层映射。

### D25: ImportResult.Warnings 未映射到 proto

**选择**：Go 结构体 `ImportResult` 包含 `Warnings` 字段（Taxonomy 导入部分失败时记录），但 proto `ImportPackResponse` 没有 `warnings` 字段，service 层未映射

**理由**：Taxonomy 导入时部门/岗位写入失败会记录为 warning 而非 error，继续导入后续节点。但这些警告信息未传递到 API 层，客户端无法看到。后续可在 proto 中添加 `warnings` 字段并映射。

### D26: buildPositionKeyPath 只返回 position 级别 key

**选择**：`convert.go` 中 `buildPositionKeyPath` 函数只返回 position 级别的 key（如 `quant_researcher`），而非完整的 taxonomy_key 路径（如 `finance/quant_trading/quant_researcher`）

**理由**：`agents.yaml` 中的 `position_key` 只是 position 级别的 key，无法确定完整的 industry/dept/pos 路径。导入时 Taxonomy 已在 P1 阶段加载，Importer 会通过 mapper 查找。但这导致 `ConvertIndustrySpecToPack` 生成的 Pack 中 `position_key` 不是完整路径格式，与 D2 的设计有偏差——`ResolvePositionKey` 需要完整路径才能在 mapper 中找到对应 ID。

**影响**：通过 `ConvertIndustrySpecToPack` 转换的行业 Pack 导入时，Agent 的 `taxonomy_position_id` 可能无法正确解析（因为 mapper 中注册的是完整路径格式的 key）。通过 `Exporter.ExportAgent/ExportIndustry` 导出的 Pack 不受影响（使用 `BuildTaxonomyKey` 构建完整路径）。

### D27: Validate 函数接受 nil repo 参数

**选择**：`Validate(ctx, p, repo)` 函数允许 `repo` 参数为 nil，此时跳过依赖校验和冲突预检，仅执行格式校验和引用完整性检查

**理由**：支持离线校验场景（如 CLI 在无数据库连接时校验 Pack 文件格式）。当 `repo == nil` 时，`validateDependencies` 和 `validateConflicts` 被跳过，只执行 `validateManifest`、`validateAgentSpecs`、`validateTeamSpecs`、`validateGraphSpecs` 和 `validateReferences`。

### D28: collectDependencies 不收集 Team 内嵌 Graph 的 FuncRef

**选择**：`exporter.go` 中 `collectDependencies` 函数只从 `p.Graphs`（独立 Graph 模板）收集 FuncRef，不从 `p.Teams[].Graph`（Team 内嵌 Graph）收集

**理由**：当前实现中 Team 内嵌 Graph 的节点不包含 `func_ref` 字段（`TeamGraphNodeSpec` 没有 `FuncRef` 字段），因此无需收集。但如果未来 Team 内嵌 Graph 节点支持 `func_ref`，需要在 `collectDependencies` 中补充收集逻辑。

**注意**：`convert.go` 中的 `collectPackDependencies` 同样只收集 Skill 依赖，不收集 FuncRef（因为 `ConvertIndustrySpecToPack` 路径生成的 Pack 不包含独立 Graph 模板）。

### D29: finance Pack manifest 已完整但 YAML 文件不完整

**选择**：`internal/scenario/packs/finance/manifest.yaml` 已列出完整的 37 个 agent 和 8 个 team 引用，但 `agents/` 目录下仅创建了 `technical-analyst-general.yaml` 一个文件

**理由**：manifest 是按照完整金融行业场景设计的，但 agent/team YAML 文件的拆分工作尚未完成。当前 `SeedPackIndustry` 使用 `loader.LoadIndustrySpec` + `ConvertIndustrySpecToPack` 动态转换路径加载金融行业数据，不依赖 `ReadPackFromFS` 读取 finance 目录。如果通过 `ReadPackFromFS` 读取 finance 目录，只能获取到 1 个 Agent 的数据，与 manifest 声明不一致。

## Risks / Trade-offs

**[R1] 内置种子迁移风险**：将 RawSQL 替换为 Pack 引擎路径，启动性能可能下降 → 缓解：当前行业数据仍通过 `loader.LoadIndustrySpec` + `ConvertIndustrySpecToPack` 动态转换后导入，性能与旧路径基本一致；内置模板已转为 .arpack 目录格式通过 `ReadPackFromFS` 加载

**[R2] orgimport 废弃后的兼容性**：现有 CLI 用户可能依赖 orgimport → 缓解：`internal/orgimport/` 包仍存在，尚未标记 deprecated；提供迁移指南后可在后续版本中删除

**[R3] Pack 格式版本演进**：api_version: v1 后续可能需要 v2 → 缓解：manifest 中预留 api_version 字段，读取时按版本分发解析逻辑

**[R4] 大型行业 Pack 的导入耗时**：金融行业 37 个 Agent + 8 个 Team，ORM 逐个写入可能较慢 → 缓解：当前行业数据通过 `ConvertIndustrySpecToPack` 动态转换后逐个写入，后续可优化为批量写入

**[R5] Skill 依赖校验的松耦合性**：Skill 通过 slug 引用，目标系统可能缺少对应 Skill → 缓解：validate 阶段报告缺失 Skill 列表。**注意**：当前代码实现中 Skill 缺失会标记为 `valid=false`（阻断项），与 `pack-import/spec.md` 中"但不阻断导入"的描述不一致。代码行为（`validator.go` 第 118-119 行 `result.Valid = false`）是实际生效的逻辑，如需改为非阻断警告，需修改代码并添加 `Warnings` 字段到 `ValidationResult`

**[R6] Graph FuncRef 不可移植**：Graph 节点的 `func_ref` 引用 Go 注册函数，不同版本可能不同 → 缓解：manifest 中声明 `dependencies.func_refs`，validate 阶段通过 `tools.Registry()` 校验注册表可用性

**[R7] seed_pack.go 编译问题**：`SeedPackBuiltinV1`、`SeedPackFinanceV1`、`SeedPackSelfmediaV1`、`SeedPackSoftwaredevV1`、`SeedPackIndustryBase` 常量在 `seed_pack.go` 中引用但尚未在 `seed_versions.go` 中定义；同时 `packImporterRepo` 结构体的 `ImporterRepo` 接口方法尚未实现（注释说"在 seed_pack_adapter.go 中实现"，但该文件不存在）。当前 `seed_pack.go` 不会被 `data.go` 调用（仍使用旧种子函数），因此编译不受影响，但若调用则编译会失败 → 缓解：需在完成种子迁移前添加版本常量到 `seed_versions.go` 并实现 `packImporterRepo` 的接口方法

**[R8] MergePacks 函数 bug**：`convert.go` 中 `MergePacks` 函数第 462 行 `result.Teams = append(result.Teams, p.Graphs...)` 应为 `result.Teams = append(result.Teams, p.Teams...)`，当前会导致 Teams 数据丢失 → 缓解：需在后续修复

**[R9] ConvertIndustrySpecToPack 的 position_key 不完整**：`convert.go` 中 `buildPositionKeyPath` 函数只返回 position 级别的 key（如 `quant_researcher`），而非完整路径格式（如 `finance/quant_trading/quant_researcher`）。这导致通过 `ConvertIndustrySpecToPack` 转换的行业 Pack 导入时，Agent 的 `taxonomy_position_id` 无法正确解析——因为 `KeyMapper` 中注册的是完整路径格式的 key，而 Pack 中的 `position_key` 只是 position 级别的 key → 缓解：需修改 `buildPositionKeyPath` 构建完整路径，或在 `importAgent` 中增加 position key 的模糊匹配逻辑

## Migration Plan

1. **Phase 1 — Pack 引擎 + 格式** ✅：实现 `internal/biz/pack/` 包，支持读写 .arpack 格式（spec.go、reader.go、writer.go、mapper.go、convert.go、pack_test.go）
2. **Phase 2 — 导出引擎** ✅：实现三种粒度导出，新增 API 端点（exporter.go、service/pack.go、pack.proto、cli/cmd/pack.go）
3. **Phase 3 — 导入引擎** ✅：实现四阶段导入 + 冲突策略，新增 API 端点（importer.go、validator.go、data/pack_repo.go）
4. **Phase 4 — 内置种子迁移** 🔄：builtin-templates Pack 已创建（含 7 个 agent templates + 6 个 graph templates），finance Pack 已创建（含 1 个 agent），seed_pack.go 已创建 `SeedPackBuiltinTemplates` 和 `SeedPackIndustry` 函数；但 selfmedia/softwaredev Pack 目录未创建（当前通过 `ConvertIndustrySpecToPack` 动态转换）、旧种子函数仍在 `data.go` 中被调用（`SeedPackBuiltinTemplates`/`SeedPackIndustry` 尚未被调用）、版本常量未定义（`SeedPackBuiltinV1` 等未添加到 `seed_versions.go`，会导致编译失败）、`packImporterRepo` 接口方法未实现（`seed_pack_adapter.go` 不存在，会导致编译失败）、templates.go 未改为 Pack 加载（`ListBuiltinTemplates` 仍从硬编码读取）
5. **Phase 5 — 清理** ❌：RawSQL 种子文件、orgimport 包、Go 硬编码模板均未删除

**回滚策略**：每个 Phase 独立可回滚。Phase 4 迁移期间保留 RawSQL 代码但注释掉，确认 Pack 加载正常后再删除。

## Open Questions

- Agent RuntimeSettings 的"可移植配置"字段列表是否需要更细粒度的控制（如允许用户选择导出哪些域）？
- 整行业导出时，是否需要包含该行业下的所有 Graph（当前 Graph 没有行业关联字段）？
- Pack 导入时是否需要支持"预览模式"（只显示将要创建的实体列表，不实际写入）？

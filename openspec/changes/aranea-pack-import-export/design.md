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

### D3: 导入写入路径 — ORM Usecase 路径

**选择**：导入通过 `biz.AgentUsecase.Create/Update` + `biz.TeamUsecase.Create/Update` 等 ORM 路径写入，不再使用 RawSQL

**替代方案**：继续用 RawSQL 批量写入——但 80+ 列硬编码已证明脆弱，且绕过 biz 层校验。

**理由**：ORM 路径天然处理 schema 变更、字段校验、事件触发；幂等通过 `agent_key` 唯一约束 + `GetByAgentKey` 查询实现。

### D4: Agent RuntimeSettings 导出粒度 — 可移植配置子集

**选择**：只导出"可移植配置"（约 30 个字段），排除实例绑定字段

**可移植字段**（导出）：
- Memory 域：enabled、L0-L4 各开关和窗口参数
- Tools 域：enabled、profile、allow/deny、parallel、retry
- Skills 域：runtime_json（slug 列表）、load_mode、intent_pass_enabled
- Evolution 域：self_evolve、skill_evolve
- Reasoning 域：mode、level
- RalphLoop 域：max_iterations 等
- Context 域：compaction_enabled、session_summary_enabled

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
├── reader.go        # Pack 读取（解压 tar.gz → 解析 YAML → 构建内存模型）
├── writer.go        # Pack 写出（内存模型 → YAML → 打包 tar.gz）
├── exporter.go      # 导出引擎（从 DB 读取 → ID→Key 转换 → 构建 Pack 模型）
├── importer.go      # 导入引擎（校验 → 冲突检测 → 四阶段写入 → Key→ID 映射）
├── validator.go     # 校验引擎（依赖检查、schema 校验、冲突预检）
└── mapper.go        # ID↔Key 映射工具
```

**理由**：Pack 引擎是 biz 层业务逻辑，不依赖 trpc-agent-go 或 api proto，符合分层规范。

### D8: API 设计 — 新增 pack.proto

**选择**：新增 `api/kratos/pack/v1/pack.proto`

```protobuf
service PackService {
  rpc ExportPack(ExportPackRequest) returns (stream ExportPackResponse);  // 流式下载
  rpc ImportPack(stream ImportPackRequest) returns (ImportPackResponse);  // 流式上传
  rpc ValidatePack(stream ValidatePackRequest) returns (ValidatePackResponse);
}
```

HTTP 映射：
- `POST /v1/packs/export` → ExportPack
- `POST /v1/packs/import` → ImportPack
- `POST /v1/packs/validate` → ValidatePack

**替代方案**：复用现有 Agent/Team/Graph API 逐个导入——无法保证事务性和依赖顺序。

## Risks / Trade-offs

**[R1] 内置种子迁移风险**：将 RawSQL 替换为 ORM 路径，启动性能可能下降 → 缓解：批量 upsert 仍使用 `ON CONFLICT DO UPDATE`，实测对比后如有性能问题可在 data 层增加批量写入优化

**[R2] orgimport 废弃后的兼容性**：现有 CLI 用户可能依赖 orgimport → 缓解：提供迁移指南，orgimport 的 YAML spec 可通过转换工具转为 .arpack 格式

**[R3] Pack 格式版本演进**：api_version: v1 后续可能需要 v2 → 缓解：manifest 中预留 api_version 字段，读取时按版本分发解析逻辑

**[R4] 大型行业 Pack 的导入耗时**：金融行业 37 个 Agent + 8 个 Team，ORM 逐个写入可能较慢 → 缓解：导入使用事务批量提交，每阶段一个事务

**[R5] Skill 依赖校验的松耦合性**：Skill 通过 slug 引用，目标系统可能缺少对应 Skill → 缓解：validate 阶段报告缺失 Skill 列表，但不阻断导入（降级为无 Skill 模式）

**[R6] Graph FuncRef 不可移植**：Graph 节点的 `func_ref` 引用 Go 注册函数，不同版本可能不同 → 缓解：manifest 中声明 `dependencies.func_refs`，validate 阶段校验注册表可用性

## Migration Plan

1. **Phase 1 — Pack 引擎 + 格式**：实现 `internal/biz/pack/` 包，支持读写 .arpack 格式
2. **Phase 2 — 导出引擎**：实现三种粒度导出，新增 API 端点
3. **Phase 3 — 导入引擎**：实现四阶段导入 + 冲突策略，新增 API 端点
4. **Phase 4 — 内置种子迁移**：将 YAML 数据源转为 .arpack，修改启动编排
5. **Phase 5 — 清理**：删除 RawSQL 种子文件、orgimport 包、Go 硬编码模板

**回滚策略**：每个 Phase 独立可回滚。Phase 4 迁移期间保留 RawSQL 代码但注释掉，确认 Pack 加载正常后再删除。

## Open Questions

- Agent RuntimeSettings 的"可移植配置"字段列表是否需要更细粒度的控制（如允许用户选择导出哪些域）？
- 整行业导出时，是否需要包含该行业下的所有 Graph（当前 Graph 没有行业关联字段）？
- Pack 导入时是否需要支持"预览模式"（只显示将要创建的实体列表，不实际写入）？

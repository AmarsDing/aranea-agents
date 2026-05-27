# M58 — Prompt 治理与组织自动化（Prompt Governance & Org Automation, PGO）

> **版本**：2026-05-27 · **状态**：📋 需求草案 · **优先级**：P0–P2
> **依赖**：M4 Agent Type（分类树）· M5 Agent Setting · M6 Agent Setting File · M25 CLI · M33 Evaluation（refine 后回归测试可选）
> **影响范围**：`internal/biz/{agent,agent_category,agent_settings_helpers}` · `internal/service/{agent,prompt_refine}` · `internal/agent/{prompt,prompt_mode,trpc_build}` · `api/kratos/{agent,agent_category,system_setting}/v1` · `web/src/{components/agents,features/agents,pages}` · 新增 `cmd/aranea/import/`、`internal/import/`
> **红线**：
> - `internal/biz` 不 import `pkg/trpc-agent-go`
> - Runner 装配只在 `internal/service`
> - 不动 `agent_category_nodes` / `agents` 的 ent schema 主键与外键
> - 字段重命名只在 UI/文案层完成，DB 列名不变

---

## 1. 模块定位

PGO 是对**「Agent 怎么定义、怎么填写、怎么批量生成」**的端到端治理。它由 4 个相互衔接的子主题组成：

| 子主题 | 业务问题 | 期望价值 | 优先级 |
|--------|----------|----------|--------|
| **PGO-1 文件裁减重组** | 默认 9 个 prompt 文件中 4 个是占位且无代码依赖，造成"看着很多其实没用"的认知负担；SOUL/IDENTITY/USER 三层语义重叠 | 默认精简到 5 个核心 + 1 个可选；语义边界清晰；token 基线下降 | P0 |
| **PGO-2 分层填写指南** | 行业 / 部门 / 岗位 / Agent 各层 description 没有指南，用户不知道写什么、写多少、写到哪一层；常把岗位职责复制到 IDENTITY.md，造成 prompt 重复 | 每个字段配套"指南卡 + 占位文本 + 样例 + 字符预算"；同一份 schema 同时驱动 UI 提示和 AI 优化 prompt | P0 |
| **PGO-3 AI 优化按钮（统一服务）** | 现有 `EditPromptFileByAI` 只覆盖 Agent 文件；行业/部门/岗位 description 与 agent_description 没有 AI 优化能力；调用入口分散 | 一个统一 `/v1/ai/refine` 服务覆盖全部 5 个 scope；前端一个 `AIRefineButton` 组件复用；为 PGO-4 提供底层能力 | P1 |
| **PGO-4 CLI 全自动建模** | 新接入一个企业 / 一个行业，要手工建 行业 → 部门 → 岗位 → N 个 Agent + Team，效率低且容易遗漏；seed-stockx-org 是一次性硬编码 | 用 `aranea import <spec.yaml \| spec.md>` 一键导入；Markdown 输入走 LLM 抽取 schema；dry-run / refine / 幂等更新 | P1 |

**核心定位**：PGO 把 Aranea 从"手工捏 Agent"升级为"**结构化定义 + AI 辅助 + 批量自动化**"的 Agent 平台。PGO-2 是基础（schema），PGO-3 在其上做单点优化（refine），PGO-4 在其上做批量自动化（import）。PGO-1 是清理性前置工作。

---

## 2. 现状评估

### 2.1 主链路现状（2026-05-27）

```mermaid
flowchart LR
  subgraph Now["现状"]
    direction TB
    Form[行业/部门/岗位<br/>description 表单] -->|仅展示/筛选| UI[Web UI]
    Form -->|不进 Prompt| Skip[(× 未注入)]
    AgentDesc[agent_description] --> Build[BuildSystemPrompt]
    Files9[9 文件 stub<br/>含 4 个 dead] --> Build
    Build --> Inst[LLMAgent Instruction]
    UserMD[用户的描述 markdown] -.->|无路径| Nothing[(× 无导入工具)]
    AIEdit[/v1/agents/{id}/files/{fid}/ai-edit] --> Refiner1[PromptFileAIEditor]
    Refiner1 -.->|只覆盖 Agent 文件| Limited[(× 不覆盖分类与 description)]
  end
```

### 2.2 9 个默认 prompt 文件代码引用矩阵

| 文件 | 代码具名引用 | mode 白名单<br/>(C/T/M) | seed 真实长度 | 默认 stub? | 评估 |
|------|------------|------------------------|--------------|-----------|------|
| AGENTS_CORE.md | evolution writeback 首选 | C/T/M | 350–450 | 是 | ✅ 核心 |
| AGENTS_TASK.md | evolution writeback | C/T | 450–700 | 是 | ✅ 核心 |
| IDENTITY.md | `HasFilteredPromptFile` 压制 `DisplayName` | C/T | 250–350 | 是 | ✅ 核心 |
| CAPABILITIES.md | `HasFilteredPromptFile` 压制 RuntimeCue 工具枚举 | C/T | 350–600 | 是 | ✅ 核心 |
| RULE.md | 无具名引用 | C/T/M | 350–450 | 是 | ✅ 核心 |
| SOUL.md | evolution persona writeback | C | 140–180 | 是 | 🟡 合并入 IDENTITY 的 `## Persona` 段 |
| USER.md | 无 | C | 25（占位） | 是 | 🔻 默认不创建，转可选模板 |
| USER_PREDEFINED.md | 无 | C | 75 | 是 | 🔻 删除（与 USER.md 重复） |
| HEARTBEAT.md | 前端硬编码 + Settings 卡片；**后端无 injector** | C/T | 35（占位） | 是 | 🟡 脱离 prompt 域，由 Settings 心跳卡片承载 |

**关键观察**：
- 4 个文件（SOUL/USER/USER_PREDEFINED/HEARTBEAT）当前都是 stub 且无代码依赖，但占用 mode 白名单槽位与用户认知带宽。
- `task` mode 白名单含 HEARTBEAT.md，与"心跳应是运行时事件不是静态 prompt"的语义不符；文档与代码不一致（详见 `docs/guides/prompt/assembly.md`）。
- `minimized` mode 文档说含 IDENTITY，代码不含——以本期统一为准（代码事实）。

### 2.3 行业分类 description 未注入路径（事实）

- `agent_category_nodes.description` 在 `internal/data/agent_repo.go`、`internal/service/agent_category.go`、`web/src/pages/AgentCategoriesPage.vue` 仅用于 UI 展示 / 列表筛选 / Team 分组。
- `internal/agent/{trpc_build.go,prompt.go,prompt_mode.go}` **不读取** `AgentCategory`。
- seed 数据已经把同一段 description 既写到 `agent_description` 又写到 `IDENTITY.md`（`cmd/seed-stockx-org/prompts.go:18-27`），重复但无人为去重。

### 2.4 LLM 调用现有资产

| 入口 | 用途 | 路径 | PGO 复用方式 |
|------|------|------|-------------|
| `CallOpenAICompatChat` | 单次 system+user → completion | `internal/agent/openai_compat.go:44` | PGO-3 refiner 底层（已被 L0 压缩 / Intent / 记忆抽取使用） |
| `PromptFileAIEditor.Revise` | 当前唯一的 prompt 文件 AI 编辑 | `internal/service/agent_prompt_ai.go:46–117` | PGO-3 泛化的起点 |
| `provider.TRPCModelForProviderModel` | 由 provider/model 构造 trpc Model | `internal/provider/trpc_llm.go:27` | PGO-3 选模型 |
| `SystemSetting.EvalLLM` / `KnowledgeEmbed` / `WebResearch` / `MemoryPlatform` | 平台级 LLM 配置（按场景） | proto + biz | PGO-3 新增 `DefaultRefineLLM` 字段 |

### 2.5 CLI 现状

- `docs/需求/25 cli.md` + `docs/需求/25-cli-implementation-plan-2026-05-27.md` 已规划 `cmd/aranea/` 二进制，**未实施**。
- 现有 `cmd/seed-stockx-org/` 是"批量创建组织 + Agent + Team"的硬编码版本，是 PGO-4 的天然参考。
- 缺失能力：YAML/Markdown 导入；dry-run；refine 集成；幂等更新；审计追溯。

### 2.6 用户故事级痛点

| 角色 | 故事 | 痛点 |
|------|------|------|
| 平台运营 | "新接入一个医疗客户，需要 1 个行业 + 3 个部门 + 8 个岗位 + 12 个 Agent" | 全手工，半天起步；新人不知道 description 该写多长 |
| Agent 创建者 | "我写完描述发现 AI 表现差" | 不知道哪个字段没写好；没人告诉我"岗位职责"应该在分类层而不是 IDENTITY |
| Agent 创建者 | "我点 AI 优化按钮想优化整个 Agent" | 只能优化单个文件；分类描述和 agent_description 没按钮 |
| 平台运营 | "客户给我一份 markdown 描述需求" | 没有工具能直接吃进去；只能人肉对照建表单 |
| Token 预算敏感者 | "我的 system prompt 4k 起" | 默认 9 个文件大多是 stub 也占位；不知道哪个能删 |

### 2.7 已有可复用资产

| 资产 | 路径 | 复用方向 |
|------|------|---------|
| `AgentCategoryUsecase.Create` | `internal/biz/agent_category.go:124` | PGO-4 import 复用 |
| `AgentUsecase.Create / CreatePromptFile` | `internal/biz/agent_usecase.go:206, 399` | PGO-4 import 复用 |
| `TeamUsecase.Create` | `internal/biz/team_usecase.go:178` | PGO-4 import 复用 |
| `seed-stockx-org` 的 buildPlan / processAgents / teams_seed | `cmd/seed-stockx-org/` | PGO-4 重构为 `internal/import/` 后被 import 与 seed 同时复用 |
| `FilesForMode` / `HasFilteredPromptFile` | `internal/biz/agent_settings_helpers.go:424` / `internal/agent/prompt_mode.go:39` | PGO-1 增加 mode 后保持函数签名 |
| `IdentityDescriptionForAgent` 去重模式 | `internal/agent/prompt_mode.go:49-55` | PGO-1 合并 SOUL → IDENTITY 时同模式 |

---

## 3. 目标与非目标

### 3.1 目标（本期）

1. **PGO-1**：默认 prompt 文件从 9 个精简到 5 个核心 + 1 个可选；SOUL 合并入 IDENTITY；USER_PREDEFINED 删除；USER 转可选；HEARTBEAT 脱离 prompt 域。Token 基线下降 ≥ 15%（粗估，按现 stub 占位行计）。
2. **PGO-1 字段重命名**：行业分类按 level 切换 label —— `行业说明 / 部门职责 / 岗位职责`；level=3 的 description 通过新通道注入 Prompt（具体注入策略详见 PGO 上轮方案的 "L1 岗位职责"）。**DB 列名保持 `description`**。
3. **PGO-2**：建立 `FieldGuide` schema，覆盖 5 个 scope（category.industry/department/position + agent.description + agent.file），同一份 schema 同时驱动：
   - 前端"指南卡 + 占位 + 样例 + 字符预算"四件套
   - PGO-3 / PGO-4 的 LLM system prompt
4. **PGO-3**：新增 `POST /v1/ai/refine`，统一覆盖 5 个 scope 的 AI 优化按钮；旧 `EditPromptFileByAI` 内部转发到新服务，保留一个版本周期。
5. **PGO-4**：交付 `aranea import <file>`：
   - 支持 YAML（严格 schema）与 Markdown（LLM 抽取）
   - 支持 `--dry-run` / `--refine` / `--update` / `--confirm` / `--output-spec`
   - 复用 `internal/import/` 公共包；`seed-stockx-org` 重构为该包的使用者
   - 完整审计（`source=cli_import, correlation_id=xxx`）

### 3.2 非目标

- **不**新建 `Organization` 实体（继续用 `agent_category_nodes` 树承载组织语义）。
- **不**修改 `agent_category_nodes` 与 `agents` 的 ent schema 主键/外键（只增字段或纯 service 层改动）。
- **不**为 `HEARTBEAT.md` 实现后端真正的周期 injector（标记为 PGO-1 后续，不在本期）。
- **不**实现自动回滚 import（v2 再做；v1 仅审计可追溯）。
- **不**为 SOUL.md 实现自动演化 Scanner（沿用 M7 evolution 现状，只调整 writeback 目标）。
- **不**做"反向 export"（`aranea export` 标记为 PGO-4 后续 stage）。
- **不**做 CLI 远端鉴权增强（沿用 M25 现有 PAT 模型）。

### 3.3 关键决策

| 决策项 | 选择 | 依据 |
|--------|------|------|
| DB 列名 | 保持 `description` 不动 | 减少 migration 与 proto breaking change；按 level 在 UI 切换 label |
| 默认文件保留集合 | AGENTS_CORE / AGENTS_TASK / IDENTITY / CAPABILITIES / RULE | 每个都有代码具名引用或 mode 强语义 |
| 可选文件 | USER_CONTEXT.md | 由 USER + USER_PREDEFINED 合并；用户主动添加 |
| SOUL 去向 | 合并入 IDENTITY 的 `## Persona` 段 | persona 与 identity 语义相邻；evolution writeback 改 anchor 替换 |
| Refine 服务边界 | 单一 `POST /v1/ai/refine` + scope 路由 | 避免 5 个 endpoint 维护成本；和现有 `evolution_pass.go` / `intent/pass.go` 同套裸 LLM 范式 |
| Refine 模型选择 | Agent scope → Agent model → SystemDefault → catalog[0]<br/>Category scope → SystemDefault → catalog[0] | 兼容现网；可被 SystemSetting 兜底覆盖 |
| CLI Spec 格式 | YAML 严格 + Markdown 宽松（LLM 抽取） | YAML 给 CI / 脚本；Markdown 给非技术人员 |
| CLI 幂等 | 通过 `key` 字段；存在 skip 默认，`--update` 覆盖 | 与 seed-stockx-org 现状一致 |
| 字符预算 | 行业 ≤ 300 / 部门 ≤ 400 / 岗位 ≤ 1000 / agent_desc ≤ 500 / 文件 ≤ 5000 | 实测 + 经验，软上限可调 |

### 3.4 与其他模块的边界

- **M4 Agent Type**：本期不改 schema；仅在 service / UI 层新增 `BuildCategoryResponsibility(positionID, mode)` 与 label 切换。
- **M5 / M6 Agent Setting + File**：默认文件清单 + Heartbeat 卡片归属变动，需同步文档；不动 ent schema。
- **M7 Agent Evolution**：persona writeback target 从 SOUL.md 改为 IDENTITY.md 的 `## Persona` 段；anchor 替换逻辑新增。
- **M25 CLI**：本期不交付完整 `aranea` 全部子命令，**只**实现 `aranea import` 子命令（独立子项目），可与 M25 主线并行；后续合入。
- **M33 Evaluation**：可选——为 refine 后的字段提供"前后对比评估"（不在本期必做）。
- **M37 Knowledge**：无关；本期不涉及 RAG。

---

## 4. 验收标准

### 4.1 业务层（用户可感）

| 编号 | 标准 |
|------|------|
| ACC-PGO-1-01 | 新建 Agent 时默认创建 5 个文件（AGENTS_CORE/AGENTS_TASK/IDENTITY/CAPABILITIES/RULE），不再有 SOUL/USER/USER_PREDEFINED/HEARTBEAT 默认 stub |
| ACC-PGO-1-02 | 设置页文件 Tab 提供"添加可选文件 → USER_CONTEXT.md" |
| ACC-PGO-1-03 | 行业分类管理页：level=1 显示"行业说明"，level=2 显示"部门职责"，level=3 显示"岗位职责" |
| ACC-PGO-1-04 | Agent 运行时 system instruction 顶部出现 `<role_responsibility source="category">…</role_responsibility>`（来自该 Agent 关联的职位 description）；可被 `agent.metadata_json.skip_category_responsibility=true` 关闭 |
| ACC-PGO-2-01 | 任一 description / 文件编辑器顶部出现可折叠"填写指南"卡片 |
| ACC-PGO-2-02 | 字数指示器实时显示 soft/hard 预算与当前字符数；超过 hard 时红色 |
| ACC-PGO-2-03 | "查看示例"按钮弹出至少 3 个跨行业样例 |
| ACC-PGO-3-01 | 5 个 scope（行业 / 部门 / 岗位 / agent_description / 任一 Prompt 文件）都有 AI 优化按钮且能产出建议 |
| ACC-PGO-3-02 | Refine modal 显示 diff，支持"应用 / 追加 / 取消"，并展示 token 节省量 |
| ACC-PGO-3-03 | 用户输入"附加要求"可重新生成（同一 modal 不关闭） |
| ACC-PGO-4-01 | `aranea import seed.yaml --dry-run` 输出完整 ASCII tree plan，不写入 DB |
| ACC-PGO-4-02 | `aranea import seed.yaml --confirm` 在空库上能创建 1 行业 + N 部门 + M 岗位 + K Agent + T Team，幂等执行第二次显示全部 skip |
| ACC-PGO-4-03 | `aranea import org.md --refine --output-spec org.yaml` 能从 markdown 抽取 YAML 落盘，并对所有用户字段调用 refine |
| ACC-PGO-4-04 | 所有 import 记录在审计日志 `source=cli_import` 并附带 `correlation_id` |

### 4.2 技术层（可自动化）

| 编号 | 标准 |
|------|------|
| TEC-PGO-1-01 | `defaultPromptFiles()` 单元测试断言 5 个文件名 + sort_order |
| TEC-PGO-1-02 | `FilesForMode` 单元测试覆盖 4 个 mode 下的白名单 |
| TEC-PGO-1-03 | `cmd/migrate-deprecated-prompts/main.go` 集成测试：能处理 stub / 非 stub / 已有 IDENTITY persona 段 三种场景 |
| TEC-PGO-2-01 | `internal/biz/field_guides.go` / `web/src/features/agents/fieldGuides.ts` schema 一致性 lint（CI 比对） |
| TEC-PGO-3-01 | `POST /v1/ai/refine` 5 个 scope 各 1 个 happy path 用例 |
| TEC-PGO-3-02 | refine 失败 fallback 链测试（agent model 失败 → system default 成功） |
| TEC-PGO-3-03 | refine 速率限制中间件单测 |
| TEC-PGO-4-01 | `internal/import/` 包含 yaml_loader / md_loader / validator / planner / applier 单元测试 |
| TEC-PGO-4-02 | `aranea import` e2e：起空白 docker-compose backend，导入 fixture spec，断言资源数量 |
| TEC-PGO-4-03 | `cmd/seed-stockx-org` 重构后行为兼容（同一份 plan 同样数量的 create） |
| TEC-PGO-RED-01 | CI lint：`cmd/aranea/` 不 import `internal/biz` `internal/agent` `internal/data` `pkg/trpc-agent-go` |
| TEC-PGO-RED-02 | CI lint：`internal/import/` 不 import `internal/server` `pkg/trpc-agent-go` |

---

## 5. Feature Flag 与回滚

| Flag | 默认 | 说明 |
|------|------|------|
| `PGO_CATEGORY_RESPONSIBILITY_INJECT` | off（PGO-1 上线后默认 on） | 是否把岗位职责注入 system instruction |
| `PGO_DEFAULT_FILES_V2` | off | 新建 Agent 用 5 文件默认集；off 时仍是 9 文件（保护现网） |
| `PGO_AI_REFINE_V2` | off | 启用 `/v1/ai/refine` 与新前端按钮；off 时旧 `/v1/agents/{id}/files/{fid}/ai-edit` 走旧路径 |
| `PGO_CLI_IMPORT_ENABLED` | on（CLI 端） | CLI 进程内是否注册 `aranea import` 子命令 |

**回滚策略**：
- PGO-1 默认文件变更可通过 flag 即时回退；存量数据不动。
- PGO-1 SOUL → IDENTITY 合并由迁移脚本完成，**保留 SOUL.md 原文件作 backup**（一个版本后再清理）。
- PGO-3 旧 endpoint 保留一个版本周期。
- PGO-4 不会修改既有数据；import 失败可按 `correlation_id` 查询审计反向手工清理。

---

## 6. 风险与对策

| 风险 | 影响 | 对策 |
|------|------|------|
| 删除默认 stub 文件后，存量 Agent 的 IDENTITY.md 已自定义但 SOUL.md 也有内容，合并冲突 | 数据丢失或重复 | 迁移脚本：① 若 IDENTITY.md 已有 `## Persona` 段则跳过；② 否则把 SOUL 内容追加到 IDENTITY 末尾；③ 旧 SOUL 文件保留并打 `deprecated:true` 标记 |
| Markdown 抽取 LLM 输出非合法 YAML | import 失败 | LLM prompt 要求严格 YAML；CLI 落盘前用 `yaml.Unmarshal` 验证；失败提示用户保存 raw output 并人工修复 |
| Refine 服务被滥用造成 token 成本爆炸 | 账单 | 速率限制 10/min/user；字符上限 5000；审计每次 token 消耗 |
| 字段重命名引起用户混淆 | UX | 同步更新 placeholder + helper text；首版上线弹一次"我们调整了字段命名"toast |
| CLI 在 CI 用 `--confirm` 大批量误操作 | 数据污染 | `--confirm` 必须与 `--base-url` 显式同时提供；远端 base-url 默认禁止 `--confirm` 除非 `ARANEA_ALLOW_REMOTE_CONFIRM=1`；审计强制开启 |
| seed-stockx-org 重构后产物不一致 | 现网回归 | 重构前后跑同一份 spec.yaml 比对数据库 row 一致；行为差异需在 PR 描述说明 |

---

## 7. 文档与培训

| 文档 | 目标读者 | 更新内容 |
|------|---------|---------|
| `docs/guides/prompt/assembly.md` | 后端开发 | 修正 `task` mode 不再含 HEARTBEAT；新增 `<role_responsibility>` 段说明；默认文件清单更新 |
| `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | AI/全员 | 新增 §"PGO 三层职责（L1 岗位 / L2 个体 / L3 文件）"小节 |
| `docs/需求/6 agent-setting-file.md` | 产品 | 默认文件清单更新；可选文件机制说明 |
| `docs/需求/4.agent-type.md` | 产品 | 字段命名"岗位职责"与注入路径说明 |
| `docs/需求/58 prompt-governance.design.md` | 全员 | 本期设计文档（同步产出） |
| `docs/需求/58-prompt-governance-development.md` | 全员 | 本期开发计划（同步产出） |
| `cmd/aranea/import/README.md` | 用户 | `aranea import` 使用手册 + spec 示例 |

---

## 8. 相关需求与历史

- 上游需求：`docs/需求/6 agent-setting-file.md` · `docs/需求/4.agent-type.md` · `docs/需求/25 cli.md`
- 历史决定：`docs/guides/prompt/assembly.md` 关于 `BuildSystemPrompt` 的拼装顺序为不可变契约
- 关联设计：`docs/需求/7 agent-evolution.md` 中 SOUL.md 演化 writeback 路径
- 关联实施：`docs/需求/25-cli-implementation-plan-2026-05-27.md`（CLI 主干分阶段；PGO-4 是独立子项目）

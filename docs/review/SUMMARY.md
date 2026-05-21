# Aranea-Agents 模块 Review 综合汇总

> **生成时间**：2026-05-21  
> **覆盖模块**：33 个（含占位模块 CLI/Ecosystem/TTS）  
> **基准文档**：`docs/需求/README-development.md` · `docs/guides/execution-plan.md`

---

## 一、总体评分

| 统计项 | 数值 |
|--------|------|
| 平均分 | 76.0 / 100 |
| 最高分 | 85（Message/WS）|
| 最低分 | 25（TTS — 占位）|
| 占位模块排除后平均分 | **79.8 / 100** |

### 模块评分排行

| 排名 | 模块 | 得分 | 备注 |
|------|------|------|------|
| 1 | Message / WS (51) | **85** | 架构设计最完整 |
| 2 | Chat (1) | **84** | 主链路质量高 |
| 2 | Event System (34) | **84** | SRP 拆分完善 |
| 4 | Team (11) | **83** | 5 种模式已通 |
| 4 | Gateway (35) | **83** | RunRegistry 完善 |
| 6 | 系统架构 (0) | **82** | 文档基线扎实 |
| 6 | Cron (21) | **82** | 主链路稳定 |
| 6 | Runner (40) | **82** | Builder 架构正确 |
| 6 | Evaluation (33) | **82** | Phase 5 完整 |
| 10 | Provider (9) | **81** | 指标/定价已通 |
| 10 | Plugin/Callback (22/28) | **81** | 9 内置插件完整 |
| 10 | A2A (26) | **81** | Phase 3.5 完整 |
| 10 | Planner (39) | **81** | ReAct/A2UI ✅ |
| 14 | Agent 2–8 | **80** | 核心功能已通 |
| 14 | Tools (23) | **80** | Override/统计 ✅ |
| 14 | MCP (19) | **80** | OAuth/重连 ✅ |
| 17 | FlowLogger (52) | **79** | Phase 2 落库待补 |
| 17 | Session (10) | **79** | 批量 UI 缺失 |
| 17 | Token/Usage (29) | **79** | 定价提示待补 |
| 20 | Graph (36) | **77** | 节点类型待补 |
| 21 | Knowledge (37) | **76** | OCR 待规划 |
| 21 | Channel (17) | **76** | 测试薄弱 |
| 23 | Memory (12–16) | **74** | P0 边界风险 |
| 24 | Telemetry (24) | **73** | Span 语义待补 |
| 25 | Artifact (27) | **72** | 附件引用缺失 |
| 26 | Avatar (50) | **70** | 功能简单但规范 |
| 27 | Admin/Auth | **75** | 缺需求文档 |
| 28 | Skill (20) | **78** | HTTP 旁路问题 |
| 29 | Monitor (18) | **78** | 分层漂移 |
| 30 | CodeExecutor (32) | **78** | 测试待补 |
| — | CLI (25) | **42** | 早期占位 |
| — | Ecosystem (30) | **38** | 接近 mock |
| — | TTS | **25** | 占位 |

---

## 二、P0 风险清单（须立即处理）

| ID | 模块 | 风险描述 | 验证方法 |
|----|------|---------|---------|
| **P0-001** | Memory | `internal/biz/memory_runtime_set.go` 疑似 import `pkg/trpc-agent-go/memory`，违反 biz 不 import trpc-agent-go 红线 | `make runtime-boundary` |
| **P0-002** | Chat | `useChatWorkspace.ts` 约 1500 行，业务状态机高度集中，Bug 传播风险极高 | 代码 review + 拆分 |
| **P0-003** | Session | 确认无 `sessionmemory.Store` 直连残留（service 层不绕过 biz Usecase 端口） | CodeGraph callers |

---

## 三、P1 风险清单（当前迭代修复）

### 架构边界

| ID | 模块 | 风险描述 |
|----|------|---------|
| P1-001 | 系统/Chat | `PendingMessageQueue` 实现在 `internal/service/chat_pending.go`，应下沉到 `internal/runtime` |
| P1-002 | 系统/Graph | `internal/data.go` 绑定 trpc session/graph checkpoint，Data 层与 runtime 耦合 |
| P1-003 | Provider | `biz <-> provider` 双向依赖（`internal/llminspect` 拆环未完成）|
| P1-004 | Skill | `server/skill_import_http.go` 绕过 proto/service 层鉴权与观测 |
| P1-005 | Memory | L0-L4 产品层与框架 MemoryService 双轨，主从未明确 |

### 功能缺口

| ID | 模块 | 风险描述 |
|----|------|---------|
| P1-006 | FlowLogger | Phase 2 落库（`ListFlowLogs` HTTP 历史查询）尚未实现 |
| P1-007 | Team | RunTest UI / `step_started` Envelope / Summary 持久化待补 |
| P1-008 | A2A | Phase 4：网关健康 Cron 未实现 |
| P1-009 | Session | 批量治理前端 UI 缺失 |
| P1-010 | Auth | 缺少 `admin-auth.md` 需求文档 |

### 前端分层

| ID | 模块 | 风险描述 |
|----|------|---------|
| P1-011 | 前端（多处）| `MonitorPage`、`EcosystemPage`、`SystemSettingsPage`、`PluginsPage` 直连 API，违反 Page→composable 分层 |
| P1-012 | Agent | `AgentSettingsPage.vue` 约 488 行，目标 < 300 行 |

### 测试缺失

| ID | 模块 | 风险描述 |
|----|------|---------|
| P1-013 | Chat/WS | WS 协议层无集成测试（连接→Envelope→断连） |
| P1-014 | Memory | `MemoryWorker` 无测试 |
| P1-015 | Channel | 三平台 Webhook 验签无自动化测试 |
| P1-016 | Quota | `CheckQuota` Turn 前拦截无单测 |

---

## 四、P2 风险清单（下迭代修复）

| ID | 模块 | 风险描述 |
|----|------|---------|
| P2-001 | Memory | 衰减算法未实现；图谱与进化 Tab 为占位 |
| P2-002 | Knowledge | OCR pipeline 未规划 |
| P2-003 | Artifact | 签名下载 URL、Chat 附件引用、CodeExecutor→Artifact 管道缺失 |
| P2-004 | Graph | LLM/Tool 节点属性面板不完整；HITL 产品化 |
| P2-005 | Monitor | latency 聚合 / Dashboard 自动刷新未实现 |
| P2-006 | Telemetry | per-span `otel_id` 缺失；gRPC 采样配置待补 |
| P2-007 | Token | 定价规则未配置时无用户提示 |
| P2-008 | A2A | 速率限制（Phase 4）未实现 |
| P2-009 | Plugin | Phase 4 沙箱/版本时间线未规划 |
| P2-010 | Evolution | 趋势图/diff/护栏（AGT-16）待补 |
| P2-011 | Agent | 批量操作（LIST-04）未实现 |
| P2-012 | Team | 5 种模式差异化测试缺失 |

---

## 五、五面完整度总览

| 模块 | Contract | Domain | Runtime | Persistence | UI/Operate |
|------|----------|--------|---------|-------------|------------|
| Chat | ✅ | ✅ | ✅ | ✅ | ✅ |
| Agent 2–8 | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Session | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Team | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Graph | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Memory | ✅ | ✅ | ⚠️ | ✅ | 🟡 |
| Tools/MCP/Skill | ✅ | ✅ | ✅ | ✅ | ✅ |
| Plugin/Callback | ✅ | ✅ | ✅ | ✅ | ✅ |
| Monitor | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Knowledge | ✅ | ✅ | ✅ | ✅ | ✅ |
| Evaluation | ✅ | ✅ | ✅ | ✅ | ✅ |
| A2A | ✅ | ✅ | ✅ | ✅ | ✅ |
| Channel | ✅ | ✅ | ✅ | ✅ | ✅ |
| Cron | ✅ | ✅ | ✅ | ✅ | ✅ |
| Artifact | ✅ | ✅ | ✅ | ✅ | 🟡 |
| Telemetry | ✅ | — | ✅ | ✅ | 🟡 |
| Token/Usage | ✅ | ✅ | — | ✅ | ✅ |
| CodeExecutor | ✅ | ✅ | ✅ | — | 🟡 |
| Ecosystem | ✅ | ❌ | ❌ | ❌ | 🟡 |
| CLI | ✅ | ❌ | ❌ | ❌ | ❌ |
| TTS | ✅ | ❌ | ❌ | ❌ | ❌ |

---

## 六、架构红线最终核查

| 红线 | 状态 | 说明 |
|------|------|------|
| `internal/biz` 不 import `trpc-agent-go` | ⚠️ **须验证** | `biz/memory_runtime_set.go` 需 `make runtime-boundary` 确认 |
| `internal/server` 不调 Runner/Agent/LLM | ✅ | `skill_import_http.go` 有旁路但不直调 Runner；P1 修复计划中 |
| Chat/Team/Monitor 实时主通道为 `/v1/ws` | ✅ | SSE 仅用于 A2A/MCP 外部协议 |
| `internal/data` 不绑定 trpc runtime | 🟡 | Graph checkpoint 仍在 data.go；P1 改进计划中 |
| 前端分层 Page→store→feature API | 🟡 | 多处存在漂移；P1 规范化计划中 |
| `make runtime-boundary` 通过 | ⚠️ **须运行** | 验证 biz/memory 边界 |

---

## 七、文档质量汇总

| 问题类型 | 数量 | 涉及模块 |
|---------|------|---------|
| 缺少完整三件套 | 3 | Admin/Auth（缺需求）、TTS（缺设计）、Chat execution trace（无独立 dev 计划）|
| 命名不规范 | 6+ | `4.agent-type.*`、`52-flow-logger*`、`50 Avatar.*`、`message-development.md`、`admin-auth.*` |
| 开发计划待同步 | 2 | `40-runner-development.md`、`23-tools-development.md` |
| 需求正文含实现细节 | 1 | `50 Avatar.md` |
| 模块编号跳号/缺失 | 1 | 31 缺失（Memory 内部引用"31 记忆管理界面"）|
| 占位模块未明确标注 | 3 | CLI、Ecosystem、TTS |

---

## 八、建议下一步行动

### 立即（本周内）

1. 运行 `make runtime-boundary`，确认并修复 `biz/memory_runtime_set.go` 边界（P0-001）。
2. 开始拆分 `useChatWorkspace.ts`，提取 `useFollowUpQueue`、`useAwaitReply`（P0-002）。
3. 将 `server/skill_import_http.go` 迁入 `SkillService`（P1-004）。

### 本迭代（2 周内）

4. 实现 FlowLogger Phase 2 落库（P1-006）。
5. 实现 Team RunTest UI + `step_started` Envelope（P1-007）。
6. 将 `PendingMessageQueue` 从 Service 下沉到 `internal/runtime`（P1-001）。
7. 补充 `admin-auth.md` 需求文档（P1-010）。
8. 补充 Auth 路径单测、Quota 拦截单测、Channel 验签单测（P1-013/015/016）。
9. 统一前端分层漂移（为 MonitorPage/PluginsPage 等添加 composable）（P1-011）。
10. 继续拆分 `AgentSettingsPage.vue` 至 < 300 行（P1-012）。

### 下迭代（1 个月内）

11. 规划 Knowledge OCR pipeline（P2-002）。
12. 实现 Artifact 签名下载 URL + Chat 附件引用（P2-003）。
13. 完善 Graph LLM/Tool 节点属性面板（P2-004）。
14. 实现 A2A Phase 4：网关健康 Cron + 速率限制（P1-008 / P2-008）。
15. 补全 Memory 衰减算法；实现图谱与进化 Tab（P2-001）。
16. 为 Ecosystem/CLI/TTS 在文档和 UI 中明确标注"技术预览"状态（P3）。

---

*本汇总文档由 AI 代码 Review 自动生成，截至 2026-05-21。所有分数和风险条目基于文档分析与代码结构扫描，需结合实际运行验证。*

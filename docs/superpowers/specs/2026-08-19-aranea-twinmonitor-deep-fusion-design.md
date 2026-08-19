# aranea × TwinMonitor 深度融合完整方案

> 版本：v1.2（实施计划交叉核验勘误版）　日期：2026-08-19
> v1.2 变更：8 份实施计划撰写时对代码逐行核验，发现 12 处总纲与代码的不一致，以代码为准勘误（见文末「附录 F：v1.2 勘误表」）。实施细节以 `docs/superpowers/plans/2026-08-19-p*.md` 为准。
> v1.1 变更：依据 R1~R9 评审结论修订——GNS3 平面定位（R1）、auto 模式与工具风险联动规则（R2）、Skill 机制修正为动态路由注入（R3）、MCP 新增域收敛为 2 个（R4）、新增 §3.6 降级/对账/配额治理（R5/R6/R7）、多环境拓扑与自监控场景补充（R8/R9）。
> 前置文档：TwinMonitor AI融合总体设计方案 v1.2 / 13-AI智能运维 v2.1 / 14-自动修复与自愈 v3.0 / 19-语音精灵 v2.0
> 本次设计产出定位：新写深度融合总纲（非修订既有文档），聚焦「单通道 MCP 收敛 + 技能/记忆体系化 + 全场景 E2E 落地路线」。

---

## 目录

1. [业务关联分析](#1-业务关联分析)
2. [目标架构（五层）](#2-目标架构五层)
3. [核心设计决策](#3-核心设计决策)
   - 3.1 决策 1：MCP 单通道收敛（三阶段迁移）
   - 3.2 决策 2：双层审批不变，destructive 工具统一挂 aranea interrupt
   - 3.3 决策 3：技能（Skill）体系化运维剧本沉淀
   - 3.4 决策 4：记忆 L0~L4 运维映射与双沉淀
   - 3.5 决策 5：知识双通道分工
4. [端到端业务流](#4-端到端业务流)
   - F1 告警自愈闭环
   - F2 语音机柜巡检 S1
   - F3 语音报告 S2
   - F4 智能告警 S3
   - F5 告警追踪自愈可视化 S5
   - F6 定时巡检与洞察看板
5. [通信契约矩阵](#5-通信契约矩阵)
6. [落地路线图](#6-落地路线图)
7. [对既有文档的修订点清单](#7-对既有文档的修订点清单)
8. [附录](#8-附录)
   - A. 工具映射全表
   - B. 事件主题表
   - C. REST 端点契约表
   - D. Skill 清单与触发词
   - E. 记忆 L0~L4 运维映射表

---

## 1. 业务关联分析

### 1.1 两系统本质关系：AI 内核与业务域的纵向分工

| 维度 | aranea-agents | TwinMonitor |
|------|---------------|-------------|
| **系统定位** | 通用多智能体编排底座（AI 内核外置） | 机场数字孪生集中监控平台（业务域全集） |
| **核心拥有** | LLM Provider 目录 / Agent 运行时 / Team·Graph·Spirit 三层编排 / 五层记忆 L0~L4 / 知识库 / Skill / MCP Host / 全链路可观测（Trace/Flow Log）/ 成本配额 | 22 个业务模块：采集(03) / 告警(04) / 资产(02) / 线路(08) / 拓扑 / 运维工具(16) / 知识工单(10) / UE5 3D(07) / 语音(19) / 运维报告(06) … |
| **核心缺失** | 没有真实业务场景与数据 | 没有 AI 内核（D1 决议：不自研，委托 aranea） |
| **落地形态** | 作为「外部编排底座」被 twinmonitor 调用和治理 | 13-aiops 作为「AI 治理与接入层」承载 twinmonitor 的运维智能体目录、场景模板治理、审批中心、任务镜像、MCP 安全层 |

**关键认知**：aranea 不是 twinmonitor 的一个子模块，而是 twinmonitor 全部 AI 能力的**外部编排底座**；twinmonitor 也不是 aranea 的一个「下游系统」，而是 aranea 在**机场运维域的专属能力层**。二者通过标准化的控制面（REST）+ 能力面（MCP）+ 事件面（Webhook/NATS）解耦协作。

### 1.2 业务价值链（六段闭环）

```
感知层          分析层              决策层              执行层            验证层            沉淀层
─────────────────────────────────────────────────────────────────────────────────────────────────────────
03采集 → 04告警 → RCA/诊断(aranea) → 策略/审批(14+13) → 编排执行(aranea) → 验证回滚(14) → 知识/记忆(双系统)
─────────────────────────────────────────────────────────────────────────────────────────────────────────
         AI 智能运维模块(13)负责触发、参数组装、落库、事件发布、误报治理
         aranea 负责：分析/编排/执行/记忆写入/Trace
         14 自动修复与自愈负责：策略匹配、状态机推进、频率冷却、验证回滚
         10 知识库负责：主数据存储
         16 运维工具负责：唯一命令下发出口（SSH/Agent/API）
```

**建设定位：L3（有条件自治）**——AI 负责分析与建议，自动执行按风险分级管控：低风险全自动、中风险审批后执行、未知/新场景仅建议。这是价值与风险平衡的现实落点（引自需求调研文档 §2.2）。

### 1.3 全场景清单

| 场景 | 触发入口 | aranea 编排模式 | MCP 工具域 | 交付物 |
|------|----------|-----------------|------------|--------|
| F1 告警自愈闭环 | 04 告警 events | Graph（remediate 图 + verify 节点裁决） | gns3 + network + alarm + metric | 修复执行记录 + RCA 报告 |
| F2 语音机柜巡检 S1 | Voice Bridge(8140) KWS 唤醒 + ASR | Spirit（单工具 + scene_actions） | asset + metric + scene | scene_actions JSON + TTS |
| F3 语音报告 S2 | 语音指令 | Spirit（多步骤分解） | metric + report + notify | 报告文件 + 大屏全息卡片 |
| F4 智能告警 S3 | 04 告警预检/风暴聚合 | Agent（RCA + 叙述生成） | alarm + metric + knowledge | 误报降级 / 事件叙述 |
| F5 告警追踪自愈可视化 S5 | 告警模式切换 / 手动追踪 | Graph / Spirit | alarm + scene（UE 指令） | UE 相机追踪 + 多告警卡片 |
| F6 定时巡检 + 洞察看板 | Cron(13) / 手动 | Team / Graph | metric + server + knowledge | 巡检报告 + 看板指标 |
| F7 基础设施自愈（aranea 自身） | 04 告警（aranea 指标） | **无（aranea 旁路，14 直驱 16）** | 不经 MCP，14→16 直连 | aranea 宿主修复记录 |

---

## 2. 目标架构（五层）

### 2.1 总体架构图

```
┌────────────────────────────────────────────────────────────────────────────────────────────┐
│                                      接入层（用户触点）                                        │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────────────────┐ │
│  │ TwinWeb 管理台│  │ UE5 DTRoomTwin    │  │ Voice Bridge     │  │ 第三方系统（未来预留）   │ │
│  │ (告警/大屏)   │  │ (3D 数字孪生+精灵) │  │ (8140 KWS+ASR+TTS│  │                         │ │
│  └──────┬───────┘  └────────┬─────────┘  └────────┬─────────┘  └─────────────────────────┘ │
│         │                    │                     │                                         │
│         │ HTTP/WS            │ WebSocket           │ WS + HTTP                               │
│         ▼                    ▼                     ▼                                         │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│                                   编排层：aranea-agents                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ LLM Provider 目录 / Agent 运行时 / Team 六模式 / Graph 图编排 / Spirit 动态编排        │  │
│  │ 五层记忆 L0~L4 / Skill 体系 / MCP Host / Trace 可观测 / 成本配额 / 上下文压缩         │  │
│  │                                                                                      │  │
│  │  ▶ 12 个预设运维 Agent（种子由 13 同步）                                              │  │
│  │  ▶ 技能：gns3-remediate-runbook / rca-evidence-path / cabinet-inspection-script    │  │
│  │  ▶ 记忆：L3 运维经验事实（作用域=agent）                                             │  │
│  └──────────────────────────────────────────────────────────────────────────────────────┘  │
│         │                           ▲                      │                                │
│         │ REST 控制面               │ Webhook 事件回写     │ MCP 能力面                     │
│         ▼                           │                      ▼                                │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│                              治理层：TwinMonitor 13+14                                      │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐    │
│  │ 13-aiops（HTTP:8100 / gRPC:9100）：接入/智能体/场景/任务镜像/审批/脚本/定时/MCP-Server│    │
│  │ 14-remediation（HTTP:8110 / gRPC:9110）：策略/状态机/频限冷却/验证回滚/工作台/仪表盘   │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│         │                          │                           │                             │
│         │ NATS 事件                │ HTTP 查询                 │ MCP SSE（唯一工具通道）      │
│         ▼                          ▼                           ▼                             │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│                              能力层：TwinMonitor 22 个业务模块                               │
│  02资产  03采集  04告警  05通知  06数据/报告  08线路  09库存  10知识/工单  12可观测  16运维工具│
│                                                                                              │
│  关键原则：16 运维工具是 AI 一切设备操作的「唯一命令出口」，AI 不持有设备凭据。               │
│  关键原则：db.* 工具统一经 06 语义层；metric.* 统一经 06 语义层。                            │
│  关键原则：告警严重度评估（alarm.severity_assess）改为本地规则，消除 MCP↔aranea 循环调用。     │
└────────────────────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
   数据层：PostgreSQL(twinmonitor/twinmonitor_log) + InfluxDB + NATS + Redis
```

### 2.2 模块隔离与职责切分总表

| 能力域 | 归属 | 说明 |
|--------|------|------|
| LLM Provider/模型目录/Key 管理/成本配额 | **aranea** | 模型密钥唯一落点在 aranea；13 不存储 LLM Key |
| Agent 定义与运行时 | **aranea** | 12 个运维预设 Agent 以"IT 运维岗位包"种子同步至 aranea；13 保留治理镜像 |
| 编排引擎（Team/Graph/Spirit） | **aranea** | 全部工作流/多智能体编排执行；含 Checkpoint/中断恢复/TimeTravel |
| 编排执行可观测（Trace/Flow Log） | **aranea** | 13 任务中心透传展示 aranea Trace 链接 |
| 运维场景模板治理 | **13（本模块）** | 场景 = aranea Graph 定义的治理注册项（graph_id 映射） |
| 任务记录与审计（运维视角） | **13** | 执行镜像落 twinmonitor_log，关联 aranea run_id |
| 人工审批（HITL）与超时治理 | **13** | 审批单/审批人/原因/超时扫描；aranea Graph Interrupt/Resume 承载挂起恢复 |
| 平台能力 MCP 化与安全层 | **13** | TwinMonitor-MCP-Server，风险五级 + 默认只读 + 全程审计 |
| 脚本库/配置模板 | **13** | 经 MCP 暴露执行，实际下发走 16 运维工具 |
| 定时调度 | **13** | 到点触发 aranea 场景执行 |
| RCA/自动诊断的记录与事件治理 | **13** | 分析执行在 aranea，结果落 twinmonitor_log |
| 修复策略/执行状态机/频限冷却/验证回滚 | **14** | 经 13 执行代理触发 aranea 修复场景 |
| 知识库文档主数据 | **10** | 经 MCP `knowledge.search/create` 供 AI 检索与写入 |
| 设备凭据与命令下发 | **16** | AI 一切设备操作经 MCP → 16，AI 不持凭据 |

### 2.3 通信矩阵（本期无 A2A）

| 通道 | 协议 | 方向 | 承载内容 | 当前状态 |
|------|------|------|----------|----------|
| REST 控制面 | HTTPS + Bearer token | 13→aranea | Agent/Graph/Run CRUD、Interrupt/Resume、记忆写入、配额查询 | ✅ 已落地（twin_openapi_compat.go） |
| Webhook 事件回写 | HTTPS + HMAC-SHA256 | aranea→13 | run 生命周期事件、interrupt 事件 | ✅ 已落地（GraphRunEventSink + outboundwebhook） |
| MCP 能力面 | SSE（MCP 2024-11-05） | aranea→13 | 工具调用（8 领域 + 脚本 + 新增 gns3/network） | ⚠️ 骨架就绪，需扩展 gns3/line 工具 |
| NATS 平台内事件 | NATS JetStream | 双向订阅/发布 | alarm.events / ai.task.events / ai.approval.pending / ai.analysis.completed / remediation.events / ai.aranea.health | ✅ 主题就绪，E2E 待验证 |
| WebSocket 场景指令 | WS JSON | aranea→Voice Bridge→UE5 | scene_actions（focus_entity/open_panel/track_alarm 等） | ⚠️ 语音模块就绪，场景指令链路待 E2E |

**A2A 演进预留**：本期不引入 A2A 协议。远期当需要支持第三方 A2A peer 联邦（如跨机场联盟、跨组织运维协作）时，aranea 的 A2A 模块（internal/a2a/ 已具备 federation/invoker 基础）可升级为标准控制面，现有 REST 控制面保留为兼容层。

---

## 3. 核心设计决策

### 3.1 决策 1：MCP 单通道收敛（三阶段迁移）

**现状**：aranea 侧有 17 个内置 twinops 工具（`gns3_exec/fault_inject/fault_clear`、`twin_line_probe` 等）直连 twinmonitor OpenAPI；13 侧有 24 个内置 MCP 工具 + 脚本动态工具。**两条工具通道并存**，审计与安全策略分裂。

**目标**：一切平台能力调用（含设备命令）收口到 13 的 TwinMonitor-MCP-Server，aranea 作为 MCP Host 消费。**aranea 内置 twinops 工具逐步退役**。

#### 3.1.1 MCP 工具目录 v2 扩展

现有 24 个内置工具按 8 大领域分组（alarm/asset/metric/server/network/database/knowledge/notify_ticket/config）。为控制治理面碎片化（评审 R4），新增工具**只新增 2 个域**：`gns3`（仿真演练域）与 `ops`（运维运营域），线路类并入既有 `network` 域：

| 新增工具名 | 领域 | 风险等级 | 只读 | 对应原 aranea 工具 |
|-----------|------|----------|------|---------------------|
| `gns3.health_check` | gns3 | readonly | ✅ | `gns3_health_check` |
| `gns3.exec` | gns3 | high | ❌ | `gns3_exec` |
| `gns3.fault_inject` | gns3 | destructive | ❌ | `gns3_fault_inject` |
| `gns3.fault_clear` | gns3 | destructive | ❌ | `gns3_fault_clear` |
| `network.line_status` | network | readonly | ✅ | `twin_line_status` |
| `network.line_probe` | network | low | ❌ | `twin_line_probe`（主动探测，有副作用） |
| `network.line_events` | network | readonly | ✅ | `twin_line_events` |
| `ops.remediation_status` | ops | readonly | ✅ | `twin_remediation_status` |
| `ops.inspection_query` | ops | readonly | ✅ | `twin_inspection_query` |
| `ops.collector_status` | ops | readonly | ✅ | `twin_collector_status` |

**GNS3 平面定位（评审 R1，关键澄清）**：当前环境的设备操作面是 **GNS3 共享仿真执行环境**（14 代码实证："并发修复同一故障会争抢共享执行环境（GNS3 console 等）"），gns3 域工具的后端是 GNS3 控制器（gns3_agent），**不是** 16 opstools 的生产设备 SSH/Agent 通道。因此：
- gns3 域独立成域，与 16 生产命令通道（server/db/network 配置类工具经 opstools 下发）物理隔离，避免演练流量误入生产通道；
- gns3 域承载双重角色：① 修复剧本的**演练验证平面**（新剧本先在 GNS3 仿真验证通过才允许上生产白名单）；② 当前演示环境的**实际设备面**；
- 生产化演进时，gns3 域工具键不变，仅后端实现切换为 opstools 生产设备通道，上层 aranea 剧本与 budget 规则零改动。

> 新增工具总表见附录 A。

**安全桥接规则**：
- gns3 域 4 个工具全部为 destructive/high 等级，其中 `gns3.fault_inject`/`gns3.fault_clear` 为 destructive
- `gns3.exec` 为 high（设备命令执行），实际命令下发经 16 运维工具通道，MCP 侧只做参数校验与审计
- 全部 gns3 工具启用时须同步在 aranea 侧 `agent_runtime_settings` 配置 `requires_confirmation=true`，触发 HITL interrupt（详见 3.2）
- MCP 调用历史（`ai_mcp_call_history`）记录全部参数摘要与结果，destructive 工具额外记录拦截原因（如未审批）

#### 3.1.2 三阶段迁移路线

| 阶段 | 目标 | 动作 | 风险 | 时长估算 |
|------|------|------|------|----------|
| **P1 双跑** | MCP 工具上线，内置工具保留 | ① 13 MCP-Server 注册新增 10 个工具；② aranea 侧 `mcp_servers` 表登记 twinmonitor MCP-Server（SSE）；③ 12 预设 Agent 的 tool_whitelist 复制为 MCP 版本（新 agent 用 MCP，旧的不动） | 低：新增工具默认禁用，需显式启用 | 1 周 |
| **P2 切换** | 运维 Agent 与 remediate 图全面切 MCP | ① 12 预设 Agent 的 tool_whitelist 全部指向 MCP 工具键；② remediate 图节点（verify/取证/fault_clear）改用 MCP 工具；③ 修复 budget 规则与循环守卫保持原语义；④ E2E 验证（test/ts10-gns3 扩展） | 中：需验证 budget 规则在 MCP 延迟下不变形 | 2 周 |
| **P3 退役** | 内置工具彻底移除 | ① `builtin_tools_seed.go` 移除 twinops 17 工具行 + reseed DDL 迁移（按既有先例：`{Version: 递增, Name: "builtin_platform_tools_twinops_reseed", Func: ddlBuiltinPlatformTools}`）；② `internal/tools/twinops/` 目录标记废弃；③ `twin_openapi_compat.go` 中不再路由 twinops 相关调用 | 低：种子幂等 ON CONFLICT DO NOTHING，重跑安全 | 1 周 |

**技术实现要点**：
- aranea 侧 MCP Host 登记：在 `mcp_servers` 表新增一行 `server_key = "twinmonitor"`，`ConfigJSON` 含 SSE URL（`http://aiops:8100/mcp/sse`）、调用凭据（由 13 侧 `ai_mcp_clients` 的 client_id/secret 对配）。`MCPVersionHash` 按 server_key + ID + ConfigJSON 计算。
- 工具装配：在 `agent_runtime_settings` 的 `tools_enabled` 中新增 `twinmonitor.*` 工具键前缀开关；高危工具（gns3.fault_*）默认关闭，需运维管理员显式授权。
- 13 侧 `ai_mcp_clients` 为 aranea 生成专用 client_id/secret，风险上限设为 `destructive`（即全开），工具子集不设限（aranea 作为内部 Host 可信）。

### 3.2 决策 2：双层审批不变，destructive 工具统一挂 aranea interrupt

**第一层（14 策略级审批）**：修复策略的 `execution_mode = approval` 时，14 创建执行记录进入 `pending_approval` 状态，值班审批员在 13 审批中心通过后才创建 aranea 执行任务。**审批对象是「该不该对这条告警执行修复」**。

**第二层（aranea 工具级确认）**：执行过程中遇到 `requires_confirmation=true` 的工具（`gns3.fault_inject`、`gns3.fault_clear` 等），aranea Graph 触发 Interrupt，生成 interrupt_id，通过 Webhook 推送 `run.interrupted` 事件到 13。13 创建审批待办（`ai_approvals` 表），值班审批员通过后调用 `POST /api/v1/runs/{id}/interrupts/{interrupt_id}/resume` 恢复执行。**审批对象是「该不该执行这条具体命令」**。

**两层关系**：
- 第一层是「准入审批」（是否启动修复场景），第二层是「执行中确认」（是否执行具体高危操作）。
- 第一层拒绝 → 执行记录状态变为 `rejected`，不创建 aranea 任务。
- 第二层拒绝 → aranea Run 状态变为 `failed`（或按图定义进入回滚分支），14 通过 `ai.task.events` 消费到失败事件后推进执行状态机到 `failed`。
- 第二层超时 → aranea 侧按 graph 超时配置自动取消；13 ApprovalScanner 扫描超时后自动拒绝，并调用 Resume 携带拒绝原因。

**与现有 ops_change_execution 机制的衔接**：现有 `ops_change_execution` 变更执行岗审批（项目记忆中「高危工具操作必须审批」规则）映射到第二层：工具级 interrupt 的审批人解析走 13 审批中心 `approver_roles` 角色码配置，与 twinmonitor 统一 RBAC 对齐。

**策略-工具风险联动规则（评审 R2，新增）**：14 的 `execution_mode=auto` 策略若不加约束地关联含 destructive 工具的修复场景，则每次执行都会被第二层 interrupt 卡住——"全自动"名存实亡。联动规则如下：

| 策略 execution_mode | 场景含工具最高风险 | 行为 |
|---------------------|-------------------|------|
| auto | ≤ high | 全自动，无 interrupt（第二层不触发） |
| auto | destructive | **需策略预授权**：策略创建/启用时对「场景×工具组合」做一次显式授权（记录 grant_policy=always + approval_ttl），授权有效期内 destructive 工具调用自动放行（aranea 侧经系统级 Resume 直通，不再产生审批待办）；未预授权的策略禁止启用（14 侧校验） |
| approval | 任意 | 第一层审批通过后，第二层仍按工具风险逐个 interrupt（双重确认） |
| suggestion | 任意 | 不创建执行，仅产出建议 |

> 预授权是「一次审批、N 次执行」的让渡，必须配套：冷却期 + 频率限制（14 已有）+ 场景级配额熔断（§3.6）三重兜底。GNS3 演练平面内的 destructive 工具可放宽为免预授权（演练环境无生产风险），由 13 按目标平面标记判定。

### 3.3 决策 3：技能（Skill）体系化运维剧本沉淀

**现状（评审 R3 修正）**：aranea 技能体系远比"CRUD + 静态绑定"成熟，真实机制是**运行时动态路由注入**（[skill_guidance_inject.go](file:///f:/myproject/aranea-agents/internal/agent/skill_guidance_inject.go)）：

- **路由**：BeforeModel hook 按本轮 query 与 Agent 策略解析候选技能——触发词（Triggers）匹配 + 语义（embedding）匹配 + 健康度指标过滤，每次 invocation 记忆化一次；
- **注入**：选中技能渲染为 guidance cue（上限 4000 字符）注入模型上下文，工具循环内复用缓存不重复渲染；
- **显式加载**：模型也可经 `skill_load` / `skill_run` 工具主动加载技能全文/执行；
- **进化闭环**：`skill_curator_worker`（技能整理）+ `llm_skill_evolver`（LLM 进化）+ `skills_butler`（推荐）+ `skill_trigger_golden_runner`（触发词金标回归）——技能是可持续演化的资产，不是静态配置；
- **版本与缓存**：`CurrentVersion` 变更使 Agent `buildKeyFP` 失效触发重建（符合「直改 agents 表必须 bump updated_at」规则）。

**设计**：把已验证的运维剧本沉淀为 4 个核心 Skill，经触发词 + Agent 技能路由策略接入对应预设 Agent：

| Skill 名称 | 绑定 Agent | 触发词（Triggers） | 内容形态 | 运行时行为 |
|-----------|-----------|-------------------|----------|-----------|
| `gns3-remediate-runbook` | 故障诊断 Agent / 变更执行 Agent | "故障自愈"、"自动修复"、"remediate" | Markdown 剧本：取证预算（≤2 次）、第 3 次必须是 fault_clear、复核预算（≤2 次）、方案 C 拦截规则、循环守卫（同工具同参数 2 次后拦截） | 命中路由后 guidance cue 注入，约束工具调用序列；remediate 图场景的任务文本固定含触发词，确保必中 |
| `rca-evidence-path` | 故障诊断 Agent | "根因分析"、"RCA"、"为什么告警" | 标准取证路径：告警详情 → 同时间窗关联告警 → 资产拓扑（asset.cabinet_tree） → 近期变更 → 历史处置经验（knowledge.search） → 指标验证（metric.query） | guidance cue 注入作为「工具调用前规划」约束，引导按固定路径取证；RCA 场景任务文本固定含"根因分析"触发词 |
| `cabinet-inspection-script` | 系统巡检 Agent | "巡检"、"inspect cabinet"、"机柜状态" | S1 语音巡检的标准话术与指令序列：overview → focus_cabinet → cabinet_detail → focus_server → hardware_explode → inventory_card | Spirit 分解任务时路由命中，guidance cue 约束生成白名单内 scene_actions 序列 |
| `alarm-triage-rules` | 告警处理 Agent | "告警分级"、"误报检测"、"告警风暴" | S3 智能告警三道防线规则：预检指标 → 动态基线比对 → 维护窗口抑制 → 聚合组叙述生成 | guidance cue 注入 triage 规则，输出结构化分级结论 |

**Skill 版本管理**：Skill 内容以 Markdown 文件存储于 `internal/skill-library/twinmonitor/`（与 `aranea-coding-guide` §Skill 体系对齐），通过 Skill CRUD API 导入 aranea，版本号按 SemVer 管理。更新 Skill 后，绑定 Agent 的 `buildKeyFP` 因 `SkillHash` 变化自动失效，重新构建 Agent 时生效（符合项目记忆「直改 agents 表必须同步 bump updated_at」规则）。

### 3.4 决策 4：记忆 L0~L4 运维映射与双沉淀

**五层记忆在运维场景的角色映射**：

| 层级 | 运维语义 | 存储 | 写入触发 | 召回方式 |
|------|----------|------|----------|----------|
| **L0 会话记忆** | 语音多轮对话的指代消解上下文（当前聚焦实体栈：cabinet→server→part） | 进程内存 / session cache | Voice Bridge 每轮提交时带「当前聚焦实体栈」 | 随 session 提交注入 prompt |
| **L1 任务记忆** | 单次 Run 内的中间产物（取证结果、命令输出、诊断结论） | Run 上下文 / checkpoint | Graph 节点输出自动累积 | 同 Run 内 LLM 请求自动携带 |
| **L2 短期记忆** | 同一 session 内跨 Run 的上下文（如先 RCA 再修复的关联） | session 存储 | `session.append_event` 自动累积 | `UpdateSessionContextFromLLMUsage` 按真实 usage 回写 |
| **L3 事实记忆** | 运维经验：修复方案有效性、设备「脾气」（如某交换机重启需 90s）、误报模式、标准处置路径 | PG（aranea 侧 facts 表） | 修复闭环终点、RCA 完成、误报标记时显式写入 | `memory.search` 工具按 fact_id 召回；作用域=agent |
| **L4 长期知识索引** | 跨 session/跨 agent 的结构性知识（设备手册、拓扑文档、规章规范） | PG + pgvector / tsvector（10 知识库 + aranea knowledge_chunks） | 知识库文档写入 + chunk 重放 | `knowledge.search`（词法/向量双路） |

**关键修复：session 作用域事实写入问题**

项目记忆已指出：`immediate_fact_writer.go` 硬编码 ScopeType=session 且 ScopeID=sessionID，但 `agent_memory_runtime_policy.go` 的 `L3ScopeTargets` 中没有 session case，导致 session 作用域事实写入后无法被召回。

**设计修正**：
- 运维场景的事实写入统一使用 **agent 作用域**（ScopeType=agent，ScopeID=agentID），而非 session 作用域。
- 修复闭环的沉淀动作：修复成功/失败后，aranea 侧调用 `POST /api/v1/memory/facts` 写入 L3 事实，payload 包含：
  ```json
  {
    "fact_id": "remediation:<execution_no>",
    "type": "domain_knowledge",
    "content": "修复方案摘要（含设备、告警模式、命令、结果、置信度）",
    "scope_type": "agent",
    "scope_id": "<agent_id>",
    "metadata": {
      "source": "twinmonitor_remediation",
      "policy_id": "<policy_id>",
      "alert_pattern": "<alert_title关键词>",
      "device_type": "<asset_category>",
      "success": true,
      "mttr_seconds": 184
    }
  }
  ```
- 13 侧 `ai_rca_records` 的「知识/记忆引用」字段保留双向追溯链接（aranea L3 fact_id ↔ RCA record_id）。

### 3.5 决策 5：知识双通道分工

| 通道 | 归属 | 内容 | 消费方 | 写入方式 | 检索方式 |
|------|------|------|--------|----------|----------|
| **10 知识库（主数据）** | twinmonitor | 人读词条（设备手册、故障案例、规章、拓扑文档） | 运维工程师、UE5 大屏、TwinWeb | 10 业务管理 HTTP API（admin 服务）或 RCA 沉淀入口 | `knowledge.search` MCP 工具（tsvector/trigram） |
| **aranea 知识库（机读增强）** | aranea | 机读 chunk（文档分块、L3 事实派生、运维剧本） | aranea Agent（LLM 检索增强） | `knowledge_write` 工具 / `knowledge.create` MCP 工具 / L3 事实 API | embedding + tsvector 双路（词法库可纯 tsvector） |
| **交叉** | 双写 | RCA 结论、修复方案、误报标记 | 双系统 | 13 侧「知识沉淀联动」功能：同步写入 10 知识库 + aranea L3 记忆 | 互链字段（aranea L3 的 metadata.source=twinmonitor_remediation；10 知识库词条的 source=aranea_rca） |

**关键规则**：
- `knowledge.search` MCP 工具优先检索 10 知识库（词法库，team KB 无语义层时 BM25 空转风险已由 `ReembedDocuments` 修复）；aranea 侧内部 `knowledge_search` 工具检索 aranea 知识库（含向量层）。
- 知识写入必须触发 **chunk 重放**（项目记忆 2026-08-15 事故根治：biz 层 `SetWriteBackReplay` 钩子确保所有写回链路消费方都经过 chunk 重放）。
- 团队知识库是纯词法库（embedding_model 空），检索走 tsvector/trigram，同样依赖 knowledge_chunks 表。

### 3.6 决策 6：降级、对账与配额治理（评审 R5/R6/R7 新增）

#### 3.6.1 aranea 不可用降级矩阵（R5）

13 HealthProber 连续 3 次探测失败 → 实例状态置 `unhealthy` 并发布 `ai.aranea.health` 事件，各消费方按矩阵降级：

| 消费方 | 降级行为 | 恢复行为 |
|--------|----------|----------|
| 14 策略引擎（auto） | 新命中告警**不再创建执行记录**，事件落 `pending_degraded` 队列；已有 running 执行不做状态假设 | 健康恢复后批量重放 degraded 事件（幂等键防重） |
| 14 策略引擎（approval/suggestion） | 照常受理（审批/建议不依赖 aranea 实时调用），仅「批准即下发」按钮置灰 | 恢复后自动解禁 |
| 13 RCA 自动触发 | 订阅者暂停创建 Run，告警事件继续落库不丢 | 恢复后按告警时间窗补触发（≤15 分钟，过期跳过） |
| 13 定时巡检 | 到点跳过并记 `skipped_aranea_down` | 下一周期正常 |
| 19 语音精灵 | 语音指令回复「AI 编排服务暂不可用」，scene_actions 降级为纯本地预置指令 | KWS 心跳探测恢复 |

#### 3.6.2 任务镜像对账（R6）

- 14 FallbackPoller（30s 轮询 running 记录）保留，作为事件丢失的短周期兜底；
- **新增周期对账 worker**（13 侧，5 分钟）：拉取 aranea `GET /api/v1/runs/{id}` 比对 `ai_tasks` 镜像状态，漂移则按 aranea 侧为准修正并发布补偿事件；
- **卡死清扫**（13 侧，10 分钟）：`running` 超过 graph 超时上限 2 倍且无新节点事件的镜像，主动调用 aranea cancel 并置 `timeout` 终态，释放 14 并发槽（在途执行占用 GNS3 共享环境，卡死会阻塞后续修复——呼应 R1）。

#### 3.6.3 场景级配额熔断（R7）

项目已有事故先例：知识检索降级空转导致单任务 179 次工具调用 / 291K tokens（sh-04）。在 aranea 既有成本配额之上，为运维场景增加两级熔断（治理配置存 13 场景定义 `definition`）：

| 维度 | 默认阈值 | 触发动作 |
|------|----------|----------|
| 单 Run 工具调用次数 | RCA 场景 15 次；remediate 场景 12 次（预算 10 + 冗余 2） | 超限即取消 Run，标记 `budget_exceeded`，产出已有证据的部分结论 |
| 单 Run LLM 成本 | 按模型档位配置（如 deepseek-chat 0.5 元/Run） | 同上，并计入洞察看板熔断率指标 |

> 与 aranea 既有循环守卫（同工具同参数 ≥3 次拦截）互补：守卫防「重复」，熔断防「总量」。

#### 3.6.4 多环境拓扑（R8）

`ai_aranea_instances` 表天然支持多实例：dev/test/prod 三套 twinmonitor 各自登记独立 aranea 实例（独立 Bearer token + webhook_secret + 健康探测）。**禁止跨环境共用 aranea 实例**——演练平面的 destructive 工具调用与生产平面必须物理隔离。种子同步按实例独立执行，Agent 编码（如 `ops.fault.diagnosis`）跨环境保持一致以便剧本移植。

#### 3.6.5 自监控闭环（R9）

twinmonitor 已将 aranea 自身纳入监控（register-monitor.ps1 / register-env.ps1 采集 aranea 指标）。自愈闭环同样覆盖 aranea 自身故障：aranea 进程告警 → 14 策略匹配「基础设施自愈」场景 → 修复剧本走 16 opstools 对 aranea 宿主执行（重启/扩容/日志采集），**此类场景不依赖 aranea 在环**（aranea 挂了无法自己修自己）——由 14 直接驱动 opstools 执行，是自愈体系中唯一合法的「aranea 旁路」。

---

## 4. 端到端业务流

### F1 告警自愈闭环（主线）

```
04 告警服务 ──alarm.events──► 13 告警订阅者（JetStream durable + Redis SET NX 去重）
                                    │
                                    ▼
                        ┌─────────────────────┐
                        │ 13 RCA 场景触发      │
                        │ 组装上下文：告警详情/指标/拓扑/变更/知识 │
                        └──────────┬──────────┘
                                   │ POST /api/v1/runs (graph_id=rca-scenario)
                                   │ webhook_url=http://aiops:8100/webhooks/aranea
                                   │ idempotency_key=alarm:<alarm_id>:rca
                                   ▼
                        ┌─────────────────────┐
                        │ aranea RCA Team      │
                        │ 经 MCP 取证：        │
                        │ alarm.get → asset.get → metric.query → knowledge.search │
                        │ Skill: rca-evidence-path（约束取证路径）               │
                        │ 产出：根因/置信度/修复建议                             │
                        └──────────┬──────────┘
                                   │ Webhook: ai.analysis.completed
                                   ▼
                        13 落 RCA 记录 → NATS ai.analysis.completed
                                   │
                                   ▼
                        14 策略匹配（冷却 + 频率限制 Redis）
                                    │ 按 execution_mode 分流
              ┌─────────────────────┼──────────────────────┐
              ▼ auto                ▼ approval             ▼ suggestion
        创建执行记录(running)   创建执行记录(pending_approval) 仅记录建议
              │                     │ 策略级审批（13 审批中心）
              └─────────────────────┘
                                    ▼ 审批通过
                        14 调用 13 任务创建 API
                        POST /api/v1/runs (graph_id=remediate-scenario)
                        参数：告警变量注入 + rca_record_id
                                    │
                                    ▼
                        ┌─────────────────────┐
                        │ aranea Remediate Graph│
                        │ 节点：取证 → fault_clear → 验证 → 回滚 │
                        │ Skill: gns3-remediate-runbook          │
                        │ 预算：取证≤2 → 第3次必须是 gns3.fault_clear │
                        │ 守卫：同工具同参数≥3次拦截，换词轮换检测   │
                        │ 确认：destructive 工具触发 interrupt   │
                        └──────────┬──────────┘
                                   │ MCP 工具调用
                                   │ gns3.fault_clear → 16 运维工具 → 设备
                                   ▼
                        Webhook 事件流：run.interrupted → run.node_end → run.completed
                                   │
                                   ▼
                        13 WebhookReceiver → NATS ai.task.events
                                   │
                                   ▼
                        14 TaskEventConsumer 推进状态机
                        success → 验证场景 → passed
                                   │
                                   ▼
                        发布 remediation.events
                        04 告警"已自动处理"回写 / 05 通知 / 07 可视化
                                   │
                                   ▼
                        沉淀：14 solution_library 置信度更新
                              10 知识库（knowledge.create）
                              aranea L3 记忆（POST /api/v1/memory/facts）
```

**关键时序约束**：
- RCA 分析默认超时 300s；remediate 图执行默认超时 600s（含 interrupt 等待时间）。
- 验证场景超时 120s，失败自动触发回滚场景（默认开启）。
- 14 FallbackPoller 对 running 状态记录兜底轮询 13 任务 API（30s 间隔），事件丢失时收敛。

### F2 语音机柜巡检 S1

```
用户语音："查看 A12 机柜的运行情况"
    │
    ▼
Voice Bridge(8140)：KWS 唤醒 → AEC 消回声 → sherpa-onnx ASR（热词注入 asset_aliases）
    │
    ▼
POST aranea "机房精灵" Spirit Agent（persona + L0 会话记忆）
    │
    ▼
Spirit TaskPlanner 判定：单工具任务 + scene_actions
    │
    ▼
MCP 调用：asset.get(cabinet="A12") + metric.query(指标集) + alarm.query(活动告警)
    │
    ▼
工具返回结构化结果 + scene_actions JSON：
{
  "cmd": "focus_entity",
  "entity": {"type": "cabinet", "id": "A12"},
  "camera": {"mode": "orbit_three_quarter", "duration": 1.2},
  "isolation": {"fade_others": 0.35},
  "ui": {"panel": "cabinet_detail", "labels": ["status", "metrics_top3"]},
  "say": "A12 机柜运行正常，32 台设备在线，当前功耗 4.2kW，最高温度 38℃，无活动告警"
}
    │
    ▼
aranea WS → Voice Bridge → 一路 TTS 播报（驱动 UE 精灵口型），一路 WS 转发 UE
    │
    ▼
UE5 执行：相机导演（UTwinCameraDirector）+ 隔离淡化（UTwinIsolationSubsystem）+ 面板展开
    │
    ▼
用户下一轮："它的 CPU 呢" → L0 会话记忆含聚焦实体栈 → 指代消解为 A12/SV-03 → 继续
```

**Skill 激活**：`cabinet-inspection-script` 在 Spirit 分解时识别触发词，生成标准 scene_actions 序列（overview → focus → detail → explode → inventory），约束 LLM 不自由发挥非白名单指令。

### F3 语音报告 S2

```
用户语音："把 SV-03 最近 30 天运行情况做个报告"
    │
    ▼
Spirit TaskPlanner 分解：
  1) 拉 30 天指标（metric.query）
  2) 拉告警统计（alarm.query）
  3) 拉修复记录（remediation.status）
  4) LLM 生成摘要段落
  5) ECharts HTML 渲染
    │
    ▼
aranea 并行执行步骤 1-3（Team 并行模式），聚合后 LLM 写 narrative
    │
    ▼
工具返回 report_generation_task_id（复用 06 既有表）
    │
    ▼
UE5 WebBrowser 内嵌展示报告全息卡片 + TTS 播报摘要
    │
    ▼
用户："把刚才的报告发我邮箱" → notify.send MCP 工具
```

### F4 智能告警 S3（三道防线）

```
04 告警规则命中 ──► 第一道：AI 预检（13 自动诊断）
    │                    只读诊断命令集经 MCP → 16
    │                    判定为疑似误报 → 降级为 info 或转入 ai_quarantine
    │                    Skill: alarm-triage-rules
    ▼
第二道：动态基线（04 既有）+ 维护窗口抑制（10 工单关联）
    ▼
第三道：告警风暴聚合（Redis 滑动窗口 + alarm_aggregation_groups）
    │    AI 增强：一个聚合组由 LLM 生成「事件叙述」
    │    "14:02 起机房 B 区网络抖动引发 23 台设备告警，根因疑似汇聚交换机 SW-B2 端口 flapping"
    ▼
05 通知只发一条事件叙述（而非 23 条）
```

### F5 告警追踪自愈可视化 S5

```
告警模式切换 / 手动追踪指令 ──► aranea "track_alarm" scene_actions
    │
    ▼
{
  "cmd": "track_alarm",
  "alarm_id": "ALM-2026-001",
  "camera": {"mode": "follow", "target": "SW-B2"},
  "ui": {"panel": "alarm_mode", "cards": ["related_alarms"]},
  "isolation": {"fade_others": 0.2}
}
    │
    ▼
UE5 告警模式控制器：相机自动追踪告警根因设备，多告警卡片呈环形排列，
修复完成后卡片自动消失并恢复 overview
```

### F6 定时巡检与洞察看板

```
13 定时任务（Cron）到点触发 ──► POST /api/v1/runs (graph_id=inspection-scenario)
    │
    ▼
aranea 巡检 Team：并行分配多个系统巡检 Agent 到不同资产集
    │
    ▼
MCP：server.top / server.disk_usage / server.service_status / metric.query
    │
    ▼
巡检结论汇总 → 生成 Markdown 报告 → 写入 10 知识库
    │
    ▼
13 洞察看板聚合：
  - 业务指标：MTTR / 自动修复成功率 / RCA 准确率 / 告警降噪率 / 知识库增长
  - aranea 运行指标：LLM 成本 / Token / Agent 成功率（经 /api/v1/quota/usage + /api/v1/metrics/agents）
```

---

## 5. 通信契约矩阵

### 5.1 REST 控制面（13 → aranea）

| 端点 | 方法 | 功能 | 状态 |
|------|------|------|------|
| `/api/v1/health` | GET | 健康探测 + 版本/Agent数/Graph数/Model数 | ✅ 已落地 |
| `/api/v1/agents` | GET/POST | Agent 清单 / 创建 | ✅ 已落地 |
| `/api/v1/agents/{id}` | PUT | 更新 Agent（种子同步用） | ✅ 已落地 |
| `/api/v1/graphs` | GET/POST | Graph 清单 / 创建 | ✅ 已落地 |
| `/api/v1/graphs/{id}` | GET/PUT | Graph 详情 / 更新 | ✅ 已落地 |
| `/api/v1/runs` | POST | 创建执行（graph_id + params + webhook_url + idempotency_key） | ✅ 已落地 |
| `/api/v1/runs/{id}` | GET | 执行状态查询（兜底轮询） | ✅ 已落地 |
| `/api/v1/runs/{id}/cancel` | POST | 取消执行 | ✅ 已落地 |
| `/api/v1/runs/{id}/interrupts/{interrupt_id}/resume` | POST | HITL 恢复执行（approval=true/false + reason） | ✅ 已落地 |
| `/api/v1/memory/facts` | POST | 写入 L3 事实（修复闭环沉淀） | ✅ 已落地 |
| `/api/v1/quota/usage` | GET | 配额与成本查询（洞察看板） | ✅ 已落地 |
| `/api/v1/metrics/agents` | GET | Agent 运行指标（洞察看板） | ✅ 已落地 |

鉴权：独立 Bearer token（`ARANEA_TWINOPENAPI_TOKEN`），与 aranea JWT 用户体系隔离。

### 5.2 Webhook 事件回写（aranea → 13）

| 事件类型 | 触发时机 | 13 处理 |
|----------|----------|---------|
| `run.created` | 执行创建成功 | 任务镜像创建（ai_tasks），状态 pending |
| `run.started` | 执行开始 | 任务镜像状态更新 running |
| `run.node_start` / `run.node_end` | 节点生命周期 | 节点镜像追加（ai_task_nodes），透传 Trace |
| `run.interrupted` | 遇到 requires_confirmation 工具 | 创建审批待办（ai_approvals），关联 interrupt_id |
| `run.completed` | 执行成功终态 | 任务镜像 success，触发 14 验证场景 |
| `run.failed` | 执行失败终态 | 任务镜像 failed，触发 14 回滚场景（默认开） |
| `run.cancelled` | 执行取消 | 任务镜像 cancelled |

签名：HMAC-SHA256（`X-Signature`），密钥取自 `ai_aranea_instances` 启用行的 `webhook_secret`（AES-256-GCM 加密落库）。幂等：`event_id` Redis SET NX 去重。

### 5.3 MCP 能力面（aranea → 13）

传输：SSE 双端点 `GET /mcp/sse` + `POST /mcp/messages?sessionId=`（MCP 2024-11-05 规范）。
鉴权：MCP 调用方凭据（`client_id` + `secret`），aranea 作为内部 Host 使用专用 client_id。

完整工具清单见附录 A。

### 5.4 NATS 事件主题

| 主题 | 方向 | 生产者 | 消费者 | 说明 |
|------|------|--------|--------|------|
| `alarm.events` | pub | 04 告警 | 13（RCA/自动诊断）、14（策略匹配） | 告警触发事件 |
| `ai.task.events` | pub | 13 WebhookReceiver | 14（状态机推进） | 任务状态变更 |
| `ai.approval.pending` | pub | 13 审批中心 | 05 通知 | 审批待办推送 |
| `ai.analysis.completed` | pub | 13 RCA 完成 | 14（策略关联）、05 通知 | RCA 分析完成 |
| `ai.aranea.health` | pub | 13 HealthProber | 07 可视化、TwinWeb | aranea 健康状态 |
| `remediation.events` | pub | 14 | 04（告警回写）、05（通知）、07（可视化） | 修复结果事件 |
| `remediation.approval.required` | pub | 14 | 05 通知 | 修复审批请求 |

---

## 6. 落地路线图

### 阶段 P0：解锁阻塞（当前第一优先级）

**目标**：aranea 在环端到端验证，打通「创建 Run → Webhook 回写 → 状态推进」最小闭环。

| 任务 | 验收标准 | 依赖 |
|------|----------|------|
| aranea 服务部署（Docker 或本地） | `GET /api/v1/health` 200，返回版本与资源计数 | 环境变量 `ARANEA_TWINOPENAPI_TOKEN` 双侧配置 |
| 13 连通性探测 | AraneaPage 探测按钮 green，健康横幅消失 | aiops 服务常驻 + aranea 端口可达 |
| 种子同步验证 | 12 预设 Agent 成功注册到 aranea，返回 agent_id 映射 | agent_preset.go 种子格式与 aranea Agent CRUD 对齐 |
| 最小 E2E（手动触发场景） | 在 13 场景模板页点击「立即执行」→ aranea Run 创建 → Webhook 回写 → 任务状态 success | webhook_url 路由可达、HMAC 签名验证通过 |
| 审批 interrupt 闭环 | 创建一个含 interrupt 的测试 Graph → 触发 interrupt → 13 审批中心通过 → Resume → 执行 success | interrupt_id 映射正确 |
| RCA 端到端 | 04 模拟告警 → 13 订阅触发 → aranea RCA Run → Webhook 回写 → RCA 记录落库 → 置信度显示 | alarm.events 路由正确 |

### 阶段 P1：MCP 工具扩展

| 任务 | 验收标准 |
|------|----------|
| 13 MCP-Server 注册新增 10 个工具（gns3 域 4 + network 域 3 + ops 域 3） | `GET /mcp/sse` tools/list 返回新增工具，风险等级正确（network.line_probe=low 非只读） |
| 13 `ai_mcp_tools` 表 upsert 幂等 | 重启服务不重复创建，治理调整字段保留 |
| aranea 侧登记 twinmonitor MCP-Server | `mcp_servers` 表新增记录，MCPVersionHash 正确计算 |
| gns3 工具后端实现（转发 GNS3 控制器，非 opstools 生产通道） | `gns3.exec` 经 gns3_agent 在仿真环境执行成功，返回 stdout/stderr；审计记录含目标平面标记 `plane=gns3_sim` |
| 新增工具冒烟测试 | McpTesterPage 可调用新增工具，返回结果符合预期 |

### 阶段 P2：双跑切换

| 任务 | 验收标准 |
|------|----------|
| 12 预设 Agent tool_whitelist 全面切 MCP | Agent 在线测试时工具调用走 MCP 通道，内置 twinops 无调用记录 |
| remediate 图节点工具切换 | verify/取证/fault_clear 节点调用 MCP 工具，budget 规则不变形 |
| 技能 4 个上线并完成路由配置 | Skill 路由测试：命中触发词后注入的 guidance cue 含 Skill 内容（relay 抓包验证） |
| 循环守卫与 budget 规则验证 | test/ts10-gns3 扩展 E2E：同工具同参数 3 次触发拦截，第 3 次 fault_clear 前强制 fault_clear |
| 双层审批验证 | destructive 工具触发 interrupt → 13 审批中心 → Resume → 执行完成 |
| 策略预授权验证 | auto 策略关联 destructive 场景：未预授权禁止启用；预授权后执行全程无 interrupt；授权过期自动回落为逐次确认 |
| L3 记忆写入验证 | 修复闭环终点调用 `POST /api/v1/memory/facts` 成功， facts 表可查 |

### 阶段 P3：内置工具退役

| 任务 | 验收标准 |
|------|----------|
| `builtin_tools_seed.go` 移除 twinops 17 工具 | 新环境 seed 后 tools 表无 twinops 工具 |
| reseed DDL 迁移 | `ddl_migration_registry.go` 新增版本，幂等重跑安全 |
| `internal/tools/twinops/` 目录标记废弃（保留 1 个迭代后删除） | 代码注释标记 deprecated，构建无引用警告 |
| 全量回归测试 | `go test ./...` + `go build ./cmd/... ./internal/...` 通过 |

### 阶段 P4：全场景 E2E（语音/UE5）

| 任务 | 验收标准 |
|------|----------|
| Voice Bridge → aranea → UE5 链路打通 | KWS 唤醒后语音指令 2s 内触发 UE 相机动作 |
| S1 语音巡检完整链路 | "查看 A12 机柜" → focus_entity → cabinet_detail → hardware_explode → 口型同步 |
| S2 语音报告生成 | "生成 SV-03 30 天报告" → 数据聚合 → 报告展示 → "发我邮箱" → 通知送达 |
| S5 告警追踪可视化 | 告警模式切换 → UE 自动追踪 → 修复完成 → 卡片消失 |

---

## 7. 对既有文档的修订点清单

总纲不直接修改既有文档，以下是需要在各模块文档下一版本中同步的修订点：

| 文档 | 章节 | 修订内容 | 优先级 |
|------|------|----------|--------|
| TwinMonitor-AI融合总体设计方案 v1.2 | §0 D1 | 补充 MCP 单通道收敛决策（原 D1 只提编排委托，未提工具通道收敛） | 高 |
| 13-AI智能运维需求文档 v2.1 | §5.9 MCP | 扩展 gns3/network/ops 三域工具（原 24 个 → 34 个），补充 destructive 工具与 aranea interrupt 的联动规则及 GNS3 平面定位 | 高 |
| 13-AI智能运维概要设计 v2.1 | §6.1/6.2 | `ai_mcp_tools` 表新增 gns3/network/ops 三域条目；`ai_mcp_call_history` 补充拦截原因与目标平面（plane）字段；新增对账/卡死清扫 worker 设计 | 高 |
| 14-自动修复与自愈详设 v2.0 | §5 | 补充策略-工具风险联动规则（auto+destructive 需策略预授权）、aranea 不可用降级矩阵、F7 基础设施自愈旁路 | 高 |
| 13-AI智能运维详设 v2.1 | §2.2.6 MCP | 新增 gns3 工具后端实现章节（转发 GNS3 控制器 gns3_agent，与 opstools 生产通道隔离） | 高 |
| 14-自动修复与自愈需求文档 v3.0 | §2.3 | 外部依赖接口契约中「场景执行引擎」明确为 aranea Graph，补充 MCP 工具调用路径 | 中 |
| 14-自动修复与自愈概设 v2.0 | §4 | 架构图中「aranea」与「TwinMonitor MCP-Server」之间增加 MCP 标注 | 中 |
| 19-语音精灵交互 | §3 | S1 场景指令集中补充 `track_alarm` / `alarm_mode`（S5 新增指令） | 中 |
| aranea-agents project_rules.md | — | 新增「MCP 单通道」架构决策 ADR 引用 | 低 |

---

## 8. 附录

### A. 工具映射全表（aranea 内置 twinops → MCP）

| # | 原 aranea 工具 | 新 MCP 工具 | 领域 | 风险 | 只读 | 后端实现（13 内部转发） |
|---|----------------|-------------|------|------|------|------------------------|
| 1 | `twin_alarm_query` | `alarm.query` | alarm | readonly | ✅ | 04 告警查询 API |
| 2 | `twin_alarm_get` | `alarm.get` | alarm | readonly | ✅ | 04 告警详情 API |
| 3 | `twin_alarm_ack` | `alarm.acknowledge` | alarm | low | ❌ | 04 告警确认 API |
| 4 | `twin_line_status` | `network.line_status` | network | readonly | ✅ | 08 线路监控 API |
| 5 | `twin_line_events` | `network.line_events` | network | readonly | ✅ | 08 线路事件 API |
| 6 | `twin_line_probe` | `network.line_probe` | network | low | ❌ | 08 线路探测 API（主动探测，非只读） |
| 7 | `twin_device_search` | `asset.get` + 参数扩展 | asset | readonly | ✅ | 02 资产查询 API |
| 8 | `twin_device_get` | `asset.get` | asset | readonly | ✅ | 02 资产详情 API |
| 9 | `twin_device_metrics` | `metric.query` | metric | readonly | ✅ | 06 指标查询 API |
| 10 | `twin_remediation_status` | `ops.remediation_status` | ops | readonly | ✅ | 14 执行记录查询 API |
| 11 | `twin_alarm_rule_get` | `alarm.query` 参数扩展 | alarm | readonly | ✅ | 04 告警规则 API |
| 12 | `twin_collector_status` | `ops.collector_status` | ops | readonly | ✅ | 03 采集状态 API |
| 13 | `twin_inspection_query` | `ops.inspection_query` | ops | readonly | ✅ | 16 巡检记录 API |
| 14 | `gns3_health_check` | `gns3.health_check` | gns3 | readonly | ✅ | GNS3 控制器健康检查（gns3_agent） |
| 15 | `gns3_exec` | `gns3.exec` | gns3 | high | ❌ | GNS3 仿真设备命令执行（gns3_agent console） |
| 16 | `gns3_fault_inject` | `gns3.fault_inject` | gns3 | destructive | ❌ | GNS3 仿真故障注入（gns3_agent） |
| 17 | `gns3_fault_clear` | `gns3.fault_clear` | gns3 | destructive | ❌ | GNS3 仿真故障清除（gns3_agent） |

> 原 aranea `twin_alarm_query` 与 MCP `alarm.query` 语义不完全一致时，13 侧 `mcp_call.go` 做参数适配。

### B. 事件主题表

| 主题 | 持久化 | 消费者模式 | 备注 |
|------|--------|-----------|------|
| `alarm.events` | JetStream, durable | Queue Group (04→13/14) | 含 Redis SET NX 去重 |
| `ai.task.events` | JetStream, durable | Queue Group (13→14) | 14 多实例时 NATS Queue Group 去重 |
| `ai.approval.pending` | 不持久 | Pub-Sub | 实时推送 05 通知 |
| `ai.analysis.completed` | JetStream, durable | Queue Group | RCA 结果关联 |
| `ai.aranea.health` | 不持久 | Pub-Sub | 60s 周期探测 |
| `remediation.events` | JetStream, durable | Queue Group | 修复结果多播 |
| `remediation.approval.required` | 不持久 | Pub-Sub | 审批待办推送 |

### C. REST 端点契约表（13 AraneaClient ↔ aranea twin_openapi_compat）

| 端点 | 方法 | 请求体关键字段 | 响应体关键字段 | aranea 内部映射 |
|------|------|---------------|---------------|-----------------|
| `/api/v1/health` | GET | — | status, version, agent_count, graph_count, model_count | monitorUsecase.Health |
| `/api/v1/agents` | GET | — | items[{id,name,description,definition}] | agentUsecase.List |
| `/api/v1/agents` | POST | name, definition{system_prompt,tool_whitelist,model_policy} | id | agentUsecase.Create |
| `/api/v1/agents/{id}` | PUT | name, definition | — | agentUsecase.Update |
| `/api/v1/graphs` | GET | — | items[{id,name,definition}] | graphUsecase.List |
| `/api/v1/graphs` | POST | name, definition{nodes,edges} | id | graphUsecase.Create |
| `/api/v1/graphs/{id}` | GET | — | id, name, definition | graphUsecase.Get |
| `/api/v1/graphs/{id}` | PUT | name, definition | — | graphUsecase.Update |
| `/api/v1/runs` | POST | graph_id, agent_id, params, webhook_url, webhook_secret, idempotency_key | id, status | graphExecUsecase.CreateRun |
| `/api/v1/runs/{id}` | GET | — | id, status, nodes[], output | graphExecUsecase.GetRun |
| `/api/v1/runs/{id}/cancel` | POST | — | status | graphExecUsecase.Cancel |
| `/api/v1/runs/{id}/interrupts/{iid}/resume` | POST | approval(bool), reason | status | sessionUsecase.ResumeInterrupt |
| `/api/v1/memory/facts` | POST | fact_id, type, content, scope_type, scope_id, metadata | — | memoryAdminUsecase.WriteFact |
| `/api/v1/quota/usage` | GET | — | tokens, cost, requests | usageUsecase.GetQuota |
| `/api/v1/metrics/agents` | GET | — | items[{agent_id,success_rate,avg_latency}] | monitorUsecase.AgentMetrics |

### D. Skill 清单与触发词

| Skill ID | 名称 | 绑定 Agent | 触发词 | 版本 |
|----------|------|-----------|--------|------|
| `skill-gns3-remediate-v1` | gns3-remediate-runbook | 故障诊断 Agent、变更执行 Agent | "故障自愈"、"自动修复"、"remediate" | 1.0.0 |
| `skill-rca-evidence-v1` | rca-evidence-path | 故障诊断 Agent | "根因分析"、"RCA"、"为什么告警" | 1.0.0 |
| `skill-cabinet-inspection-v1` | cabinet-inspection-script | 系统巡检 Agent | "巡检"、"inspect cabinet"、"机柜状态" | 1.0.0 |
| `skill-alarm-triage-v1` | alarm-triage-rules | 告警处理 Agent | "告警分级"、"误报检测"、"告警风暴" | 1.0.0 |

### E. 记忆 L0~L4 运维映射表

| 层级 | 运维语义 | 存储位置 | 作用域 | 写入 API | 召回 API |
|------|----------|----------|--------|----------|----------|
| L0 | 语音多轮指代消解 | Voice Bridge 内存 / WS 会话 | session | Voice Bridge 上下文注入 | 随 WS message 提交 |
| L1 | Run 内中间产物 | Run checkpoint / 节点输出 | run | Graph 自动累积 | 同 Run LLM 上下文 |
| L2 | 跨 Run 会话上下文 | session 存储 | session | `session.append_event` | `UpdateSessionContextFromLLMUsage` |
| L3 | 运维经验事实 | PG facts 表 | **agent**（修正后） | `POST /api/v1/memory/facts` | `memory.search`（fact_id） |
| L4 | 结构性知识索引 | PG knowledge_chunks | agent/team | `knowledge_write` / `knowledge.create` MCP | `knowledge.search`（embedding+tsvector） |

### F. v1.2 勘误表（以代码为准）

实施计划撰写期间逐行核验代码，以下总纲表述与代码不一致，**以代码/实施计划为准**：

| # | 总纲位置 | 原表述 | 代码事实（勘误后） | 证据 |
|---|----------|--------|---------------------|------|
| E1 | §5.2 Webhook 事件表 | `run.interrupted` | 实际事件名为 **`run.waiting_approval`**（常量 `AraneaEventRunWaitingApproval`） | aiops biz/webhook.go |
| E2 | §3.4 L3 payload 示例 | `fact_id` 字段 | aranea `FactUpsert` 无 FactID 字段，业务键经 **`Fingerprint`** 承载（`handleWriteMemoryFact` 把 `key` 写入 Fingerprint） | aranea twin_openapi_compat.go |
| E3 | 附录 A / §3.1.1 | 12 预设 Agent 白名单已对应合法 MCP 键 | `agent_preset.go` 含 11 个注册表不存在的**幽灵键**（`alarm.detail`/`asset.detail`/`server.top`/`server.disk_usage` 等），P2 计划按注册表真实键（`server.process_list`/`server.exec_command` 等）修正 | aiops biz/agent_preset.go × mcp_registry.go |
| E4 | §2.1 架构图 | `alarm.severity_assess` 已改本地规则 | 该工具仍在 MCP 注册表（保留供其他调用方），但已从告警处理 Agent 白名单移除；是否下线工具本身留 P3 裁定 | aiops biz/mcp_registry.go |
| E5 | §3.1 工具命名 | MCP 工具名点分风格 `gns3.exec` | aranea `NamedToolSet` 装配后以 `_` 拼接前缀（如 `twinmonitor_gns3.exec`），effective_tools 配置前须先查 tools/list 实际返回名 | aranea internal/tools/mcp |
| E6 | §3.3 Skill 库路径 | `internal/skill-library/twinmonitor/` | aranea 运行时技能根由 `internal/skill/storage/root.go` 解析（默认 `~/.config/aranea/skills`）；代码库维护 SKILL.md + Import API 导入运行时（P4 采用此弥合方式） | aranea internal/skill/storage |
| E7 | §4 F2 | 「机房精灵」属 12 预设 Agent | 不在 `agent_preset.go` 种子内；voice 模块经配置 `agent_key: sprite` 动态引用，调用走 `POST /v1/sessions` + `WS /v1/ws` | twinmonitor voice 模块 |
| E8 | §4 F2 scene_actions 示例 | `focus_cabinet`/`focus_server` | 白名单实际为 `focus_entity`（entity.type 区分 cabinet/server）+ `view_server` 等，以 19 详设/代码为准 | 19 语音精灵详设 |
| E9 | §4 F2 ASR 判停 | `end_window_size=800/force_to_speech_time=0` | 该配方属 aranea 侧 volcengine ASR；Voice Bridge 走 sherpa-onnx（`EnableEndpoint:1`+`silence_eof_ms:8000`），两条链路判停机制不同 | aranea internal/data/speech / voice 模块 |
| E10 | §4 F3 | 报告任务表 | 06 报告物理表为 `report_generation_tasks`（twinmonitor_log 库）；通知送达证据落 `notice_records`（05） | twinmonitor 06/05 |
| E11 | §3.5 知识沉淀 | 「13 侧知识沉淀联动」自动闭环 | `DepositKnowledge` 在 RcaUsecase 中且为**手动触发**；P5 计划根治为 14 终态自动调用 | aiops biz/rca.go |
| E12 | §6 P0 | llm 抓包先例 `test/ts10-gns3/llm_relay.py` | 该文件在 aranea 仓库不存在（项目记忆中的路径已过期）；P4 新建 `skill_routing_verify.py` 承担同类职责 | aranea test/ 目录 |

另：实施计划主动补全的代码新增点（非错误）：13 `AraneaRuntimeStore` 需追加 `SetDegraded/IsDegraded`；`GraphDefinition` 需追加 budget 字段（或走 definition JSON 内部字段）；14 `ManualProcessUsecase` 需注入 degraded 状态源（跨域经 Redis 键读取，避免直接引用 13 域接口）；13 `configs/config.yaml` 需新增 `gns3agent` 客户端配置；`ai_mcp_call_history.plane` 取值三级（`gns3_sim`/`readonly`/`production`）。

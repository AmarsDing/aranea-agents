# 52 FlowLogger Review

> **评分**：79 / 100 | **风险等级**：P1  
> **文档**：[52-flow-logger.md](../需求/52-flow-logger.md) · [52-flow-logger.design.md](../需求/52-flow-logger.design.md) · [52-flow-logger-development.md](../需求/52-flow-logger-development.md)  
> **代码锚点**：`internal/biz/flow_log.go` · `internal/event/` TraceEmitter · `internal/data/flow_log_repo.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | Phase 1a/1b/3 已完成；Phase 2（落库 + HTTP 历史查询 `ListFlowLogs`）是最大缺口 |
| 架构一致性 | 21 | 25 | SlogBridge 已移除 ✅；TraceEmitter 路径清晰；FlowLog 经 EventBus WS channel:monitor 推送 |
| 后端实现质量 | 17 | 20 | TraceEmitter、步骤注册表、trace_id、severity 均已落地；落库路径待补 |
| 前端实现质量 | 13 | 15 | Monitor Logs 双 Tab（流程/进程）已实现；LogStreamPanel 共享单 WS；历史查询 UI 待补 |
| 测试与验证 | 6 | 10 | TraceEmitter 单测；FlowLog 落库路径无测试 |
| 文档一致性 | 6 | 10 | 三件套文档存在（`52-flow-logger*.md`）但命名使用连字符而非空格，与其他模块命名风格不一致 |

---

## 模块定位

FlowLogger（Flow Log v2）负责将 Agent 运行过程中的步骤事件（chat.turn、tool.call、memory.read 等）以结构化日志形式记录，支持实时 WS 推送给 Monitor 页面以及持久化历史查询。

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| TraceEmitter（步骤 ID、trace_id、severity） | ✅ Phase 1a |
| SlogBridge 移除 | ✅ Phase 1b |
| Team TraceEmitter（`team.run.*`） | ✅ Phase 3 |
| Rerank fallback → `knowledge.rerank.fallback` | ✅ Phase 3 |
| EventBus 失败 `SessionSysLogError` | ✅ Phase 3 |
| `chat.turn.enter` 步骤 ID 对齐 | ✅ Phase 3 |
| Monitor Logs 流程/进程双 Tab | ✅ |
| WS enable_log 修复 | ✅ |
| `ListFlowLogs` HTTP 查询 | ❌ Phase 2 未实现 |
| FlowLog 落库 | ❌ Phase 2 未实现 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| FL-P1-01 | **Phase 2 落库缺失**：`ListFlowLogs` HTTP 历史查询接口和 `flow_log_repo` 落库未实现；Monitor Logs 只能看实时流，无法查历史 | 实现 `flow_log_repo.go` + `ListFlowLogs` RPC；在 Ent schema 加 `flow_logs` 表 |
| FL-P1-02 | 文档命名用 `52-flow-logger*`，与 `需求/NN <name>.md` 标准风格不符 | 标注为已知命名例外，或在文档规范中说明 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| FL-P2-01 | 步骤注册表（§5.1）与实际 TraceEmitter 调用点的对齐需要定期维护 | 加注册表自检测试（每个已注册 step 都有对应 Emit 调用） |
| FL-P2-02 | 无 FlowLog 存储层测试 | 实现落库后补单测 |

---

## 步骤注册表状态

| 已注册步骤 | 状态 |
|-----------|------|
| `chat.turn.enter` / `.complete` / `.error` | ✅ |
| `tool.call` / `.result` / `.error` | ✅ |
| `team.run.*` | ✅ Phase 3 |
| `knowledge.rerank.fallback` | ✅ Phase 3 |
| `system.agent.build` | ✅ |
| Memory L0–L4 步骤 | 🟡 部分 |

---

## 建议优化路径

1. **高优**：实现 Phase 2 落库（`flow_logs` Ent schema + `flow_log_repo` + `ListFlowLogs` RPC + HTTP 路由）。
2. 补充步骤注册表与 TraceEmitter 调用点的对齐测试。
3. 前端 Monitor Logs 历史查询 UI（依赖 Phase 2 后端）。

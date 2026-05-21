# 00 系统架构 Review

> **评分**：82 / 100 | **风险等级**：P1  
> **文档**：[0 系统框图.md](../需求/0%20系统框图.md) · [0-system-development.md](../需求/0-system-development.md) · [execution-plan.md](../guides/execution-plan.md)  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 18 | 20 | 系统框图清晰，分层架构、依赖方向、运行时边界均有文档化；五面"乐高积木"定义完整 |
| 架构一致性 | 23 | 25 | Kratos v2 + tRPC-Agent 分工明确；主链路（Chat/Team/Graph）经 RunRegistry + EventBus 统一；少量债务（PendingQueue 在 Service、Data 绑定 trpc runtime） |
| 后端实现质量 | 18 | 20 | 核心三层（service/biz/data）职责清晰；RunRegistry、RunGateway 已落地；Wire DI 完整 |
| 前端实现质量 | 11 | 15 | `/v1/ws` 单连接多路复用已实现；前端各域分层不均匀，store/API 双轨共存 |
| 测试与验证 | 6 | 10 | `make runtime-boundary` 脚本存在；单测覆盖核心路径；E2E 缺失 |
| 文档一致性 | 6 | 10 | 系统框图 + 开发计划 + execution-plan 三文档对齐；部分模块文档滞后；命名规范不统一 |

---

## 架构概述

Aranea-Agents 采用 Kratos v2（传输/DI）+ pkg/trpc-agent-go（Agent 运行时）双框架。

```
接入层 (Web/CLI/Channel/Cron/A2A)
    ↓ HTTP/WS
传输层 (internal/server — 注册 only)
    ↓ proto service 接口
服务桥接层 (internal/service — Runner 组装 + 事件投影)
    ↓ biz 接口
领域层 (internal/biz — Usecase + Repo 接口)
    ↓ Repo 实现
数据层 (internal/data — Ent SQLite + pgvector)
        ↕ (运行时适配层)
internal/agent / internal/team / internal/tools
    → pkg/trpc-agent-go Runner / Agent / Session / Tool / Event
```

### 分层健康度

| 边界 | 状态 | 问题 |
|------|------|------|
| server → service | ✅ 健康 | `skill_import_http.go` 有路由旁路，但未直调 Runner |
| service → biz | ✅ 基本健康 | `PendingMessageQueue` 实现留在 Service，应下沉 |
| biz 不 import trpc-agent-go | ⚠️ 待确认 | `biz/memory_runtime_set.go` 需 `make runtime-boundary` 验证 |
| data 不绑 trpc runtime | 🟡 中等 | `data.go` 绑定 trpc session/graph checkpoint — 计划上移 |
| internal/agent/team/tools → biz | ✅ 可接受 | Builder 需领域配置，经 Catalog DTO 稳定依赖 |
| web 分层 | 🟡 中等 | 多页面直连 API；store/API 双轨 |

---

## 已验收优势

1. **RunRegistry + RunnerManager + RunGateway**：会话级运行控制（enqueue/cancel/status/artifact/ingest）已统一落地，Chat/Team/Cron/Channel 共用入口。
2. **Envelope 协议**：31 种 EnvelopeType，单 WS 连接多路复用，背压三级策略。
3. **Wire DI**：编译期依赖注入，依赖图清晰，无运行期反射。
4. **架构文档**：`0 系统框图.md` + `0-system-development.md` + `execution-plan.md` 三文档提供稳定真相源。
5. **EventBus SRP**：buffer/runner/state 三 handler 已拆分，EventProjector 独立。

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| SYS-P1-01 | `PendingMessageQueue` 实现留在 `internal/service/chat_pending.go`，Service 层承担过多状态机 | 将 Queue 实现下沉到 `internal/runtime` 或 `biz.ChatUsecase` |
| SYS-P1-02 | `internal/data.go` 绑定 trpc session/graph checkpoint — Data 层测试受运行时影响 | provider 上移到 Wire 阶段组装 |
| SYS-P1-03 | `biz <-> provider` 双向依赖（llm inspect 与模型目录边界不稳） | 抽 `internal/llminspect` 端口或 biz 接口隔离 |
| SYS-P1-04 | 前端 store/API 双轨：`SystemSettingsPage`、`EcosystemPage`、`MonitorPage` 等页面直连 `features/*/api` | 统一 feature 模板，page → composable/store → feature API |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| SYS-P2-01 | Ecosystem、TTS、CLI 三个模块文档有目标但代码几乎无后端闭环 | 标注为 API-only 或 UI-mock，不计入"完成"状态 |
| SYS-P2-02 | 模块文档命名不统一（空格/点号/连字符混用），部分缺少三件套 | 制定文档命名规范并在 README-development.md 公示 |
| SYS-P2-03 | `make runtime-boundary` 是否纳入 CI/CD 自动检查未明确 | 在 Makefile + CI 中强制执行 |

---

## 五面完整度评价

| 面 | 状态 | 说明 |
|----|------|------|
| Contract (proto) | ✅ 27 个 proto service | 唯一对外契约，生成客户端与服务端 |
| Domain (biz) | ✅ 完整 Usecase + Repo 接口 | 领域规则清晰 |
| Runtime (trpc-agent-go) | ✅ agent/team/tools 均通过 pkg 接口集成 | 未复制内部实现 |
| Persistence (data) | ✅ Ent + SQLite；pgvector 可选 | Data 层有轻度 runtime 耦合需修复 |
| UI/Operate | 🟡 核心页面存在，分层不均 | 前端 store 策略需统一 |

---

## 建议优化路径

1. **立即**：运行 `make runtime-boundary`，确认 `biz/memory_runtime_set.go` 边界。
2. **本迭代**：将 `PendingMessageQueue` 实现从 Service 下沉到 `internal/runtime`。
3. **下迭代**：统一前端 feature 模板，消除 page 直连 API 漂移。
4. **长期**：抽 `internal/llminspect` 消除 biz-provider 双向依赖。

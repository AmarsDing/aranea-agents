# 变更记录（changelog）

> 本目录存放项目各阶段的变更摘要。changelog 是**只读历史**，写完不可改；如有新发现需修正，写新 devlog 补丁，不去改旧 changelog。
>
> ⚠️ 自 2026-05-17 起，**新增**变更记录请同时在 `execution-plan.md` §1 / §3 / 附录 A 增改对应行；只写 changelog 不更新 execution-plan 视为未完成。

---

| 文档 | 说明 |
|------|------|
| [2026-05-12-Provider.md](./2026-05-12-Provider.md) | ADK → trpc-agent-go 迁移 + 多 Provider 支持 |
| [2026-05-13-Session.md](./2026-05-13-Session.md) | Session 核心数据结构重构 |
| [2026-05-16-Graph.md](./2026-05-16-Graph.md) | Graph 工作流完善（校验引擎 + 模板 + 全链路） |
| [2026-05-16-Session-Optimize.md](./2026-05-16-Session-Optimize.md) | Session 模块优化（通用更新 / 恢复 / 分页 / 排序 / 过滤） |
| [2026-05-17-Session-Turns.md](./2026-05-17-Session-Turns.md) | Session Turns 编排追踪 + Detail 页 + Restore / Archive |
| [2026-05-17-S1-Hardening.md](./2026-05-17-S1-Hardening.md) | S1 P0 红线加固：单连接池 / WS 接入 / biz 解耦 / 内存缓存修复 / 并发安全 / EventBus 可靠投递 |
| [2026-05-17-S2-Architecture.md](./2026-05-17-S2-Architecture.md) | S2 架构债清理：runtime 包重构 / EventBus 背压 / Agent 缓存 LRU / Pinia store / axios 拦截器 / 统一 WS 客户端 |
| [2026-05-17-S3-Observability.md](./2026-05-17-S3-Observability.md) | S3 业务可观测：RunStatus RPC / Callback Chain / apierror / Workspace 中间件 / Prometheus metrics / lint 工具 / CI / 测试基线 |
| [2026-05-17-S4-Plugin-Skill-Planner.md](./2026-05-17-S4-Plugin-Skill-Planner.md) | S4 功能补全：Plugin 运行时 / Skill DB 仓库 / Planner 多策略 / Memory 工具注入 / AutoMemory 后台任务 |
| [2026-05-17-S5-Artifact-Cron-Tests.md](./2026-05-17-S5-Artifact-Cron-Tests.md) | S5：Artifact 制品 / Cron 重试 DLQ / AgentRuntimeSettings 拆分 / 测试矩阵 60% |
| [2026-05-17-S6-Knowledge-Eval-A2A.md](./2026-05-17-S6-Knowledge-Eval-A2A.md) | S6：Knowledge / Evaluation / A2A / CodeExecutor 沙箱（注：四项均未端到端接入 Agent 装配链，详见 `execution-plan.md` §1.3） |
| [2026-05-17-batch2-EP-RT07-BIZ02-FE.md](./2026-05-17-batch2-EP-RT07-BIZ02-FE.md) | 二批收尾：EP-RT-07 Cron 走 plugin runtime / EP-BIZ-02 Docker executor backend selector / EP-BIZ-05/07 渠道禁用与进化降级 / EP-FE-03/05/06 设计 token / EP-RULE-01/03 红线补充 |

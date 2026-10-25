# 87-orchestration-chain-hardening 需求文档

> 设计文档：[87-orchestration-chain-hardening.design.md](./87-orchestration-chain-hardening.design.md) · 开发计划：[87-orchestration-chain-hardening.development.md](./87-orchestration-chain-hardening.development.md)
> 审查依据：[docs/reports/2026-10-20-review-spirit-orchestration-chain.md](../reports/2026-10-20-review-spirit-orchestration-chain.md)
> 本文档只描述用户视角的需求与验收标准，不含架构设计。

## 1. 背景

精灵会话分配任务后，经历「分档 → 组队 → 经验复用 → DAG/graph 执行 → 状态上报 → UI 显示」全链路。2026-10-20 三轮对抗审查确认 25 项缺陷、8 项疑似风险，集中在：取消语义失真、并发互斥盲区、经验缓存契约失守、前端状态显示错误。本专项修复全部 33 项，不改变架构边界。

## 2. 用户故事

- **US-1（取消）**：作为用户，我在团队会话点「停止」时，该团队应被记为「已取消」——不显示为失败、不拖垮依赖它的下游团队、不生成「失败总结」、更不允许几秒后被自动重跑。
- **US-2（单跑）**：作为用户，无论我向团队会话发消息、还是后台编排自动启动团队，同一团队永远只有一个执行实例。
- **US-3（总结不丢）**：作为用户，所有团队完成后的汇总报告即使第一次生成失败，也会自动补上，不需要我再发问触发。
- **US-4（经验可信）**：作为用户，系统复用历史团队配方越用越准：失败会降权；剧本换约、组织变更、换工作区后旧配方不会误用；别的租户的经验不会串到我这里。
- **US-5（并行可信）**：作为用户，DAG 里的健康团队不会因并发竞态被判失败，整板不会被误杀；并发团队数不超配置上限。
- **US-6（暂停/恢复连续）**：作为用户，图执行暂停再恢复后，观察视图、遥测、数据库状态保持一致，不出现 UI 冻结或状态矛盾。
- **US-7（显示真实）**：作为用户，界面上每个 agent、每个团队、每个图节点的状态都与后端一致：失败显示失败、取消显示取消、部分失败显示部分失败、完成后永不退回"运行中"。

## 3. 功能需求清单

> 编号映射：C*=审查报告 §2 confirmed；P*=§3 plausible。

### US-1 取消语义可信

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-01 | C1 | 三处团队错误回调统一取消判别（registry/err 链/ctx 三源） | 取消的团队 `teams.status=cancelled`；单测：cancelled/err 三源各为真时入参 status=cancelled |
| REQ-02 | C1 | 失败分类器把用户取消判为 terminal-cancel | 取消的 PlanStep 不自动重试；下游按 skip 收敛非 fail；`PlanStepFailedEvent` 不携带取消语义 |
| REQ-03 | C1 | 取消路径终态落账不被取消上下文传染 | 终态写库在 `context.WithoutCancel` 上执行；取消的团队其 TeamStage/TeamRun/MemberSession 均收敛 cancelled 终态 |
| REQ-04 | C1 | 取消不再触发失败叙事 synthesis | `CancelledTeams>0` 时跳过总结报告；已取消团队不出现在 `ListFailedTeamBriefs` |
| REQ-05 | C4 | 终态守卫补 `partial_failure`；`recordTeamCompletion`/`scheduleDependentTeams` 以 CAS 转移成功为前置 | 同状态 completed→completed 重放：orcCache 计数/交付物落库/演化建议均不重复（单测计数器=0） |
| REQ-06 | P5 | cancelled 分支对未转换的团队补 `TransitionStatus(cancelled)` | 从 pending/running 取消均可正确落 cancelled；对拒绝（invalid/conflict）容忍 Warn 且幂等 |
| REQ-07 | C5 | 新增 `EventKindTeamRunCancelled` 并贯通持久化/回放/前端路由 | CriticalJournal 回放可区分取消与失败；Monitor 事件流不再把取消渲染为 warn 级失败 |
| REQ-08 | P1 | poller/lease 释放两条 synthesis 触发路径注入 `RootTaskActivityID` | synthesis 不再产生"触发文本落成新用户气泡"；父 Task 正常关闭不永久 Running |

### US-2 团队单跑互斥

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-09 | C2 | `StartTeamTurn` 接入运行注册+会话锁，与 chat 入口双方向互可见 | 并发 chat turn + 调度启动：仅一方获得执行；`RunTurnFromInput` 调用计数=1 |
| REQ-10 | C2 | `TransitionStatus(Running)` 失败必须中止而非 Warn 继续 | 转换失败时后续团队执行不启动，有明确日志 |

### US-3 synthesis 生命周期

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-11 | C3 | 兜底轮询能复活「全部团队终态但未出 synthesis」的 interrupted 会话 | synthesis 失败 30s 内自动重试（有重试预算与持久化守卫防无限循环）；报告最终可产出 |
| REQ-12 | P1 | `synthesisAlreadyProduced` 读失败 fail-closed 化 | 读失败不重复产出总结 |

### US-4 经验复用可信

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-13 | C8 | DQ 双向：失败降权 + synthesis 报错型失败纳入学习 | 连续失败的配方 DQ 下降；降至阈值后不再被命中 |
| REQ-14 | C9 | `ConstraintFingerprint`/`PlaybookID` 真正写入并在全部消费点校验 | 剧本换约后旧配方不再命中（fail-closed）；旧库条目指纹空按一次性降级处理不强淘汰 |
| REQ-15 | C10 | 读侧归一化 + 「其他」域禁读写 | "创作 / 文学"与"创作/文学"命中同一配方；「其他」域永不写入永不命中 |
| REQ-16 | C11 | 缓存持久化分行 + 版本 CAS，杜绝整 blob 覆盖 | 两个并发写互不丢更新；旧行一次性导入不丢数据 |
| REQ-17 | C12 | 经验复用对操作者可见 | 命中路径产出可见性信号（事件或日志结构化字段），不再只有日志 |
| REQ-18 | C13 | 配方成员收缩时有效 DQ 重算 + 同分确定性 | 成员缺失的配方返回折减后的分数；同分顺序确定性 |
| REQ-19 | C14 | 配方按 workspace 隔离 | 跨工作区同文本任务不命中他人配方与 agent_keys |

### US-5 DAG 并发与配额

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-20 | C15 | check-and-dispatch 原子化 | 并发完成最后依赖时，健康步骤不会被覆盖为 Failed；板不被误杀（并发单测） |
| REQ-21 | C17 | `r.canceled` 写入持锁 | `go test -race` 相关用例不报 race |
| REQ-22 | C7 | DAG 扇出受并行配额闸门 | 扇形 DAG 并发团队数 `≤ max_concurrent_teams`；超限时延后准入且不死锁 |

### US-6 Graph 运行时一致性

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-23 | C16 | interrupt 不再误触发 onComplete | resume 后投影器继续工作、trace 未提前 Finish（单测：onComplete 调用计数） |
| REQ-24 | P4 | streamGen 守卫对称覆盖 GraphDone/NodeStart/NodeEnd | 旧代际事件不再写新世代 steps/state（注入旧 gen 事件单测） |
| REQ-25 | C17 | 终态先落库后外发；standalone 执行有启动终态化扫描 | 崩溃后 standalone graph_executions 不永久卡 running |

### US-7 前端状态显示真实

| REQ | 关联 | 需求 | 验收标准 |
|-----|------|------|---------|
| REQ-26 | C19 | run_status 新鲜度守卫（runId 绑定+终态粘性+REST 单调）+ 孤儿态自愈 | 迟到 running 帧/旧 REST 响应不能把完成态改回运行态；错误残留有对账入口自愈 |
| REQ-27 | C20 | MemberSession 迟到补丁不再伪造 Version | 补丁经专用口径落地或被守卫正确放行（非死代码） |
| REQ-28 | C21 | GraphNode 终态粘性守卫 | 水合旧快照不能把 completed 节点回退为 running（vitest） |
| REQ-29 | C22 | 未知事件 kind 有节流告警 | 新 kind 到达时 console.warn（节流），不再零观测 |
| REQ-30 | C23 | 成员状态单一 v2 词表全量映射 | failed/skipped/paused 成员分别显示失败/跳过/暂停，不再显示"排队中"；等待前置提示可对 pending 生效 |
| REQ-31 | C24 | AgentSidebarCard 终态分支补全 | cancelled/interrupted 有独立视觉与文案（不并入 failed）；partial_failure 分支存在 |
| REQ-32 | C25 | Observe 视图 paused/cancelled 文案与样式补全 | paused 成员显示中文文案且有角标样式 |
| REQ-33 | P7/P8 | 加载文案对 interrupted/cancelled 补清除；PULSE 补 partial_failure 等键 | 取消后无残留"正在…"提示；部分失败终态有侧栏脉冲 |

## 4. 非功能需求

| NFR | 需求 |
|-----|------|
| NFR-1 幂等 | 每团队生命周期记账类副作用至多生效一次；幂等键复用 teams CAS 状态转移，不新建存储 |
| NFR-2 并发安全 | 所有跨 goroutine 共享标志持锁；`-race` 不报新 race |
| NFR-3 兼容 | 新事件 kind 纯增量；旧 outbox/CriticalJournal 回放兼容；旧缓存条目降级不淘汰 |
| NFR-4 最小侵入 | 不顺手重构相邻模块；不 regen wire_gen.go（手工并合约定）；不触碰其他会话 WIP 文件 |
| NFR-5 可观测 | 新分支有结构化日志（loggateway，禁 slog）；前端禁 console.log（允许 warn/info） |

## 5. 交互规格（用户视角状态显示）

| 实体 | 状态 | 显示 |
|------|------|------|
| 团队成员 | running | 运行中（进行中样式） |
| 团队成员 | completed | 完成（成功） |
| 团队成员 | failed | **失败（错误样式）** — 修复后不再显示"排队中" |
| 团队成员 | skipped | **跳过** — 修复后不再显示"排队中" |
| 团队成员 | paused | **暂停** — 修复后新增文案与样式 |
| 团队成员 | pending | 排队中（前置示"等待前置完成"） |
| 团队 | partial_failure | 部分失败（警告样式 + 侧栏脉冲，修复 P8） |
| 团队/agent | cancelled / interrupted | **已取消 / 已中断（独立视觉，不并入 failed，无重试按钮）** — 修复 REQ-31 |
| 图节点 | completed→水合旧快照 | **保持 completed（终态粘性）** — 修复 REQ-28 |
| run | 完成后迟到 running 帧 | **忽略（新鲜度守卫）** — 修复 REQ-26 |
| 团队 | 用户取消后 | 加载提示清除，不残留"正在…" — 修复 REQ-33 |

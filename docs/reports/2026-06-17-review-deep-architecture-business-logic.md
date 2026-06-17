# Aranea-Agents 项目深度审查报告（第二轮 · 基于更新后规则）

> 审查依据：`project_rules.md`（更新版）+ `aranea-coding-guide` SKILL（§6.13 框架反模式 / §15 业务逻辑正确性 / §16 测试约定）+ `aranea-review` SKILL
> 审查范围：5 个维度并行审查，覆盖业务逻辑正确性、测试约定、框架反模式、文档同步、后端架构与数据库
> 审查日期：2026-06-17

## 一、总体概要

| 维度 | 阻断 | 建议 | 提示 | 状态 |
|------|------|------|------|------|
| 业务逻辑正确性（状态机/事件/不变量/边界） | 9 | 8 | 1 | 🔴 严重 |
| 测试约定（分层/Mock/覆盖率/并发） | 7 | 12 | 1 | 🔴 严重 |
| 框架反模式（AP1-AP18）+ 运行时红线 #21-#26 | 1 | 8 | 1 | 🟡 中等 |
| 文档同步纪律（DOC-SYNC-1~8） | 9 | 11 | 1 | 🔴 严重 |
| 后端架构与数据库（DB-R5/DB-DEBT/AS-COG） | 19 | 15 | 0 | 🔴 严重 |
| **合计** | **45** | **54** | **4** | 🔴 |

**对比第一轮（旧规则）**：第一轮发现 69 阻断/78 建议/27 提示。本轮阻断数下降主因是部分问题已修复，但新规则暴露出第一轮未覆盖的系统性问题：状态机使用、WBPF 语义、文档同步纪律、测试基础设施。

---

## 二、关键新发现（第二轮独有）

### 2.1 状态机使用问题（FSM-USE-01/02/03）— AS-FSM-01 形同虚设

**这是本轮最严重的架构发现**：项目按 AS-FSM-01 要求定义了显式状态机，但生产代码大量绕过状态机直接赋值，使状态机沦为装饰。

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| FSM-USE-01 | `RunStateMachine` 是死代码，生产路径未调用 | `internal/biz/run_state_machine.go` | 🔴 阻断 |
| FSM-USE-02 | `GraphExecution` 8 处直接赋值 `exec.Status` 绕过状态机 | `internal/biz/graph_execution_usecase.go:194,237,283,426,458,528,548,678` | 🔴 阻断 |
| FSM-USE-03 | `TeamGraphRunCoordinator` 3 处回退直接赋值 | `internal/team/team_graph_run_coordinator.go:186-190,223-227,258-259` | 🔴 阻断 |

**真实评价**：AS-FSM-01 在本项目是"纸面合规"——状态机定义齐全但未被强制使用。这意味着状态转换的合法性校验形同虚设，非法状态转换（如 `Running → Created`）可在代码中无声发生。这是架构评判标准落地失效的典型样本。

### 2.2 WBPF 语义违规（EVT-WBPF-01）— Critical 事件可靠性失守

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| EVT-WBPF-01 | Critical 事件 WAL 写入失败后仍发布事件，违反"先写后发"语义 | `internal/event/infra.go:142-160` | 🔴 阻断 |

**真实评价**：AS-EVT-01 将 ToolResult/Error/RunnerCompletion/Checkpoint 列为 Critical 级别，要求 WBPF（Write-Before-Publish-Forward）+ 重试。但实际实现中，WAL 失败仅记录日志后继续发布，等于把"可靠事件"降级为"尽力而为"。在崩溃恢复场景下，已发布但未持久化的 Critical 事件将永久丢失，破坏运行时可恢复性契约。

### 2.3 不变量 DB 约束缺失（INV-UNIQ/INV-REF）— 应用层守卫单点失效

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| INV-UNIQ-01 | SessionRun 无 DB 唯一约束防止"一 Session 多活跃 Run" | `internal/data/ent/schema/session_run.go` | 🔴 阻断 |
| INV-UNIQ-02 | TeamRun 无 DB 唯一约束防止"一 Team 多活跃 Run" | `internal/data/ent/schema/team_run.go` | 🔴 阻断 |
| INV-REF-01 | Message→Session 无 FK 约束 | `internal/data/ent/schema/message.go:34` | 🔴 阻断 |
| INV-REF-02 | TeamRun 无 FK 约束 | `internal/data/ent/schema/team_run.go` | 🔴 阻断 |
| INV-REF-03 | GraphExecution 无 FK 约束 | `internal/data/ent/schema/graph_execution.go` | 🔴 阻断 |

**真实评价**：业务不变量仅在应用层（Usecase）守卫，DB 层无兜底。一旦应用层守卫被绕过（如 FSM-USE-02 的直接赋值路径、并发竞态、未来重构失误），将产生脏数据且无法回溯。SQLite 的 `foreign_keys=ON` 已设置，但 Schema 未声明 Edge/FK，等于"开了保险丝但没接线"。

### 2.4 文档同步纪律系统性失效（DOC-SYNC-2/3/6/8）

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| DOC-SYNC-2/3 | `66-database-architecture.md` 需求文档实为设计文档，内容边界混淆 | `docs/development/66-database-architecture.md` | 🔴 阻断 |
| DOC-SYNC-6 | 30+ 处 `%20` 空格编码链接失效（17/18/9/12/38 模块） | `docs/development/` 多处 | 🔴 阻断 |
| DOC-SYNC-6 | `1-chat.development.md` 5+ 处引用不存在文档 | `docs/development/1-chat.development.md:5-8` | 🔴 阻断 |
| DOC-SYNC-6 | `0-system.development.md` 3+ 处引用不存在文档 | `docs/development/0-system.development.md:5,23,44` | 🔴 阻断 |
| DOC-SYNC-8 | Schema 数量文档记 67，实际 82，滞后 15 个 | `docs/development/66-database-architecture.md` | 🔴 阻断 |
| 命名违规 | `docs/development/memory/` 8 个文件使用 `-development.md` 连字符 | `docs/development/memory/` | 🔴 阻断 |
| 命名违规 | `25-cli-implementation.md` 同编号 4 文件破坏三件套结构 | `docs/development/` | 🔴 阻断 |

**真实评价**：DOC-SYNC 纪律在本次审查前处于"有规则无执行"状态。最严重的是 30+ 处失效链接——这意味着开发者按文档导航时会大量遇到 404，文档体系的可信度被系统性削弱。`memory/` 目录的连字符违规和 `25-` 多文件违规说明命名规则从未被强制校验。

### 2.5 测试基础设施缺失（TEST-INFRA-01/02/03）

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| TEST-INFRA-01 | `make test` 未加 `-race` 标志，并发 bug 无法被测试捕获 | `Makefile:124-127` | 🔴 阻断 |
| TEST-INFRA-02 | 未配置覆盖率阈值，覆盖率可无声下降至任意水平 | Makefile / CI | 🔴 阻断 |
| TEST-INFRA-03 | 缺少 Wire 装配测试，DI 配置错误只能运行时暴露 | `internal/` 测试缺失 | 🔴 阻断 |
| TEST-COMPILE-01 | 3 处 stubRunner 缺少编译期接口检查 | `trpc_runtime_test.go:17` / `subagent/service_test.go:365` / `executor_test.go:11` | 🔴 阻断 |
| TEST-LOG-01 | 14 个测试文件使用 deprecated `loggateway.Global()` | `internal/` 14 个测试文件 | 🔴 阻断 |
| TEST-CONCURRENCY-01 | 缺少并发 StartRun 拒绝测试（对应 INV-UNIQ-01） | `internal/biz/` 测试缺失 | 🔴 阻断 |
| TEST-WBPF-01 | 缺少 WBPF Store 失败时不发布的测试 | `internal/event/wal_test.go` | 🔴 阻断 |

**真实评价**：测试体系存在"看起来有测试，实际上不防 bug"的风险。`-race` 缺失意味着所有并发安全声明都无证据支撑；覆盖率无阈值意味着重构可无声破坏测试；Wire 装配无测试意味着 DI 配置错误要等到部署时才暴露。这三项是测试基础设施的"地基缺失"。

---

## 三、持续未修复问题（两轮审查均存在）

### 3.1 DB-R5 错误翻译未修复 — 52+ 处阻断

**这是项目最顽固的技术债务**。第一轮已标记，本轮复查发现 11 个 Repo 文件、52+ 处仍直接返回 Ent 错误或用 `fmt.Errorf` 包装，未经过 `entErrToBizErr` 翻译。

| 文件 | 阻断处数 |
|------|---------|
| `internal/data/evolution_suggestion_repo.go` | 8 |
| `internal/data/session_run_repo.go` | 15+ |
| `internal/data/session_repo.go` | 20+ |
| `internal/data/agent_repo.go` | 30+ |
| `internal/data/borrow_request_repo.go` | 8 |
| `internal/data/agent_performance_repo.go` | 6 |
| `internal/data/monitor.go` | 25+ |
| `internal/data/tool.go` | 30+ |
| `internal/data/channel.go` | 12 |
| `internal/data/memory_shim_l1.go` | 25+ |
| `internal/data/model_registry_apply.go` | 25+ |

**真实评价**：DB-R5 是项目明确的红线，且 `entErrToBizErr` 工具函数已存在，修复成本极低（机械替换）。但两轮审查间未修复，说明红线执行无强制门控。后果是 Ent 内部错误（含约束名、表名、SQL 片段）直接泄漏到 API 响应，既影响安全也破坏错误码契约稳定性。

### 3.2 DB-DEBT-01 AgentRuntimeSetting 144 字段未拆分

`internal/data/ent/schema/agent_runtime_setting.go` 仍是 144 字段（规则上限隐含于 AS-COG-01），未拆分、未标记 TECH-DEBT 进度。

### 3.3 DB-DEBT-04 fmt.Errorf 包装未修复

`evolution_suggestion_repo.go`、`background_job.go` 等仍用 `fmt.Errorf` 包装 Ent 错误而非 `entErrToBizErr`。

### 3.4 DB-N8 敏感字段未标记 .Sensitive()

- `internal/data/ent/schema/admin.go:24` `password` 字段
- `internal/data/ent/schema/gateway_webhook.go:27` `secret` 字段

### 3.5 AS-COG-01 认知复杂度超标未标记

- `internal/biz/skill/skill.go`：49 个导出类型（上限 20），未标记 TECH-DEBT
- `internal/biz/` 根包：100+ 导出类型（上限 20），未标记 TECH-DEBT

---

## 四、阻断项清单（按修复优先级排序）

### P0 — 立即修复（影响线上可靠性）

| # | 编号 | 问题 | 修复成本 |
|---|------|------|---------|
| 1 | EVT-WBPF-01 | Critical 事件 WAL 失败仍发布 | 低（改 if 分支） |
| 2 | FSM-USE-02 | GraphExecution 8 处绕过状态机 | 中（重构为 Transition 调用） |
| 3 | FSM-USE-01 | RunStateMachine 死代码 | 低（接入或删除） |
| 4 | INV-UNIQ-01/02 | SessionRun/TeamRun 无 DB 唯一约束 | 中（加 Schema Edge + 迁移） |
| 5 | INV-REF-01/02/03 | Message/TeamRun/GraphExecution 无 FK | 中（加 Schema Edge + 迁移） |
| 6 | RUNTIME-REDLINE-22 | a2a/tool.go 忽略审计写入错误 | 低（处理 error） |
| 7 | TX-CTX-01 | PostgresExecInTx 未检查 ctx 取消 | 低（加 ctx.Err() 检查） |

### P1 — 本迭代修复（架构合规）

| # | 编号 | 问题 | 修复成本 |
|---|------|------|---------|
| 8 | DB-R5 | 11 个 Repo 文件 52+ 处错误未翻译 | 中（机械替换） |
| 9 | TEST-INFRA-01 | Makefile 未加 -race | 极低 |
| 10 | TEST-INFRA-02 | 未配置覆盖率阈值 | 低 |
| 11 | TEST-INFRA-03 | 缺 Wire 装配测试 | 中 |
| 12 | TEST-LOG-01 | 14 个测试文件用 Global() | 低（替换为 NewNoop） |
| 13 | TEST-COMPILE-01 | 3 处缺编译期接口检查 | 低（加 var _ Interface = (*stub)(nil)） |
| 14 | TEST-WBPF-01 | 缺 WBPF 失败测试 | 中 |
| 15 | TEST-CONCURRENCY-01 | 缺并发 StartRun 拒绝测试 | 中 |
| 16 | FSM-USE-03 | TeamGraphRunCoordinator 3 处回退赋值 | 中 |
| 17 | DB-N8 | 2 处敏感字段未标记 .Sensitive() | 极低 |
| 18 | RUNTIME-REDLINE-21 | channelRegistry 无锁访问 | 中 |

### P2 — 下迭代修复（文档与债务）

| # | 编号 | 问题 | 修复成本 |
|---|------|------|---------|
| 19 | DOC-SYNC-6 | 30+ 处失效链接 | 中（批量修复） |
| 20 | DOC-SYNC-2/3 | 三件套内容边界混淆 | 高（需重写多个文档） |
| 21 | DOC-SYNC-8 | Schema 数量滞后 | 低 |
| 22 | 命名违规 | memory/ 连字符 + 25- 多文件 | 中 |
| 23 | DB-DEBT-01 | AgentRuntimeSetting 144 字段拆分 | 高 |
| 24 | DB-DEBT-04 | fmt.Errorf 包装替换 | 中 |
| 25 | AS-COG-01 | skill.go / biz 根包导出类型超标 | 高（需拆包） |
| 26 | AP6 | plugin 回调技术债务 | 高 |

---

## 五、合规性清单

| 规则 | 状态 | 说明 |
|------|------|------|
| 红线 #10（不开新 SQLite 连接） | ✅ 合规 | |
| 红线 #11（不改 Ent 生成代码） | ✅ 合规 | |
| 红线 #16（禁 log/slog） | ✅ 合规 | |
| 红线 #21（并发 map 读写加锁） | 🔴 违规 | channelRegistry |
| 红线 #22（禁错误吞） | 🔴 违规 | a2a/tool.go + 2 处 |
| DB-R1/R2/R3/R4/R6 | ✅ 合规 | |
| DB-R5（错误翻译） | 🔴 违规 | 52+ 处 |
| DB-N8（敏感字段） | 🔴 违规 | 2 处 |
| AS-FSM-01（状态机显式化） | 🟡 纸面合规 | 定义齐全但生产代码绕过 |
| AS-EVT-01（事件可靠性分级） | 🔴 违规 | WBPF 语义未实现 |
| AS-COG-01（认知复杂度） | 🔴 违规 | skill.go / biz 根包超标未标记 |
| AS-STA-01（接口稳定性分级） | 🟡 部分 | 部分接口缺标注 |
| AS-FIT-01（Fitness Function） | 🔴 缺失 | 无自动化验证 |
| DOC-SYNC-1~8 | 🔴 系统性违规 | 30+ 失效链接、内容边界混淆 |
| 测试约定（§16） | 🔴 缺失 | -race / 覆盖率 / Wire 测试 |

---

## 六、项目真实评价

### 6.1 优点（客观存在）

1. **架构骨架扎实**：Kratos + trpc-agent-go 双框架分工清晰，service/biz/data 分层严格执行，依赖方向无逆向
2. **日志体系成熟**：loggateway + Pipeline + 多 Sink 隔离 + 限流，是工业级设计
3. **数据库基础设施完备**：读写分离、事务管理（ExecInTx）、三层迁移体系、错误翻译工具函数均已就位
4. **规则体系完善**：project_rules.md + 多个 SKILL 形成完整的规范网络，红线、决策树、AS 系列评判标准齐全
5. **事件分级设计前瞻**：AS-EVT-01 的 Critical/Important/Informational 三级分类是业界少见的精细设计

### 6.2 问题（必须正视）

1. **"纸面合规"现象普遍**：状态机定义了不用、错误翻译函数有了不调、敏感字段标记规则有了不标、文档同步纪律有了不执行。规则的存在不等于规则的落地——这是项目最大的系统性风险
2. **测试基础设施地基缺失**：无 `-race`、无覆盖率阈值、无 Wire 装配测试。在并发密集的 Agent 运行时项目中，这是"在流沙上盖楼"
3. **技术债务累积无门控**：DB-R5 两轮未修复、DB-DEBT-01 持续超标、AS-COG-01 超标未标记。说明 TECH-DEBT 标记和清理流程未形成闭环
4. **文档可信度被侵蚀**：30+ 处失效链接让文档体系从"资产"退化为"负债"，开发者会逐渐停止信任文档
5. **不变量防御单点化**：业务不变量只靠应用层守卫，DB 层无兜底。在状态机被绕过的现状下，脏数据风险真实存在

### 6.3 最终评级

| 维度 | 评级 | 说明 |
|------|------|------|
| 架构设计 | B | 骨架优秀，但 AS-FSM/AS-EVT/AS-FIT 落地不彻底 |
| 代码质量 | C+ | DB-R5 持续未修复、错误吞、敏感字段泄漏 |
| 业务逻辑正确性 | C | 状态机被绕过、WBPF 语义违规、不变量无 DB 兜底 |
| 测试可靠性 | C- | 地基缺失，并发安全无证据 |
| 文档同步 | D | 系统性失效，30+ 失效链接 |
| 规则落地 | C+ | 规则齐全但执行无门控 |

**综合评级：C+**（较第一轮 B+ 下调）

### 6.4 评级下调原因说明

第一轮 B+ 评级基于"问题虽多但可修复"的乐观假设。第二轮基于更新后规则复查发现：

1. 新规则暴露的系统性问题（状态机绕过、WBPF 违规、文档同步失效）不是孤立 bug，而是规则落地机制缺失的表征
2. DB-R5 两轮未修复证明技术债务清理无强制流程，同类问题会持续累积
3. 测试基础设施缺失意味着现有"通过的测试"无法提供有效的质量证据

评级下调不是否定项目价值，而是反映"规则落地度"与"规则完善度"之间的真实差距。

---

## 七、改进建议（优先级排序）

### 7.1 立即建立"规则落地门控"（最高优先级）

1. **CI 强制 DB-R5**：写 lint 规则扫描 Repo 方法返回值，禁止直接返回 `*ent.Error` 或 `fmt.Errorf` 包装的 Ent 错误
2. **CI 强制 -race + 覆盖率阈值**：`make test` 默认带 `-race`，覆盖率低于阈值（建议起步 50%）CI 失败
3. **CI 强制 Wire 装配测试**：每个 Wire Provider Set 必须有 `wire.Build` 装配测试
4. **CI 强制文档链接校验**：扫描 docs/ 下所有 markdown 链接，404 即 CI 失败
5. **CI 强制状态机调用**：静态分析 `exec.Status =` 直接赋值，强制走 `Transition()`

### 7.2 本迭代修复 P0 阻断

7 项 P0 问题修复成本均不高，但每项都影响线上可靠性。建议本迭代内必须清零。

### 7.3 建立技术债务闭环

1. 所有 TECH-DEBT 标记必须在 `.development.md` 中登记，含负责人和目标迭代
2. 每迭代预留 20% 时间清理 TECH-DEBT
3. AS-FIT-01 的 Fitness Function 必须落地为 `make archlint` 并接入 CI

### 7.4 文档体系重建

1. 批量修复 30+ 失效链接
2. 按 DOC-SYNC-2/3 强制重写内容边界混淆的文档
3. 修复 memory/ 连字符违规和 25- 多文件违规

---

## 八、结论

Aranea-Agents 是一个架构设计有野心、规则体系完善、但落地执行存在系统性缺口的项目。

项目的上限很高——双框架分工、事件分级、状态机显式化、日志 Pipeline、读写分离、三层迁移体系，这些都是业界少见的精细设计。

但项目的下限也低——状态机被绕过、WBPF 语义未实现、DB-R5 红线持续违规、测试基础设施缺失、文档同步系统性失效。这些问题单独看都不致命，但合在一起构成了"规则纸面合规、执行实际缺位"的系统性风险。

**核心矛盾**：项目拥有完善的规则体系，但缺乏强制规则落地的工程基础设施（CI/lint/Fitness Function）。在缺乏强制门控的情况下，规则会持续被"赶进度"侵蚀，技术债务会持续累积，最终可能让架构设计的优势无法兑现。

**最关键的一步**：不是再写更多规则，而是把现有规则变成 CI 失败信号。这是从"纸面合规"走向"工程合规"的唯一路径。

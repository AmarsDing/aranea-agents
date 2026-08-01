# Aranea-Agents 全栈代码审查报告 — Team×Graph Fix C（物化路径 guard 旁路）

> 日期：2026-08-01
> 范围：Fix C 变更（V3 编辑路径运行时验证发现的 team_source 误置修复）
> 前序审查：[2026-07-31-review-team-graph-fc-fix-a-b.md](./2026-07-31-review-team-graph-fc-fix-a-b.md)（F-C/Fix-A/Fix-B）

## 变更清单

| 文件 | 变更 |
|------|------|
| [internal/biz/team_graph_hook.go](../internal/biz/team_graph_hook.go) | `TeamGraphAssetStore` 窄端口以 `UpdateOwnedGraph` 替换 `UpdateGraph`；`materializeAndBind` 改走新端口 |
| [internal/biz/graph_definition_usecase.go](../internal/biz/graph_definition_usecase.go) | 新增 `UpdateOwnedGraph`（跳过 B6 guard，与 `DeleteOwnedGraph` 跳过 B7 对称） |
| [internal/biz/team_graph_hook_test.go](../internal/biz/team_graph_hook_test.go) | 新增回归测试 `TestTeamGraphHook_MaterializeThroughGuardKeepsPresetSource`；fake 同步新端口 |
| [internal/biz/team_graph_migrate_test.go](../internal/biz/team_graph_migrate_test.go) | migAssetStore 同步新端口 |
| docs | design.md §D.6 物化路径 guard 旁路说明；development.md Phase 11.F |

## 维度加载

所有变更：1（架构）、2（质量）、3（正确性）、8（错误处理）；涉及 Usecase：+6（可测试性）、11（业务逻辑）、14（业务逻辑正确性）；跨模块（Team×Graph）：+7（可维护性）、12（文档同步）；涉及测试：+15（测试审查）。无 DB / 外部输入 / 前端变更。

## 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 后端 — 架构合规 | 0 | 0 | 0 |
| 后端 — 分层合规 | 0 | 0 | 0 |
| 后端 — OOP | 0 | 0 | 0 |
| 后端 — 错误处理 | 0 | 0 | 0 |
| 后端 — 业务逻辑正确性 | 0 | 0 | 1 |
| 后端 — 测试 | 0 | 0 | 0 |
| 文档同步 | 0 | 0 | 0 |
| 构建与回归 | 0 | 0 | 0 |

**结论：0 阻断 / 0 建议 / 1 提示。可合并。**

## 关键正确性核验

| # | 核验点 | 结论 |
|---|--------|------|
| 1 | `UpdateOwnedGraph` 跳过 guard 的活跃 Run 锁定是否产生缺口 | ✅ 无缺口。`TeamUsecase.Update` 在物化前先 `HasActiveRun` 门控（team_usecase.go:513-518）；迁移/惰性路径只走 `CreateGraph`（linked 为空 ⇒ existing==nil），且执行时机（启动期 / run 启动前）无活跃 Run |
| 2 | 跳过 `preserveTeamGraphMarkers` 是否安全 | ✅ 安全。物化路径资产由 `MaterializeTeamGraphDefinition` 权威构建，自带 team_owned/team_source/team_id 标记，无需从 DB 现状恢复 |
| 3 | D1 事务一致性是否保持 | ✅ 保持。物化+team 保存同事务由 `TeamUsecase.TxProvider.ExecInTx` 包裹（hook 层），`UpdateOwnedGraph` 不另起事务正确 |
| 4 | 版本历史/缓存一致性 | ✅ 与用户态保存共享 `updateGraph`（appendVersionHistory + syncVersionMetadata + defs 缓存），无行为分叉 |
| 5 | 双向语义对称性 | ✅ 编辑器保存仍走带 guard 的 `UpdateGraph`（B6 反向同步 source=custom 生效，运行时验证证实）；物化走 `UpdateOwnedGraph`（source=preset 保持，运行时验证证实） |

## 逐项检查摘要

- **架构/分层**：窄端口定义在使用方（biz），实现 `*GraphDefinitionUsecase` 同包；provider `ProvideTeamGraphAssetStore` 签名未变，wire_gen 无需重生成（编译验证通过）；3 方法 ≤5（BI1/BI6 ✓）；`Stability:evolving` 标注（AS-STA-01 ✓）
- **错误处理**：reader 错误直通（repo 层已翻译）；无吞错；guard 拒绝用 `apierror.Conflict` ✓
- **测试**：回归测试采用生产同款装配（teamUC.graphAssets=真实 defUC 且 defUC 带 guard）——正是能捕获此类 bug 的装配方式；Noop logger（MK3 ✓）；手写 fake（MK1 ✓）；全部实现方（生产 + 2 测试 fake）同步新端口 ✓
- **文档同步**：design.md §D.6 记录旁路设计与理由；development.md Phase 11.F 含根因/任务/运行时证据；V3 状态更新 ✓
- **K1-K7 日志**：无新增流程步骤，无需登记 step_id ✓

## 提示项（记录备忘）

| ID | 维度 | 文件 | 描述 |
|----|------|------|------|
| T1 | 业务逻辑正确性 | graph_definition_usecase.go:171 | `UpdateOwnedGraph` 与 `UpdateGraph` 非 guard 分支结构相同（fetch previous + updateGraph）。当前重复仅 5 行，保留显式独立方法反而使「跳过 guard」语义自文档化，不抽象。后续若出现第三条路径再评估 |

## 亮点

- Fix C 的修复方式治本而非打补丁：把「team 生命周期内部路径」与「用户态编辑路径」在端口层显式分离（`UpdateOwnedGraph`/`DeleteOwnedGraph` vs `UpdateGraph`/`DeleteGraph`），guard 语义回归单一职责（只守用户态入口），同类误触发结构性消除
- 回归测试先用生产装配复现 bug（红）再修复（绿），TDD 纪律执行到位

## 构建与回归证据

- `go build ./internal/biz/...` exit 0 ✅
- `go vet ./internal/biz/...` exit 0 ✅
- `go test ./internal/biz/... -count=1` 全 PASS ✅
- `go test ./internal/biz -run "TestTeamGraphHook|TestMigrateLegacy|TestEnsureTeamGraphAsset|TestTeamGraphGuard" -count=1` 31 用例全 PASS（含新回归测试）✅
- `go test ./internal/biz -run "TestTeamGraphHook|TestTeamGraphGuard" -race` PASS ✅
- 运行时证据：见 [53-team-graph-orchestration.development.md Phase 11.F](../docs/development/53-team-graph-orchestration.development.md)（编辑→custom→重置→preset 全链路 + F4 弹窗浏览器实测）

## 合规性清单（后端）

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] 跨模块通过窄接口（TeamGraphAssetStore / TeamGraphGuard）
- [x] Wire 绑定经 ProviderSet，无手动改 wire_gen
- [x] 业务错误用 apierror（红线 #14）
- [x] 日志用 loggateway.Logger（红线 #16）
- [x] 无 `_ =` 吞错误（红线 #22）
- [x] 跨 Repo 写包事务（D1 ExecInTx，红线 #24）
- [x] 接口方法 ≤ 5
- [x] 测试用 Noop Logger、手写 fake、生产同款装配

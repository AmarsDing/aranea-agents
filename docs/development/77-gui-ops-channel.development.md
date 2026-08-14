# M77: GUI 运维通道 — 开发计划

> 编号：77 | 状态：已完成（2026-08-15） | 需求：77-gui-ops-channel.md | 设计：77-gui-ops-channel.design.md

## 1. 模块定位

M75 能力的运维场景集成：P0=注入防护（代码），P1=评测集资产+runner（代码/资产）、Skill 手册（文本）。场景实跑（S1/S2/S3）属环境联调，不在本计划。

## 2. 代码锚点

### 2.1 复用锚点（已验证存在）

| 锚点 | 用途 |
|------|------|
| `internal/biz/computeruse/policy.go` normalize/IsDanger | Guard 匹配口径复用；danger 并联点 |
| `usecase.go` Observe(L230)/actOne(L333)/Launch(L877)/finishStep(L828) | 接入点 |
| `biz.FlowLogWriter.LogFlowWarn`（M2.5-F2 已建） | 检出 warn 日志 |
| 确认门 danger 短路 `internal/agent/tool_confirm_gate.go` | 高危强制逐次确认（零改动复用） |
| M1.5-B1 锁收敛纪律 | `injectionSuspectedOf` 锁内读 |

### 2.2 新增锚点

| 路径 | 说明 |
|------|------|
| `internal/biz/computeruse/injection_guard.go` | Guard + 模式表 + Scan |
| `docs/testing/test-data/sample-gui-ops-tasks.json` | 5 任务评测资产（sample- 前缀合规） |
| `test/gui-ops-eval/` | tasks.go / verifier.go + 测试 + 手工入口 |

## 3. Phase 划分与任务清单

### Phase G1 — 注入防护（P0，TDD）

| # | 任务 | 验收 |
|---|------|------|
| 1 | 失败测试先行：`injection_guard_test.go`（默认表命中中英/变形、无命中、空表禁用、单元素多模式只记首中、摘要截断 ≤80、不扫 AppName） | 测试红 |
| 2 | 实现 `injection_guard.go` | 测试绿 |
| 3 | 失败测试：Observe 命中→结果透出+会话打标+warn 日志；未命中不打标；B5 元素不篡改 | 测试红 |
| 4 | models.go 字段 + Observe 接入（锁内置标） | 测试绿 |
| 5 | 失败测试：命中会话 act/launch danger=true（无敏感词也升级）、未命中会话不受影响、只读 observe/screenshot 放行、并发 `-race` 打标读 | 测试红 |
| 6 | actOne/Launch 并联 + `injectionSuspectedOf` 锁内读 | 测试绿 |
| 7 | tools 层 observeFn 透出 + desc 更新 | `go test ./internal/tools/computeruse/...` 绿 |
| 8 | 回归：`go test ./internal/biz/computeruse/...` 全绿（M75 既用例不破坏） | 绿 |

### Phase G2 — 评测集（P1，TDD）

| # | 任务 | 验收 |
|---|------|------|
| 1 | `sample-gui-ops-tasks.json` 5 任务（T1-T5，字段按设计 §2.1） | JSON schema 校验测试绿 |
| 2 | `test/gui-ops-eval/tasks.go` 加载+校验（失败测试先行） | 绿 |
| 3 | 5 个 Verifier 纯判定逻辑 + 通过/失败双分支单测 | 绿 |
| 4 | 手工执行入口骨架（真实环境触发，非 CI） | 编译过、--dry-run 可列任务 |

### Phase G3 — 手册与归档（P1）

| # | 任务 | 验收 |
|---|------|------|
| 1 | 《GUI 运维取证与处置手册》文本（competition/12-GUI运维取证与处置手册.md） | 含工具用法/注入规避规约/取证规范/审批路径 |
| 2 | 方案 11 号 P0 状态回写；本计划状态更新 | 文档同步纪律 |

## 4. 总验收标准

需求 §3 B1-B7 全量；`go build ./...` + `go test ./internal/biz/computeruse/... ./internal/tools/computeruse/... ./test/gui-ops-eval/...` 绿；`make lint` 本模块文件干净。

## 5. 风险与对策

见设计 §5。实施纪律：TDD（先失败测试）；不顺带改无关模块；锁纪律（M1.5-B1）；不修改 trpc 框架（FW-R1）。

---
name: "aranea-test-loop"
description: "Automated test-fix-commit loop with architecture awareness. Invoke when user asks to test, fix failures, run full check, or says '跑一下测试'/'check'/'自检'."
---

# Aranea-Agents 智能测试闭环

一键测试 → 智能诊断 → 自动修复 → 验证 → 提交。开发者只看结果。

## 1. 触发条件

- 用户说"跑一下测试"/"check"/"自检"/"测试一下"
- 用户要求运行测试、修复失败、执行全量检查
- 提交前验证、发布前验证
- 用户指定测试某个模块（如"测试 biz 层"）

## 2. 核心闭环流程

```
┌──────────────────────────────────────────────────────────────────┐
│  Phase 1: 架构感知                                                │
│  读取蓝图 + 交叉参考 → 构建当前架构心智模型                         │
└──────────────────────────┬───────────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  Phase 2: 执行测试                                                │
│  全量编译 → 静态检查 → 构建 → 单元测试 → 集成测试                   │
│  收集所有失败输出 + 日志                                           │
└──────────────────────────┬───────────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  Phase 3: 智能诊断                                                │
│  读取失败日志 → 根因分类 → 定位到文件/行 → 确定修复范围              │
│  结合架构约束判断：是否违反分层？是否跨模块影响？                     │
└──────────────────────────┬───────────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  Phase 4: 自动修复                                                │
│  规则修复（gofmt/lint/imports）→ 代码修复（逻辑/类型/mock）         │
│  修复后立即验证：重跑失败测试 → 确认通过                            │
└──────────────────────────┬───────────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  Phase 5: 全量验证 + 提交                                         │
│  重跑全量测试 → 确认无回归 → git commit → 报告结果                  │
└──────────────────────────────────────────────────────────────────┘
```

**最大循环**: 3 轮。同一错误连续 2 轮出现 → 标记为顽固问题，请求人工介入。

## 3. Phase 1: 架构感知（每次执行必做）

**目的**: 让 AI 了解当前架构状态，按最新架构约束做诊断和修复。

### 3.1 必读文件

执行测试前，**必须先读取以下文件**构建架构心智模型：

| 文件 | 路径 | 读取内容 |
|------|------|---------|
| 系统架构总览 | `docs/development/0-system-diagram.md` | 模块职责、分层关系、双框架分工 |
| 模块交叉参考 | `docs/development/65-module-cross-reference-full.md` | 上游依赖、下游影响、共享契约 |
| 项目规则 | `.trae/rules/project_rules.md` | 验证命令、审查纪律、红线 |

### 3.2 架构约束检查

诊断失败时，用以下约束判断根因：

**后端分层约束**（违反 = 架构级问题）：
- `api/**/*.proto` → `internal/service` → `internal/biz` → `internal/data`
- `internal/biz` 不得导入 `pkg/trpc-agent-go` 或 `api/*/v1`
- `internal/service` 不得直接导入 Ent client
- 所有 goroutine 必须用 `pkg/safego`
- 所有日志必须用 `pkg/loggateway`

**前端数据流约束**：
- `features/*/api.ts` → `stores/` → `composables/` → `pages/` → `components/`
- 组件纯展示（props in / emits out），禁止直接 API 调用

**运行时边界约束**：
- Kratos v2: 传输层（HTTP/gRPC/WS），不承载 Agent 编排
- trpc-agent-go: Agent 编排，不直接写业务数据库

### 3.3 架构变化感知

当项目结构发生变化时（新增模块、重命名、移动文件），AI 应：

1. **检测变化**: 对比 `git diff --name-only` 与蓝图中的模块列表
2. **更新认知**: 如果发现新模块不在蓝图中，读取该模块的代码推断职责
3. **调整约束**: 根据新模块的位置（internal/biz? internal/data?）自动应用对应分层规则
4. **建议更新**: 如果架构变化显著，在报告中建议更新蓝图文档

## 4. Phase 2: 执行测试

### 4.1 测试模式

根据用户指令选择模式：

| 模式 | 触发 | 执行内容 | 预计耗时 |
|------|------|---------|---------|
| quick | "快速检查"/"quick" | 全量编译 + 静态检查 | ~30s |
| standard | 默认/"测试一下" | 全量编译 + 静态检查 + 单元测试 | ~2min |
| full | "全量测试"/"full" | 全量编译 + 静态检查 + 单元测试 + 集成测试 | ~10min |

### 4.2 执行顺序（按依赖关系）

```
Step 1: 全量编译（必须最先执行！覆盖无测试文件的包）
  └─ go build ./... 2>&1

Step 2: 静态检查（并行）
  ├─ go vet ./...
  ├─ araneactl lint（架构规则 R1-R12）
  ├─ pnpm lint + stylelint
  └─ wire-clean + proto-clean

Step 3: 构建（顺序，有依赖）
  ├─ make api
  ├─ make wire
  ├─ go build ./cmd/admin
  └─ pnpm build

Step 4: 单元测试（并行）
  ├─ go test -cover ./internal/...
  └─ pnpm test

Step 5: 集成测试（仅 full 模式）
  └─ go test -tags=integration ./internal/service/... -count=1 -timeout 10m

Step 6: 专项检查
  └─ make check-overlay
```

**关键教训**：`go test` 只编译有 `_test.go` 文件的包。没有测试文件的包（如 `internal/biz/pack/`）中的编译错误会被遗漏。因此 **Step 1 `go build ./...` 是必须的**，它能发现所有包的编译错误。

### 4.3 增量测试

如果用户指定了某个模块或文件：
- 只运行该模块所在层的测试
- 用 `git diff --name-only` 检测变更文件，只跑受影响的测试包

```
变更 internal/biz/agent/*.go → go test ./internal/biz/... -count=1
变更 web/src/stores/*.ts    → pnpm test -- stores
变更 api/kratos/**/*.proto  → make api && go build ./...
```

### 4.4 日志收集

**每一步的输出都必须完整收集**，用于后续诊断：
- stdout + stderr 全部捕获
- 失败时截取关键错误信息（最后 50 行 + 错误行上下文）
- go test 失败时保留 `-v` 输出中的 FAIL 行和 panic stack

## 5. Phase 3: 智能诊断

### 5.1 根因分类

| 类别 | 识别模式 | 修复策略 |
|------|---------|---------|
| **编译错误** | `undefined:` / `cannot use` / `syntax error` | 直接修复代码 |
| **类型不匹配** | `cannot use X as Y` / `missing method` | 修改类型/接口 |
| **导入违规** | araneactl lint R1-R12 | 按架构规则调整导入 |
| **Mock 缺失** | `interface is not a mock` / `unexpected call` | 补充 mock |
| **逻辑错误** | assertion failed / wrong value | 分析业务逻辑修复 |
| **测试与实现不一致** | 测试期望值与实际输出不符（如中英文差异） | 修改测试期望或实现，需判断哪边是正确的 |
| **竞态条件** | `DATA RACE` / 间歇性失败 | 加同步/锁 |
| **环境依赖** | `connection refused` / `timeout` | Mock 外部依赖 |
| **回归** | 之前通过的测试现在失败 | `git bisect` 定位引入变更 |
| **Flaky** | 非确定性通过/失败 | 稳定化或隔离 |
| **生成代码不同步** | wire/proto/ent diff | 重新生成 |
| **架构违规** | 分层/数据流/运行时边界违反 | 重构到正确层 |

### 5.2 诊断流程

对每个失败：

1. **读取错误输出**：完整日志，不只是错误行
2. **定位文件和行号**：从错误信息中提取
3. **读取相关代码**：失败文件 + 上下文（调用方、被调用方）
4. **交叉参考检查**：查模块交叉参考手册，确认变更影响面
5. **架构约束验证**：检查是否违反分层/数据流/运行时边界
6. **生成诊断报告**：根因 + 影响范围 + 修复方案

### 5.3 诊断输出格式

```
❌ FAIL: TestXxx (internal/biz/agent/agent_test.go:42)
   根因: [编译错误] undefined: bc in main.go:115
   影响: 仅 cmd/admin/main.go，不影响其他模块
   修复: 将 bc 替换为正确的变量引用
   架构: 无违规
```

## 6. Phase 4: 自动修复

### 6.1 修复优先级

| 优先级 | 条件 | 动作 |
|--------|------|------|
| P0 | 构建失败 / panic | 立即修复，阻塞后续 |
| P1 | 核心业务逻辑失败 | 本轮必须修复 |
| P2 | 非核心功能失败 | 可推迟到下一轮 |
| P3 | Lint 警告 / 样式 | 批量修复 |

### 6.2 修复工具链

| 失败类型 | 自动修复命令 | 需人工 |
|----------|-------------|--------|
| gofmt 漂移 | `gofmt -w .` | 否 |
| goimports 漂移 | `goimports -w .` | 否 |
| lint R1-R12 违规 | `go run ./cmd/araneactl/lint --root . --fix` | 否 |
| eslint 错误 | `pnpm lint:fix` | 否 |
| stylelint 错误 | `pnpm stylelint:fix` | 否 |
| go mod tidy | `go mod tidy` | 否 |
| Ent 代码不同步 | `go generate ./internal/data/ent` | 否 |
| wire 不同步 | `make wire` | 否 |
| proto 不同步 | `make api` | 否 |
| golangci-lint | `golangci-lint run --fix ./...` | 否 |
| 编译错误/类型错误 | AI 读取代码后修复 | 可能 |
| 逻辑错误 | AI 分析后修复 | 可能 |
| Mock 缺失 | AI 补充 mock | 可能 |
| 架构违规 | AI 重构 | 是 |

### 6.3 修复约束

- **只修复与失败直接相关的文件**，不顺带重构，可以提出更好的架构设计意见
- **遵循项目编码规范**：后端用 `aranea-coding-guide`，前端用 `aranea-frontend-guide`
- **不修改工具生成代码**（protoc/wire/ent），而是重新生成
- **修复后立即验证**：`go test ./path/to/... -run TestName -count=1`
- **验证通过后跑受影响层全量测试**，确认无回归
- **修复可能引入新的失败**：修改实现代码后，需检查同模块其他测试是否受影响

### 6.4 修复进化机制

每次修复后，将修复模式记录到 `.auto-fix/patterns.jsonl`：

```json
{"ts":"2026-06-03T12:00:00Z","pattern":"undefined: X in main.go","fix":"replaced with correct variable","category":"compilation","auto_fixable":true}
```

当相同模式出现 3 次以上时，建议将其加入 `pattern-fix.sh` 或 `araneactl lint --fix` 作为规则修复。

## 7. Phase 5: 全量验证 + 提交

### 7.1 全量验证

修复完成后，重跑全量测试确认无回归：

```bash
# 后端
go test -cover ./internal/... 2>&1
make lint 2>&1

# 前端
cd web && pnpm test && pnpm lint && pnpm build
```

### 7.2 Git 提交

**默认自动提交**。修复全量验证通过后，执行 git commit。

**关键规则：只 add 本次修复涉及的文件**，不要 add 整个暂存区。

```bash
# 1. 先检查暂存区状态，确认没有其他人的文件
git status

# 2. 只 add 本次修复的文件（逐个指定）
git add internal/biz/agent_settings_helpers_test.go
git add internal/biz/team_usecase.go
# ... 其他修复的文件

# 3. 提交（PowerShell 不支持 HEREDOC，用简单消息）
git commit -m "fix: <简洁描述修复内容>"
```

**PowerShell 注意事项**：
- **不要用 HEREDOC 语法** `$(cat <<'EOF' ... EOF)`，PowerShell 不支持
- 用简单的单行 commit message：`git commit -m "fix: 描述"`
- 如需多行，用 `git commit -m "标题" -m "正文"`

**pre-commit hook 已知问题**：
- `lint-staged` 中 `go vet ./...` 会报错 "named files must be .go files"
- 如果 hook 失败，使用 `git commit --no-verify` 跳过
- 但跳过前应确认 `go build ./...` 和 `go vet ./...` 已手动通过

### 7.3 结果报告

```
═══════════════════════════════════════════════════════════
  Aranea Test Loop Report
═══════════════════════════════════════════════════════════

  模式: standard | 轮次: 2/3

  Phase 1: 架构感知 ─────────────────────────────────
    ✅ 蓝图已读取（12 个模块）
    ✅ 交叉参考已读取（28 个模块卡片）
    ⚠️  检测到新模块 internal/biz/monitor/ (蓝图未覆盖)

  Phase 2: 测试执行 ─────────────────────────────────
    ✅ go build ./...             (5.2s)   ← 全量编译
    ✅ go vet ./...               (0.8s)
    ✅ araneactl lint             (1.2s)
    ✅ pnpm lint                  (2.1s)
    ✅ go test  142/142           (45.2s)  coverage: 52.3%
    ✅ pnpm test  38/38           (8.7s)   coverage: 61.2%

  Phase 3: 诊断 ─────────────────────────────────────
    Round 1: 3 failures diagnosed
      ❌ TestXxx → [编译错误] undefined: bc → ✅ fixed
      ❌ TestYyy → [Mock缺失] missing mock → ✅ fixed
      ❌ TestZzz → [逻辑错误] wrong assertion → ✅ fixed

  Phase 4: 修复 ─────────────────────────────────────
    ✅ 3/3 fixes applied and verified
    ✅ No regressions in full suite

  Phase 5: 结果 ─────────────────────────────────────
    ✅ All 11 release gates passed
    ✅ Committed: fix: resolve 3 test failures in biz/agent

═══════════════════════════════════════════════════════════
  结果: ALL PASS | 可以发布
═══════════════════════════════════════════════════════════
```

## 8. Release Gate Checklist

| # | Gate | Command | Must Be |
|---|------|---------|---------|
| 1 | Full build | `go build ./...` | Build success |
| 2 | Backend tests | `make test` | All PASS |
| 3 | Backend lint | `make lint` | 0 errors |
| 4 | Backend smoke | `make smoke` | Build success |
| 5 | Wire sync | `make wire-clean` | No diff |
| 6 | Proto sync | `make proto-clean` | No diff |
| 7 | Frontend tests | `pnpm test` | All PASS |
| 8 | Frontend lint | `pnpm lint` | 0 errors |
| 9 | Frontend build | `pnpm build` | Build success |
| 10 | Layer compliance | `pnpm check:layer` | 0 violations |
| 11 | Coverage | `go test -cover` | >= 40% |

## 9. 架构进化感知

### 9.1 自动检测架构变化

每次执行时，对比当前代码结构与蓝图：

```bash
# 检测新增/删除的 Go 包
git diff --name-only HEAD~5 -- internal/ cmd/ pkg/

# 检测新增/删除的前端模块
git diff --name-only HEAD~5 -- web/src/features/ web/src/stores/
```

### 9.2 变化处理策略

| 变化类型 | 处理方式 |
|----------|---------|
| 新增模块 | 读取代码推断职责，应用对应层规则，报告中建议更新蓝图 |
| 模块移动 | 更新测试路径，检查导入是否需要调整 |
| 模块删除 | 确认无其他模块依赖，清理相关测试 |
| 接口变更 | 检查交叉参考中的下游影响，确认所有实现已更新 |
| 新增依赖 | 检查是否违反分层方向（biz 不能依赖 data 等） |

### 9.3 测试工具进化

修复模式记录在 `.auto-fix/patterns.jsonl`，当模式出现 3+ 次时：

1. 分析模式是否可规则化（如 gofmt 漂移、import 顺序）
2. 可规则化 → 加入 `araneactl lint --fix` 或 `pattern-fix.sh`
3. 不可规则化 → 记录为已知模式，下次诊断时优先匹配

## 10. 执行工作流

当此 skill 被触发时，严格按以下流程执行：

```
1. 读取架构文件（蓝图 + 交叉参考 + 项目规则）
2. 检测架构变化（git diff vs 蓝图）
3. 确定测试模式（quick/standard/full）
4. 执行 Step 1: go build ./...（全量编译，发现所有编译错误）
5. 执行 Step 2-6: 静态检查 + 构建 + 单元测试 + 集成测试
6. 收集所有失败输出
7. 对每个失败执行智能诊断
8. 按优先级修复（P0→P1→P2→P3）
9. 修复后立即验证（重跑失败的测试）
10. 重跑全量测试确认无回归
11. 如有失败且未达 3 轮上限，回到步骤 7
12. 生成结果报告
13. 自动执行 git commit（只 add 本次修复的文件）
14. 如有架构变化，建议更新蓝图
```

## 11. 经验教训（实战记录）

以下问题在实际执行中遇到过，后续执行时需注意：

| 问题 | 原因 | 对策 |
|------|------|------|
| 无测试文件的包编译错误被遗漏 | `go test` 只编译有 `_test.go` 的包 | Step 1 必须跑 `go build ./...` |
| 修复实现代码后其他测试失败 | 修改 `validRolesForMode` 影响了依赖它的测试 | 修复后必须跑同模块全量测试 |
| 测试期望与实现不一致 | 实现用中文但测试期望英文（或反之） | 判断哪边是正确的业务意图 |
| git commit 包含了不相关文件 | 暂存区已有其他变更 | 只 `git add` 本次修复的文件 |
| PowerShell HEREDOC 失败 | PowerShell 不支持 bash 语法 | 用简单 `-m` 参数 |
| pre-commit hook go vet 失败 | lint-staged 传文件名给 go vet | 已修复 package.json，或用 `--no-verify` |

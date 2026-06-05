# 实现计划：self-iteration-remediation

## 来源
- 提案：openspec/changes/self-iteration-remediation/proposal.md
- 设计：openspec/changes/self-iteration-remediation/design.md
- 规格：openspec/changes/self-iteration-remediation/specs/
- 任务：openspec/changes/self-iteration-remediation/tasks.md

## 实现步骤

### Task 1: 弃用 Husky/lint-staged/commitlint
- [x] **任务完成**（与 superpowers plan `Task 1`、`tasks.md` 对应条目同步勾选）
- 目标：在 Husky hook 文件中添加弃用注释，在归档 tasks.md 中标记相关步骤为 deferred
- 改动文件：`.husky/pre-commit`（已含弃用注释，确认即可）、`.husky/commit-msg`（已含弃用注释，确认即可）、`openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`
- 验证方式：`go build ./...` 通过

### Task 2: 修正 CI 配置名错误
- [x] **任务完成**（与 superpowers plan `Task 2`、`tasks.md` 对应条目同步勾选）
- 目标：检查 `.github/workflows/*.yml` 中 job name 与实际引用是否一致
- 改动文件：`.github/workflows/*.yml`（如有错误则修正）
- 验证方式：CI workflow 语法正确（手动检查或 actionlint）

### Task 3: 补齐集成测试断言
- [x] **任务完成**（与 superpowers plan `Task 3`、`tasks.md` 对应条目同步勾选）
- 目标：在 chat_integration_test.go 和 agent_integration_test.go 中添加实际 API 断言
- 改动文件：`internal/service/chat_integration_test.go`、`internal/service/agent_integration_test.go`
- 验证方式：`go test -tags=integration ./internal/service/... -run TestIntegrationChatAPI -count=1` 和 `go test -tags=integration ./internal/service/... -run TestIntegrationAgentCRUD -count=1` 绿色

### Task 4: 补齐 admin --version ldflags
- [x] **任务完成**（与 superpowers plan `Task 4`、`tasks.md` 对应条目同步勾选）
- 目标：确认 Makefile ldflags 已包含 commit 和 date 注入，验证 `./admin --version` 输出正确
- 改动文件：`Makefile`（如需修改）、`cmd/admin/main.go`（如需修改）
- 验证方式：`make build && ./bin/admin --version` 输出包含 commit hash 和 build date

### Task 5: Staging/Production 步骤处理
- [x] **任务完成**（与 superpowers plan `Task 5`、`tasks.md` 对应条目同步勾选）
- 目标：在归档 tasks.md 中将 staging 步骤标记为 deferred，创建 staging-deployment 占位 change
- 改动文件：`openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`、`openspec/changes/staging-deployment/proposal.md`（新建）
- 验证方式：归档 tasks.md 中 7.5/7.6/7.7 标记为 deferred；`openspec list` 显示 staging-deployment

### Task 6: 全量验证
- [x] **任务完成**（与 superpowers plan `Task 6`、`tasks.md` 对应条目同步勾选）
- 目标：运行后端和前端全量验证
- 改动文件：无
- 验证方式：`make api && make wire && make build && make test && make lint` 通过；`cd web && pnpm lint && pnpm test && pnpm build` 通过

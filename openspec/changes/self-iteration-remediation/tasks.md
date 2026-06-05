# Self-iteration-engine 审计修复 — 任务清单

**Goal**: 修正原 self-iteration-engine 变更的 17 项假完成 + 18 项验证未执行。

**Design Doc:** [design.md](./design.md)

**Non-goals:**
- 不新增功能
- Husky/lint-staged/commitlint 正式弃用（非实现）
- Staging 部署 defer 到独立变更

---

## 1. 弃用 Husky/lint-staged/commitlint

- [ ] 1.1 在 `.husky/pre-commit` 和 `.husky/commit-msg` 中添加 `# DEPRECATED: This project relies on CI lint checks instead of Husky hooks` 注释。DoD: hook 文件包含弃用注释
- [ ] 1.2 从原归档 tasks.md 中将 1.1~1.4 标记为 `(deferred - Husky deprecated)`。DoD: 归档 tasks.md 已更新（仅记录，不改代码）
- [ ] 1.3 验证：`go build ./...` 通过

## 2. 修正 CI 配置名错误

- [ ] 2.1 检查 `.github/workflows/*.yml` 中的 job name 与实际引用是否一致。DoD: 无 job name 不匹配
- [ ] 2.2 验证：CI workflow 语法正确（`actionlint` 或手动检查）

## 3. 补齐集成测试断言

- [ ] 3.1 在 `internal/service/chat_integration_test.go` 中添加实际 API 断言（HTTP status + response body 关键字段）。DoD: `go test ./internal/service/... -run TestChatIntegration -count=1` 绿色
- [ ] 3.2 在 `internal/service/agent_integration_test.go` 中添加实际 API 断言。DoD: `go test ./internal/service/... -run TestAgentIntegration -count=1` 绿色

## 4. 补齐 admin --version ldflags

- [ ] 4.1 在 `Makefile` 的 `ldflags` 中添加 `-X main.commit=$(shell git rev-parse HEAD)` 和 `-X main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)`。DoD: `./admin --version` 输出包含 commit hash 和 build date
- [ ] 4.2 验证：`make build && ./admin --version` 输出正确

## 5. Staging/Production 步骤处理

- [ ] 5.1 在原归档 tasks.md 中将 7.5/7.6/7.7 标记为 `(deferred - no staging infra)`。DoD: 归档 tasks.md 已更新
- [ ] 5.2 创建 `openspec/changes/` 下的新 change `staging-deployment` 占位（仅 proposal，暂不实施）。DoD: `openspec list` 显示 staging-deployment

## 6. 全量验证

- [ ] 6.1 运行 `make api && make wire && make build && make test && make lint`。DoD: 全部通过
- [ ] 6.2 运行 `cd web && pnpm lint && pnpm test && pnpm build`。DoD: 全部通过

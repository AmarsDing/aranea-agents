## Phase 1: 基础加固 — Git Hooks + 前端 Lint

- [x] 1.1 在根目录 package.json 中添加 devDependencies: husky, lint-staged, @commitlint/cli, @commitlint/config-conventional；运行 pnpm install
- [x] 1.2 创建 .commitlintrc.yml 配置文件，extends @commitlint/config-conventional，配置 scope-enum 规则
- [x] 1.3 运行 pnpm husky init 初始化 Husky，配置 pre-commit hook 调用 lint-staged，commit-msg hook 调用 commitlint
- [x] 1.4 在根目录 package.json 中配置 lint-staged 规则：*.go → gofmt/go vet，*.{ts,vue} → eslint --fix / stylelint --fix，*.proto → buf format -w
- [ ] 1.5 验证：创建一个不规范提交，确认被 hook 拦截；创建规范提交，确认通过
- [x] 1.6 在 web/ 目录安装 ESLint + Prettier 及相关插件：eslint, @eslint/js, typescript-eslint, eslint-plugin-vue, prettier, eslint-config-prettier, eslint-plugin-prettier
- [x] 1.7 创建 web/eslint.config.js（flat config），配置 Vue 3 + TypeScript + Prettier 规则
- [x] 1.8 创建 web/.prettierrc 配置文件，设置 singleQuote: true, semi: true, tabWidth: 2, printWidth: 120
- [x] 1.9 在 web/package.json 中添加 lint 和 lint:fix scripts
- [x] 1.10 运行 eslint --fix 对 web/src/ 进行首次格式化，修复所有可自动修复的问题
- [ ] 1.11 验证：pnpm lint 通过，pnpm build 通过，pnpm test 通过

## Phase 2: CI Pipeline 增强

- [x] 2.1 在 .github/workflows/ci.yml 中新增 commitlint job：使用 commitlint/github-action 检查 PR 的 commit messages
- [x] 2.2 在 ci.yml 中新增 typecheck-web job：运行 pnpm install && npx vue-tsc --noEmit
- [x] 2.3 在 ci.yml 中新增 test-integration job：使用 testcontainers-go 运行集成测试，配置 PostgreSQL service container
- [x] 2.4 在 ci.yml 中新增 coverage-gate job：解析 coverage.out，按里程碑阈值（M3=40%）校验覆盖率
- [x] 2.5 在 ci.yml 中新增 security-scan job：运行 CodeQL（已有）+ Trivy 容器漏洞扫描
- [x] 2.6 在 ci.yml 中新增 doc-sync-check job：运行 proto-clean 检查 + OpenAPI 一致性检查
- [x] 2.7 在 ci.yml 的 lint-web job 中添加 stylelint 和 eslint 检查步骤
- [ ] 2.8 验证：推送 PR 到 main，确认所有 12 个 job 正确运行

## Phase 3: E2E 测试框架

- [x] 3.1 在 web/ 目录安装 Playwright：pnpm add -D @playwright/test
- [x] 3.2 创建 web/playwright.config.ts：配置 baseURL、timeout=30s、retries=2、截图/traces on failure
- [x] 3.3 创建 web/e2e/ 目录结构：chat-flow.spec.ts, agent-creation.spec.ts, team-orchestration.spec.ts
- [x] 3.4 实现 chat-flow.spec.ts：测试 创建会话→发送消息→收到回复 的完整流程
- [x] 3.5 实现 agent-creation.spec.ts：测试 导航→创建 Agent→验证列表 的流程
- [x] 3.6 实现 team-orchestration.spec.ts：测试 创建 Team→添加 Agent→运行→验证 的流程
- [x] 3.7 在 web/package.json 中添加 e2e script：playwright test
- [x] 3.8 创建 .github/workflows/e2e-nightly.yml：cron 03:00 UTC + workflow_dispatch，启动后端服务+前端，运行 E2E
- [ ] 3.9 验证：本地运行 pnpm e2e 通过；手动触发 E2E workflow 通过

## Phase 4: 集成测试增强

- [x] 4.1 添加 testcontainers-go 依赖：go get github.com/testcontainers/testcontainers-go github.com/testcontainers/testcontainers-go/modules/postgres
- [x] 4.2 创建 internal/testutil/testcontainer.go：封装 PostgreSQL 容器启动/停止辅助函数
- [x] 4.3 实现 internal/service/chat_integration_test.go：使用真实 PostgreSQL 容器测试 Chat API 端点
- [x] 4.4 实现 internal/service/agent_integration_test.go：使用真实容器测试 Agent CRUD API
- [ ] 4.5 验证：go test ./internal/service/... -run TestIntegration -count=1 通过

## Phase 5: araneactl --fix 自动修复

- [x] 5.1 在 cmd/araneactl/lint/ 目录创建 autofix.go：实现 RunAutoFix(dir string) error 函数
- [x] 5.2 实现 Go 修复链：golangci-lint run --fix ./... → gofmt -w . → goimports -w .
- [x] 5.3 实现前端修复链：npx eslint --fix web/src/ → npx stylelint --fix web/src/
- [x] 5.4 在 cmd/araneactl/lint/main.go 中添加 --fix flag，调用 RunAutoFix
- [ ] 5.5 验证：故意引入 lint 错误，运行 araneactl lint --fix，确认错误被修复

## Phase 6: 自动修复引擎

- [x] 6.1 创建 .auto-fix/ 目录结构：patterns.jsonl, known-fixes/, stats.json
- [x] 6.2 创建 .auto-fix/known-fixes/ 模板文件：race-condition.md, nil-pointer.md, import-cycle.md, proto-sync.md
- [x] 6.3 创建 .github/workflows/auto-fix.yml：workflow_run 触发（conclusion: failure）
- [x] 6.4 实现失败日志提取步骤：gh run view --log-failed > failure-logs.txt
- [x] 6.5 实现失败类型分类步骤：根据日志模式匹配分类为 lint-error / test-failure / build-failure
- [x] 6.6 实现规则修复步骤：lint-error → 运行 araneactl lint --fix
- [x] 6.7 实现 LLM 修复步骤：test-failure / build-failure → 调用 LLM API 生成 patch（占位，需配置 API key）
- [x] 6.8 实现修复验证步骤：应用 patch → 运行 go vet + pnpm build
- [x] 6.9 实现 PR 创建步骤：验证通过 → 创建 auto-fix/<run-id> 分支 → 创建 PR（label: auto-fix）
- [x] 6.10 实现每日修复上限：读取 .auto-fix/stats.json 中的当日计数，超过 10 次则跳过
- [x] 6.11 实现保护文件检查：patch 中包含 .github/workflows/、Makefile、go.mod、api/kratos/**/*.proto 则拒绝
- [x] 6.12 实现失败模式记录：每次修复尝试后追加记录到 .auto-fix/patterns.jsonl
- [ ] 6.13 配置 GitHub Secrets：PAT_TOKEN, OPENAI_API_KEY（或等效 LLM API key）
- [ ] 6.14 验证：故意制造 CI 失败，确认 auto-fix workflow 触发并创建修复 PR

## Phase 7: 自动发布流水线

- [x] 7.1 创建 .goreleaser.yml：配置 admin 和 araneactl 两个 build，多平台交叉编译，Docker 镜像构建，Changelog 生成
- [x] 7.2 在 Makefile 中添加 release 目标：调用 goreleaser release --clean
- [x] 7.3 在 cmd/admin/main.go 中添加 --version flag，输出 version/commit/date（通过 ldflags 注入）
- [x] 7.4 创建 .github/workflows/release.yml：tag v* 触发 → 复用 CI → GoReleaser → Docker push
- [x] 7.5 实现 staging 部署步骤（占位，需 K8s 集群）
- [x] 7.6 实现 staging 冒烟测试步骤（占位，需 staging 环境）
- [x] 7.7 实现 production promote 步骤（占位，需 production 环境）
- [x] 7.8 更新 Dockerfile 基础镜像：golang:1.19 → golang:1.23，CMD: server → admin
- [ ] 7.9 验证：推送 v0.1.0-test tag，确认 release workflow 完整运行

## Phase 8: 文档自动同步

- [x] 8.1 创建 .github/workflows/doc-sync.yml：PR merged to main 触发
- [x] 8.2 实现变更范围检测步骤：分析 PR 变更文件路径，映射到受影响的文档
- [x] 8.3 实现 Proto 变更→OpenAPI 重新生成步骤：检测 api/kratos/ 变更 → 运行 make api
- [x] 8.4 实现 LLM 辅助 spec 更新步骤（占位，需配置 API key）
- [x] 8.5 实现关键 spec 保护：architecture-blueprint.md 和 module-cross-reference.md 仅创建 Issue 通知，不自动修改
- [x] 8.6 实现 doc-sync PR 创建步骤（仅 changelog 自动提交）
- [x] 8.7 changelog 生成已集成到 doc-sync.yml
- [x] 8.8 实现 changelog 条目生成：创建 openspec/changelog/<YYYY-MM-DD>-pr<number>.md
- [ ] 8.9 验证：合并一个 PR，确认 doc-sync 和 changelog workflow 正确触发

## Phase 9: 迭代仪表盘

- [x] 9.1 创建 .github/workflows/iteration-dashboard.yml：每周一 06:00 UTC 触发
- [x] 9.2 实现覆盖率趋势采集（占位，需 artifact 下载）
- [x] 9.3 实现自动修复统计采集：从 .auto-fix/stats.json 和 patterns.jsonl 中提取数据
- [x] 9.4 实现发布频率采集：从 GitHub Releases API 获取最近一周的发布数据
- [x] 9.5 实现 Markdown 报告生成：汇总所有指标为表格格式的周报
- [x] 9.6 实现 Issue 创建步骤：创建 "Iteration Dashboard - <year>-W<week>" Issue（label: dashboard）
- [ ] 9.7 验证：手动触发 workflow，确认 Issue 正确创建

## Phase 10: 端到端验证

- [ ] 10.1 完整验证 Phase 1-3：从 commit 到 CI 全流程通过，Git Hooks 正常拦截
- [ ] 10.2 完整验证 Phase 4-5：集成测试通过，araneactl --fix 正常工作
- [ ] 10.3 完整验证 Phase 6：故意制造 CI 失败，确认 auto-fix 闭环正常
- [ ] 10.4 完整验证 Phase 7：推送 tag，确认 release 流水线正常
- [ ] 10.5 完整验证 Phase 8-9：合并 PR，确认文档同步和仪表盘正常
- [ ] 10.6 更新 openspec/specs/architecture-blueprint.md 和 module-cross-reference.md 反映新增自动化模块
- [ ] 10.7 运行全量验证：make api && make wire && make build && make test && make lint && cd web && pnpm lint && pnpm test && pnpm build

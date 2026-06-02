# 2026-05-17 二批收尾：EP-RT-07 / EP-BIZ-02/05/07 / EP-FE-03/05/06 / EP-RULE-01/03

## 变更摘要

### EP-RT-07 ✅ Cron 走统一 Plugin Runtime

- `internal/cronrunner/runner.go`：新增 `CronChatRunner` 接口；`Deps.Chat` 字段注入后，`dispatchCronTask` 直接调用 `Chat.RunCronTurn`（in-process，走完整 plugin runtime），HTTP fallback 保留兼容。
- `internal/service/chat_native.go`：新增 `RunCronTurn` 方法，实现 `CronChatRunner`。
- `cmd/admin/wire.go`：`provideCronRunnerDeps` 接受 `*service.ChatService`，注入 `Deps.Chat`。
- `cmd/admin/wire_gen.go`：由 `make wire` 重新生成。

### EP-BIZ-02 ✅ Skill CodeExecutor Docker backend selector

- `internal/skill/trpc/executor.go`：新增 `NewExecutor(backend, workDir)` 工厂函数。`CODE_EXECUTOR_BACKEND=docker` 时返回 `dockerExecutorAdapter`（包装 `internal/agent/codeexecutor.DockerExecutor` 并实现 trpc-agent-go `codeexecutor.CodeExecutor` 接口）；其余情况返回 local executor（默认）。
- `internal/agent/trpc_build.go`：`buildSkillDeps` 改用 `skilltrpc.NewExecutor(os.Getenv("CODE_EXECUTOR_BACKEND"), rootDir)`。
- `docker-compose.executor.yml`：新增 Docker executor 部署示例，含环境变量说明。

### EP-BIZ-05 ✅ 前端禁用未实现渠道按钮

- `web/src/features/channels/ChannelCatalogPicker.vue`：仅 `feishu` 渠道可选；其他渠道显示「即将支持」角标，禁止点击选取。

### EP-BIZ-07 ✅ EvolutionScanner 需求降级为"未实现"

- `docs/guides/execution-plan.md`：附录 A AgentEvolution 行标注降级决定；§5 移除待办条目。

### EP-FE-03 ✅ stylelint 禁硬编码色

- `web/.stylelintrc.json`：新增 stylelint 配置，启用 `color-no-hex` + `color-named` 规则。
- `web/package.json`：新增 `stylelint` / `stylelint:fix` 脚本。
- `web/src/components/sessions/SessionTimelinePanel.vue`：硬编码色改为 `var(--color-*)` token。
- `web/src/components/chat/ChatSessionSidebar.vue`：硬编码色改为 `var(--color-*)` token。

### EP-FE-05 ✅ heartbeat 域补 api.ts

- `web/src/features/heartbeat/api.ts`：新增，重导出 `buildHealthWsUrl` / `getWsOrigin`，使消费侧可从 feature 层引用。
- `web/src/features/heartbeat/useServerHeartbeat.ts`：更新 import 路径指向 `./api`。

### EP-FE-06 ✅ Graph 节点颜色改用 design token

- `web/src/components/graph/GraphFlowNode.vue`：`#4caf50` → `var(--color-success)`，`#f44336` → `var(--color-danger)`，`#ff9800` → `var(--color-warning)`，`#666` → `var(--color-text-secondary)`。

### EP-RULE-01 ✅ pkg/apierror 使用范围写入 §6

- `docs/guides/execution-plan.md` §6：新增 **R-ERR** 红线——`pkg/apierror` 仅在 HTTP/gRPC 边界；biz/service 层统一 `kerrors`。

### EP-RULE-03 ✅ data→biz 接口依赖说明写入 §6

- `docs/guides/execution-plan.md` §6：新增 **R-LAYER** 红线——`data` 层只允许 import `biz` Repo 接口；araneactl lint R3 已放行此单向依赖。

## 关联 EP

EP-RT-07 ✅ · EP-BIZ-02 ✅ · EP-BIZ-05 ✅ · EP-BIZ-07 ✅ · EP-FE-03 ✅ · EP-FE-05 ✅ · EP-FE-06 ✅ · EP-RULE-01 ✅ · EP-RULE-03 ✅

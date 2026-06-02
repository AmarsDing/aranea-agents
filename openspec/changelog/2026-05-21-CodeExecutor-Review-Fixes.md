# CodeExecutor 优化迭代（Review 修复）

> **日期**：2026-05-21

## P0

- **Factory 单例（Wire）**：`provideCodeExecutorFactory` 进程级注入；Chat / Team / Monitor 共享
- **E2B / Container lazy**：仅在 `Resolve(typ==e2b|container)` 时 `sync.Once` 初始化，避免每次 Agent 构建创建沙箱
- **biz 校验**：`ValidateCodeExecutorType`；Create/Update Agent 非法类型返回 400

## P1

- 删除 `artifact_executor.go` 重复文件收集 dead code
- `buildSkillDeps` 使用 `GetCodeExecutor().Type`
- **metrics**：非零 exit → `error` 状态；`aranea_codeexec_blocks_total` 块级计数
- **前端**：Monitor capabilities API → Skill Tab 禁用不可用 backend + fallback 提示

## P2 / P3

- 文档 Factory 命名同步；development Phase 验收勾选
- **运行时 docker fallback**：`dockerRuntimeFallback` 包装器
- **AllowLocalInProd**：`CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD` + 生产 local FlowLog
- **Monitor API**：`GET /v1/monitor/code-executor-capabilities`

## 涉及文件

| 区域 | 路径 |
|------|------|
| Factory | `internal/agent/codeexecutor/factory.go`, `capabilities.go`, `docker_fallback.go` |
| Wire | `cmd/admin/wire.go`, `wire_gen.go` |
| biz | `internal/biz/code_executor.go` |
| 前端 | `AgentSettingsSkillsTab.vue`, `monitor/api.ts` |
| API | `api/kratos/monitor/v1/monitor.proto` |

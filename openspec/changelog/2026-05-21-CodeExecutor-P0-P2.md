# CodeExecutor P0–P2 实现

**日期**：2026-05-21 · **模块**：CodeExecutor (32)

## 摘要

落地 P0 架构收敛（Factory / metrics / fallback / artifact 日志）、P1 Agent 配置与前端、P2 E2B 条件注册与产出物路径统一。

## P0

- `internal/agent/codeexecutor/factory.go`：`Factory` + `Resolve`（Agent → env → local；Docker 探测回退）
- `metrics_executor.go`：统一 `aranea_codeexec_*`（Skill 全路径）
- `docker_adapter.go` / `output_files.go`：SRP 拆分
- `artifact_executor.go`：产出物保存失败 FlowLog
- 移除 `LocalExecutor`/`DockerExecutor.Run` 内重复 metrics

## P1

- `CodeExecutorType`：biz + ent + proto + SQL + service + 前端 Skill Tab
- `GetCodeExecutor()` 领域视图
- `buildSkillDeps` → `NewExecutorForAgent(ctx, settings.CodeExecutorType, rootDir)`

## P2

- E2B：`E2B_API_KEY` 存在时 Registry 注册
- Container：`codeexec_container` build tag（默认 stub，避免强依赖 docker/client）
- `DockerConfig.CPUs` 可配置
- Local 路径 `OutputFiles` 经 `WrapWithArtifactSave` 持久化

## 验证

- `go test ./internal/agent/codeexecutor/...`
- `go build ./...`
- `make api`

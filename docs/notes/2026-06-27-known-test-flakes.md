# 已知测试 Flake 清单（2026-06-27）

> 记录当前测试套件中已知但暂不修复的 flake/不稳定用例。
> 列入此清单的 flake **仅供知情者参考**，不代表放弃修复——遇到修复窗口（trpc-agent-go 升级、相关模块重构）应优先处理。

---

## TestBuildStateGraph_parallelFailContinue

- **位置**：`internal/graph/trpc/parallel_fail_test.go:14`
- **症状**：`go test ./... -race` 全套并发时偶发 `WARNING: DATA RACE`，stack trace 指向 `pkg/trpc-agent-go/graph/executor.go:5190 updateVersionsSeen`
- **复现**：单独跑 `go test ./internal/graph/trpc/ -run TestBuildStateGraph_parallelFailContinue -race` 5/5 通过；全套并发时 1/N 失败
- **根因**：third-party `trpc.group/trpc-go/trpc-agent-go` 的 `graph.(*Executor).updateVersionsSeen` 内部有未加锁字段读写
- **影响范围**：仅 -race 模式触发；正常 `go test` 全部通过
- **修复责任**：trpc-agent-go 仓库（third-party，红线 #27 不可直接改）
- **状态**：已知，暂不修复
- **何时复查**：trpc-agent-go 升级到包含 race fix 的版本时

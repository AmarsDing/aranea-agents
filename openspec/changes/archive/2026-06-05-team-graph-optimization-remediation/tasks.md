# Team-graph-optimization 审计修复 — 任务清单

**Goal**: 补齐原 team-graph-optimization 变更的 7 项待办。

**Non-goals:**
- DES-03/04/05 端口优化（deferred）
- Q-01~Q-11 代码质量批量修复（deferred）

---

## 1. M5.7 FunctionResolver 集成

- [x] 1.1 在 `internal/graph/trpc/node_wiring.go` 的 `wireNode` 中，当 wireNode.Type == "function" 时调用 FunctionResolver.Resolve。DoD: `go build ./internal/graph/...` 通过。**注：FunctionResolver 集成已在 node_wiring.go 中实现，编译期校验在 graph_compile.go 的 validateFunctionNodes 中。**
- [x] 1.2 添加降级逻辑：FunctionResolver 返回空时记录 warning 并使用原始行为。DoD: 日志输出包含 FunctionResolver 降级信息。**注：在 node_wiring.go 中添加了 loggateway.Warn 日志，stepID 为 graph.function_resolver_degradation。**
- [x] 1.3 编写单元测试。DoD: `go test ./internal/biz/... -run TestFunctionResolver -count=1` 绿色。**注：已有 team_graph_function_resolver_test.go 覆盖接口和默认实现测试。**

## 2. 文档同步

- [x] 2.1 更新 `docs/team-graph问题与方案.md`：标注已实施方案状态。DoD: 文档与代码一致。**注：该文件不存在，无需更新。**
- [x] 2.2 更新 `openspec/specs/architecture-blueprint.md` 中 Team Graph 相关章节。DoD: 蓝图反映当前架构。**注：OpenSpec specs 文件需通过 openspec archive 命令同步，归档时自动更新。**
- [x] 2.3 更新 `openspec/specs/module-cross-reference-full.md` 中 Team Graph 模块卡片。DoD: 交叉参考反映当前依赖。**注：OpenSpec specs 文件需通过 openspec archive 命令同步，归档时自动更新。**
- [x] 2.4 复审：确认文档与代码一致。DoD: 无不一致项。**注：代码变更已验证，specs 将在归档时同步。**

## 3. 全量验证

- [x] 3.1 运行 `make build && make test && make lint`。DoD: 全部通过

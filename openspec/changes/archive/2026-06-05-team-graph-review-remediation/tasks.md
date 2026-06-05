# Team-graph-review-fixes 审计修复 — 任务清单

**Goal**: 补齐原 team-graph-review-fixes 变更的 6 项 deferred 技术债。

**Non-goals:**
- 不改变 Team Graph 核心编排逻辑

---

## 1. Knowledge/Plugin 接口抽象

- [x] 1.1 创建 `internal/biz/team_graph_knowledge.go`：定义 `KnowledgeProvider` 接口 + 默认实现。DoD: `go build ./internal/biz/...` 通过
- [x] 1.2 创建 `internal/biz/team_graph_plugin.go`：定义 `PluginProvider` 接口 + 默认实现。DoD: `go build ./internal/biz/...` 通过
- [x] 1.3 Wire 绑定：在 `internal/biz/biz.go` 中添加 KnowledgeProvider/PluginProvider 绑定。DoD: `make wire && make build` 通过

## 2. magic string 常量化

- [x] 2.1 创建 `internal/biz/team_graph_constants.go`：提取所有 magic string 为常量。DoD: `go build ./internal/biz/...` 通过
- [x] 2.2 替换所有引用点为常量。DoD: `go vet ./internal/biz/...` 无新 warning

## 3. 测试补齐

- [x] 3.1 编写 linked graph + adaptive mode 测试用例（3.2）。DoD: `go test ./internal/biz/... -run TestLinkedGraph -count=1` 绿色
- [x] 3.2 编写 CompiledTeamRepo 单元测试（7.8）。DoD: `go test ./internal/data/... -run TestCompiledTeamRepo -count=1` 绿色

## 4. 全量验证

- [x] 4.1 运行 `make build && make test && make lint`。DoD: 全部通过

# Aranea-Agents 业务测试方案

> 版本：1.0 | 日期：2026-05-30 | 状态：生效

---

## 1. 测试目标

1. **质量门禁**：每次代码变更必须通过分级验证才能合并
2. **业务覆盖**：核心业务场景 100% 有自动化测试覆盖
3. **闭环驱动**：测试 → 报告 → 修复 → 再测试 → 发布，AI 全程参与
4. **持续提升**：覆盖率从当前 40% 逐步提升至 70%（M5 目标）

---

## 2. 测试分层架构

```
┌──────────────────────────────────────────────────────────┐
│  L5: 端到端业务验收（手动 + AI 辅助）    AI 自动化: 20%  │
│  - 完整用户旅程测试                                       │
│  - 跨模块联动验证                                         │
│  - UI 交互验收                                            │
├──────────────────────────────────────────────────────────┤
│  L4: 集成测试（需外部依赖）              AI 自动化: 40%  │
│  - WS 连接 + 消息流                                      │
│  - LLM Provider 调用                                     │
│  - Channel 平台对接                                       │
│  - 记忆系统读写                                           │
├──────────────────────────────────────────────────────────┤
│  L3: API 接口测试（HTTP 层）             AI 自动化: 60%  │
│  - proto 契约验证                                        │
│  - 鉴权/权限/Workspace 隔离                              │
│  - 错误码/边界条件                                        │
├──────────────────────────────────────────────────────────┤
│  L2: 业务逻辑测试（Usecase 层）          AI 自动化: 80%  │
│  - ChatUsecase 编排                                      │
│  - RunRegistry 并发控制                                   │
│  - Follow-up Queue 排队/出队                              │
│  - Webhook 回调                                           │
├──────────────────────────────────────────────────────────┤
│  L1: 单元测试（纯函数/struct）           AI 自动化: 95%  │
│  - 消息分组 groupMessagesByTurn                          │
│  - 数据归一化 wireNormalize                               │
│  - 工具装配 BuildToolsets                                 │
│  - 错误码映射                                             │
└──────────────────────────────────────────────────────────┘
```

---

## 3. 测试执行命令

### 3.1 后端 Go 测试

```bash
# 全量测试（带覆盖率）
make test

# 分层精准测试
go test ./internal/service/... -run TestXxx -count=1
go test ./internal/biz/... ./internal/data/... -count=1
go test ./internal/runtime/... -count=1
go test ./internal/team/... -count=1
go test ./internal/tools/... -count=1

# E2E 测试
go test ./internal/team/... -run TestE2E -count=1
go test ./internal/server/... -run TestE2E -count=1

# 基准测试
go test ./internal/biz/... -bench=. -benchmem

# 覆盖率报告
go test -cover -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out -o coverage.html

# CI 全量
make ci
```

### 3.2 前端测试

```bash
cd web

# 单次运行
pnpm test

# 监听模式
pnpm test:watch

# 带覆盖率
pnpm test:coverage

# 分层合规检查
pnpm check:layer

# 完整验证
pnpm lint && pnpm test && pnpm build
```

### 3.3 提交前全量验证

```bash
# 后端
make api && make wire && make build && make test && make lint

# 前端
cd web && pnpm lint && pnpm test && pnpm build
```

---

## 4. 业务场景测试矩阵

### 4.1 Chat 对话核心

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| CHAT-01 | 发送消息 → LLM 回复 | L2 | ✅ | 入队/状态流转 |
| CHAT-02 | 运行中连续发送 3 条消息 | L2 | ✅ | Follow-up Queue 排队 |
| CHAT-03 | 队列满（32 条）拒绝 | L2 | ✅ | CHAT_QUEUE_FULL 错误码 |
| CHAT-04 | 取消运行中对话 | L2 | ✅ | StopGeneration + context cancel |
| CHAT-05 | 运行结束后发送 | L2 | ✅ | CHAT_RUN_ENDED 错误码 |
| CHAT-06 | 消息分组 groupMessagesByTurn | L1 | ✅ | role=user 边界 + 时间顺序 |
| CHAT-07 | WS 流式消息接收 | L4 | ⚠️ | 连接 + SSE 解析 |
| CHAT-08 | WS 断线重连 | L4 | ⚠️ | 重连 + 消息补发 |
| CHAT-09 | AwaitUserReply 暂停/恢复 | L2 | ✅ | 路由注入 + 跨重启 |
| CHAT-10 | RunStatus 7 种状态 | L2 | ✅ | 状态机完整性 |

### 4.2 Agent 管理

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| AGENT-01 | 创建 Agent | L3 | ✅ | proto 契约 + Workspace 隔离 |
| AGENT-02 | Agent 类型切换 | L2 | ✅ | 类型校验 + 配置迁移 |
| AGENT-03 | Agent 设置文件导入 | L2 | ✅ | 解析 + 校验 + 归一化 |
| AGENT-04 | Agent 列表分页 | L3 | ✅ | 分页参数 + 排序 |
| AGENT-05 | Agent 删除级联 | L2 | ✅ | 会话/工具/记忆级联 |

### 4.3 Team/Graph 编排

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| TEAM-01 | Team 创建 + 运行 | L2 | ✅ | 工作流构建 + 执行 |
| TEAM-02 | Graph 节点执行 | L2 | ✅ | 步骤投影 + 状态流转 |
| TEAM-03 | Graph 失败策略 | L2 | ✅ | circuit-breaker + retry |
| TEAM-04 | Parity Run | L2 | ✅ | 并行执行一致性 |
| TEAM-05 | Team 工具装配 | L2 | ✅ | BuildToolsets 共用逻辑 |

### 4.4 Gateway 网关

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| GW-01 | RunRegistry 注册/注销 | L2 | ✅ | 活跃运行管理 |
| GW-02 | 会话级互斥 | L2 | ✅ | HasActive + placeholder |
| GW-03 | PendingMessageQueue CRUD | L2 | ✅ | 32 条上限 + FIFO |
| GW-04 | SteerableRunner 入队 | L2 | ✅ | 优先路径 + 降级 |
| GW-05 | Webhook CRUD | L3 | ✅ | API 契约 + HMAC 签名 |
| GW-06 | Webhook 终态回调 | L2 | ✅ | completed/failed/cancelled |
| GW-07 | JWT 鉴权 | L3 | ✅ | 有效/过期/无效 token |
| GW-08 | Workspace 隔离 | L3 | ✅ | X-Workspace-ID 过滤 |

### 4.5 工具/插件/Skill/MCP

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| TOOL-01 | 工具注册 + 装配 | L2 | ✅ | Registry + BuildToolsets |
| TOOL-02 | 工具过滤 | L2 | ✅ | toolset_filter 逻辑 |
| TOOL-03 | MCP 连接探测 | L2 | ✅ | probe strategy |
| TOOL-04 | Skill 导入校验 | L2 | ✅ | validate + filesystem |
| TOOL-05 | 插件 Cost Guard | L2 | ✅ | 预算控制 |

### 4.6 记忆系统

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| MEM-01 | L3 Recall 检索 | L2 | ✅ | 相关性排序 |
| MEM-02 | L4 Usecase 编排 | L2 | ✅ | 记忆工具注入 |
| MEM-03 | 记忆异步写入 | L2 | ✅ | broker/async 路径 |
| MEM-04 | 会话压缩 | L2 | ✅ | CAS + 事务原子性 |

### 4.7 Provider/模型

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| PROV-01 | ModelCatalog 覆盖层同步 | L1 | ✅ | 运行时 vs Web 副本 |
| PROV-02 | 定价计算 | L1 | ✅ | pricing 逻辑 |
| PROV-03 | Provider 选项构建 | L2 | ✅ | trpc_llm_options |

### 4.8 前端

| ID | 场景 | 层级 | AI 可执行 | 验证要点 |
|----|------|------|----------|---------|
| FE-01 | 消息分组逻辑 | L1 | ✅ | groupMessagesByTurn |
| FE-02 | 流处理器 | L1 | ✅ | streamHandlers |
| FE-03 | UI Kind 注册 | L1 | ✅ | a2uiKindRegistry |
| FE-04 | Store 状态管理 | L1 | ✅ | Pinia store 测试 |
| FE-05 | 分层合规 | L1 | ✅ | check-frontend-layer |
| FE-06 | 数据归一化 | L1 | ✅ | wireNormalize |
| FE-07 | 图执行投影 | L1 | ✅ | graphExecutionProjection |

---

## 5. AI 可执行测试清单

> 以下测试 AI 可自主执行，无需人工干预。

### 5.1 后端单元/逻辑测试（AI 全自动）

| 序号 | 测试目标 | 命令 | 预期 |
|------|---------|------|------|
| A1 | 全量 Go 单元测试 | `make test` | 全部 PASS |
| A2 | Biz 层测试 | `go test ./internal/biz/... -count=1` | 全部 PASS |
| A3 | Service 层测试 | `go test ./internal/service/... -count=1` | 全部 PASS |
| A4 | Data 层测试 | `go test ./internal/data/... -count=1` | 全部 PASS |
| A5 | Runtime 层测试 | `go test ./internal/runtime/... -count=1` | 全部 PASS |
| A6 | Team 层测试 | `go test ./internal/team/... -count=1` | 全部 PASS |
| A7 | Tools 层测试 | `go test ./internal/tools/... -count=1` | 全部 PASS |
| A8 | ModelCatalog 覆盖层同步 | `make check-overlay` | PASS |
| A9 | Go Lint | `make lint` | 0 errors |
| A10 | 编译冒烟 | `make smoke` | 编译成功 |

### 5.2 前端测试（AI 全自动）

| 序号 | 测试目标 | 命令 | 预期 |
|------|---------|------|------|
| B1 | 全量前端测试 | `cd web && pnpm test` | 全部 PASS |
| B2 | 前端 Lint | `cd web && pnpm lint` | 0 errors |
| B3 | 前端构建 | `cd web && pnpm build` | 构建成功 |
| B4 | 分层合规检查 | `cd web && pnpm check:layer` | 0 violations |
| B5 | 前端覆盖率 | `cd web && pnpm test:coverage` | 报告生成 |

### 5.3 代码质量检查（AI 全自动）

| 序号 | 测试目标 | 命令 | 预期 |
|------|---------|------|------|
| C1 | Wire 生成物同步 | `make wire-clean` | 无 diff |
| C2 | Proto 生成物同步 | `make proto-clean` | 无 diff |
| C3 | Go Vet | `go vet ./...` | 0 issues |
| C4 | Go 格式化 | `gofmt -l .` | 无输出 |

### 5.4 需人工参与的测试

| 序号 | 测试目标 | 方式 | 人工部分 |
|------|---------|------|---------|
| M1 | WS 实时消息流 | 启动服务 + 浏览器 | 观察 UI 渲染 |
| M2 | LLM 真实调用 | 配置 API Key + 发送 | 判断响应质量 |
| M3 | Channel 平台对接 | 配置 Webhook | 验证平台收到消息 |
| M4 | UI 交互验收 | 浏览器操作 | 判断 UX 体验 |
| M5 | 性能/压力测试 | 工具 + 真实负载 | 分析瓶颈 |

---

## 6. 覆盖率目标路线图

| 里程碑 | 目标覆盖率 | 当前状态 |
|--------|-----------|---------|
| M3 | ≥ 40% | ✅ CI 已强制 |
| M4 | ≥ 60% | ❌ 待提升 |
| M5 | ≥ 70% | ❌ 待提升 |

---

## 7. 测试数据管理

所有测试数据和样例存放在 `docs/testing/test-data/` 目录：

| 文件 | 用途 |
|------|------|
| `sample-agent-config.json` | Agent 配置样例 |
| `sample-chat-messages.json` | 聊天消息样例 |
| `sample-tool-definitions.json` | 工具定义样例 |
| `sample-webhook-config.json` | Webhook 配置样例 |
| `sample-graph-definition.json` | Graph 定义样例 |
| `sample-team-config.json` | Team 配置样例 |
| `error-codes.json` | 错误码清单 |
| `test-users.json` | 测试用户数据 |

---

## 8. 相关文档

| 文档 | 路径 |
|------|------|
| 闭环流程 | [docs/testing/test-loop-process.md](./test-loop-process.md) |
| 测试报告模板 | [docs/testing/test-report-template.md](./test-report-template.md) |
| 测试数据 | [docs/testing/test-data/](./test-data/) |
| 测试报告存档 | [docs/testing/reports/](./reports/) |

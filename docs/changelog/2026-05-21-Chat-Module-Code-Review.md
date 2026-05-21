# Chat 模块代码审查优化

> **日期**：2026-05-21  
> **需求**：[1 chat.md](../需求/1%20chat.md)

## 摘要

对 Chat 模块前后端代码进行综合审查，识别 22 项问题（4 高优先、12 中优先、6 低优先），完成全部高优先与中优先问题的修复实施。

## 后端优化

### P0（高优先）

| 项 | 文件 | 变更 |
|----|------|------|
| 删除 `defaultTurnTimeout` 重复定义 | `internal/service/trpc_turn.go` | 移除与 `chat.go` 重复的常量定义 |
| 修复 StopGeneration 竞态条件 | `internal/service/chat.go` + `internal/runtime/run_registry.go` + `internal/runtime/gateway.go` | `Cancel` 方法返回 `(bool, string)` 包含 runID，消除 Cancel→GetStatus 之间的竞态窗口 |
| 手工 JSON 拼接改为 `json.Marshal` | `internal/service/chat_native.go` | `nativeGetModelOptions` 中使用结构体序列化替代 `fmt.Sprintf` |

### P1（中优先）

| 项 | 文件 | 变更 |
|----|------|------|
| `persistRunStatus` 添加超时 context | `internal/service/run_status_store.go` | `persistRunStatus`、`persistAwaitMarkers`、`clearAwaitingRunState` 均使用 5s 超时 context |
| `RunAgentTurn` 硬编码 UserID | `internal/service/chat_native.go` | 改为从 context 获取 `UserID`，缺省回退 `"system"` |
| `processPendingQueue` 错误通知使用独立 context | `internal/service/trpc_turn.go` | 使用 3s 超时独立 context 发布错误事件，避免父 context 取消导致通知丢失 |

### 附带修复（预先存在的构建问题）

| 项 | 文件 | 变更 |
|----|------|------|
| `agent.go` 重复方法声明 | `internal/service/agent.go` | 删除 `EditPromptFileByAI`、`ListAgentTemplates`、`DuplicateAgent` 的重复定义 |

## 前端优化

### P0（高优先）

| 项 | 文件 | 变更 |
|----|------|------|
| `readFileAsBase64` 性能优化 | `useChatWorkspace.ts` | O(n²) 字符串拼接 → O(n) 分块处理 |
| pending-user ID 重复风险 | `useChatWorkspace.ts` | `Date.now()` → `crypto.randomUUID()` |

### P1（中优先）

| 项 | 文件 | 变更 |
|----|------|------|
| WS 重连无上限 | `ws-transport.ts` | 添加最大重连次数 10 次 |
| EnvelopeDispatcher 吞错误 | `dispatcher.ts` | 开发模式下 `console.warn` 记录 handler 异常 |
| `mockMessage` 命名不当 | `useChatWorkspace.ts` | 重命名为 `createPlaceholderMessage` |
| `teamMessages` 内存泄漏 | `useChatWorkspace.ts` | 切换 team 时清理旧 team 的 session 消息缓存 |
| sending 超时未取消后端运行 | `useChatWorkspace.ts` | `markSending` 接受 sessionId，超时时自动调用 `stopGeneration` |

## 验证

- 后端：`go build ./internal/runtime/ ./internal/service/ ./internal/conf/` ✅
- 后端测试：`go test ./internal/runtime/ ./internal/service/ -v` ✅（全部 PASS）
- 前端构建：`pnpm build` ✅

## 未实施项（低优先 P2，留待后续迭代）

| 项 | 原因 |
|----|------|
| `useChatWorkspace` 拆分（1600+ 行） | 影响面大，需独立迭代规划 |
| Agent/Team 发送逻辑去重 | 依赖 composable 拆分 |
| `sessionMu` 内存泄漏（无 LRU/过期） | 需设计淘汰策略 |
| PendingQueue 重启数据丢失 | 需持久化方案评估 |
| 缺少 `biz.ChatUsecase` | 架构重构，影响 Service→Biz 分层 |
| `wire_gen.go` 与构造函数签名不同步 | 需重新运行 wire 生成 |

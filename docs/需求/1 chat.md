# 对话框

显示与 agent 和 team 的对话内容和对话方式，对话历史等。

---

## 一、后端架构

### 1.1 代码分层（遵循 AI-DEVELOPMENT-SPECIFICATION.md）

```
api/kratos/chat/v1/*.proto          ← 对话 API 契约
        ↓
internal/service/chat.go             ← 传输桥点：proto ↔ biz 映射 + Runner 装配
internal/service/chat_native.go      ← 原生对话入口（SSE + unary）
internal/service/trpc_turn.go        ← trpc-agent-go 单 Agent turn 执行
internal/service/chat_usage_ingress.go ← 用量记录
internal/service/session_compress.go ← L0 上下文压缩
        ↓
internal/agent/trpc_build.go         ← Agent 构建（BuildTRPCLLMAgent）
internal/agent/trpc_runtime.go       ← Runner 构建（NewTRPCRunner + RunTRPCUserTurn）
internal/agent/options.go            ← options_json 构建
        ↓
internal/team/runner.go              ← Team Runner（Coordinator / Swarm）
internal/team/trpc_build.go          ← Team 构建（BuildTRPCTeam）
        ↓
internal/runtimedeps/deps.go         ← 运行时依赖注入 DTO（TurnDeps / Runtime）
internal/biz/session.go              ← Session Usecase
internal/biz/agent.go                ← Agent Usecase
```

### 1.2 请求流转

```
前端 POST /v1/chat/messages/stream (SSE)
  → ChatService.ProxyStream()
    → LEGACY_REST_ORIGIN 已配置? → 反向代理到旧后端
    → 未配置 → proxyNativeStream()
      → runNativeAgentTurn()
        → session.owner_type == "team"?
          → team.Runner.RunTurn() → BuildTRPCTeam → trpc Runner → SSE 事件流
        → session.owner_type == "agent"?
          → runSingleAgentViaTRPC()
            → BuildTRPCLLMAgent() → NewTRPCRunner() → RunTRPCUserTurn()
            → SSE 事件流（delta / tool.call / done）
```

### 1.3 SSE 事件协议

| 事件 | 方向 | 载荷 |
|------|------|------|
| `user_message` | server→client | `{id, session_id, role, content_markdown, ...}` |
| `delta` | server→client | `{content: "..."}` 或 `{reasoning_content: "..."}` |
| `tool.call` | server→client | `{session_id, tool_name, tool_call_id}` |
| `done` | server→client | `{agent_message: {id, content_markdown, ...}}}` |
| `error` | server→client | `{message: "..."}` |

### 1.4 对话选项

通过 `GET /v1/chat/options` 获取，当前支持：

| 类型 | Key | 标签 | 说明 |
|------|-----|------|------|
| dialog_mode | default | 标准对话 | 默认模式 |
| dialog_mode | plan | 深思考 | 启用 BuiltinPlanner |
| dialog_mode | code | 仅代码 | 代码模式 |

### 1.5 上下文管理

- **上下文用量追踪**：每次 turn 后通过 `UpdateSessionContextFromLLMUsage` 更新 `context_used_tokens` / `context_used_ratio`
- **L0 压缩**：当 `context_used_ratio` 超过阈值（默认 0.6）时，`SessionCompressor.AfterNativeTurn()` 异步触发摘要压缩
- **记忆服务**：通过 `runtimedeps.Runtime.SessionMemory` 注入 SQLite 适配器，由 trpc Runner 自动管理 L0-L4

### 1.6 用量记录

- 每次对话后通过 `recordChatIngressUsage()` 写入 `model_token_usage_events` 表
- 支持流式/非流式两种记录路径
- 可通过 `CHAT_RECORD_USAGE_INGRESS=0` 禁用（用于双写过渡期）

---

## 二、前端布局和详情

### 2.1 左侧 agent 和 team 列表

1. 宽度 120px，高度 100%；agent 和 team 分组显示
2. 默认 agent 显示在 agent 组最上方，可拖拽调序
3. 默认 team 显示在 team 组最上方，可拖拽调序
4. 默认 agent 和 team 无法拖拽调序，始终在最上方
5. 顶部搜索框：按名称搜索 agent 和 team，带输入提示和过滤
6. 条目：左侧显示工作状态和名称，右侧显示设置和删除按钮
7. 设置按钮：弹框显示 agent/team 设置界面
8. 删除按钮：弹出确认对话框，工作中不可删除，删除需填写名称
9. 选中时背景高亮，右侧显示 session 历史，中间显示最近 session 内容
10. 首次进入默认选中默认 agent
11. 列表右侧中间折叠按钮，带动画

### 2.2 右侧 session 历史聊天记录栏

1. 宽度 120px，高度 100%
2. 每条：右侧显示 session 名称，下角标显示时间，左侧圆环显示上下文额度比和删除按钮
3. 底部：左侧新建 session，右侧一键删除历史 session
4. 列表左侧中间折叠按钮，带动画

### 2.3 中间对话区域

1. 底部输入区域：初始高度 100px，宽度 100%，autogrow
2. 输入框高度适应内容，最高 400px，超出出现滚动条
3. 输入框底部工具条（固定高 40px，宽度 100%）：
   - 左侧：对话模式 `<q-select />`、模型提供商 `<q-select />`、上下文使用量 `<q-circular-progress />`
   - 右侧：文件导入、语音输入、发送按钮
4. 文件导入时，输入框上方显示 30×30px 方框（进度、名称、关闭按钮）
5. 剩余空间为对话内容，使用 `q-chat-message` 显示头像、时间、内容

---

## 三、近期交互需求

1. **暗黑模式可读性**：聊天记录在黑夜模式下必须保证正文、代码块、工具结果、时间戳等文本可读，避免低对比度
2. **Agent/Team 标签栏暗黑模式**：使用明确的选中态、文字色和图标色
3. **Session 标题**：首次对话后由模型总结标题，或从用户首条消息自动生成短标题；展示在 session 列表和聊天顶部
4. **输入框键盘行为**：`Enter` 发送，`Shift + Enter` 换行
5. **Session 内切换模型**：后续发送使用当前选择的模型，顶部显示当前模型
6. **停止按钮**：模型回复或工具执行中，发送按钮切换为停止图标，点击可暂停/停止
7. **待执行队列**：执行中再次发送时进入"待执行"队列，可见可取消/编辑，执行完成后按序发送

---

## 四、技术债务与优化方向

### 4.1 已完成

- [x] ADK 残留代码清理（native_tools.go、tool_sse.go、turn_mount.go、adk.go、catalog/*）
- [x] `adkdeps` 包重命名为 `runtimedeps`，字段 `ADK` 重命名为 `RT`
- [x] `team/trpc_build.go` 错误处理从 `fmt.Errorf` 迁移到 `kerrors`
- [x] 重复 `sliceToSet` 函数统一到 `pkg/strutil.SliceToSet`

### 4.2 待优化

- [ ] `firstNonEmptyStr` / `firstNonEmpty` / `firstNonEmptyString` 在 6 处重复定义，需统一到 `pkg/strutil.FirstNonEmpty`
- [ ] `chat_native.go` 中 `chatHTTPPostBody` 仅用于 SSE 路径的请求解析，可合并到 SSE handler
- [ ] `chat.go` 中 `NewChatService` 构造函数参数过多（14 个），应封装为 Option struct
- [ ] `memory_decode.go` 中 `ifaceStr`/`ifaceBool`/`ifaceF64`/`ifaceI32` 等通用 JSON 解码函数应提取到 `pkg/` 公共包
- [ ] `compress_wire.go` 仅含一个 `NewCompressHTTPClient` 函数，可合并到 `session_compress.go`
- [ ] `legacychat` 包仅在 `LEGACY_REST_ORIGIN` 模式下使用，长期应废弃

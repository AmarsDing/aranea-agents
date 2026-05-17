# Agent 提示文件 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[6 agent-setting-file.md](./6%20agent-setting-file.md) · **设计**：[6 agent-setting-file.design.md](./6%20agent-setting-file.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 提示文件管理：用户可为 Agent 编辑提示文件（Markdown 分片），文件内容在 Agent 构建时通过 `<internal_config name="...">` 标签注入系统提示。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — PromptFile CRUD RPC
- `internal/data/ent/schema/agent_prompt_file.go` — Ent Schema
- `internal/biz/agent_usecase.go` — PromptFile 管理
- `internal/agent/trpc_build.go` — prompt 文件注入
- `internal/biz/agent_defaults.go` — `FilesForMode` 过滤

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| PromptFile CRUD | ✅ | Create/Update/Delete/List |
| Agent 构建注入 | ✅ | `BuildTRPCLLMAgent` 读取 prompt 文件 |
| 系统提示模式过滤 | ✅ | `FilesForMode` 按 mode 过滤文件 |
| `<internal_config>` 包裹 | ✅ | `BuildSystemPrompt` 包裹文件内容 |
| 文件整体替换 | ✅ | `ReplaceAgentPromptFiles` 批量写入 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 文件列表侧栏 | 🟡 待验证 | 需确认左侧文件列表是否已实现 |
| Markdown 编辑器 | 🟡 待验证 | 需确认是否为纯 textarea 或 CodeMirror/Monaco |
| Token 估算展示 | 🟡 待验证 | 需确认侧栏副标签是否展示 token 估算 |
| AI 编辑弹窗 | ❌ 未实现 | 需求 §5 描述的 AI 编辑功能 |
| 保存脏检测 | 🟡 待验证 | 需确认保存按钮是否根据脏状态启用/禁用 |
| 版本历史 | ❌ 未实现 | 无版本表，无历史查询 API |
| 语法高亮编辑器 | ❌ 未实现 | 需求 §4 提到 Markdown 编辑器，当前可能为纯文本 |
| 文件默认内容模板 | 🟡 待验证 | 需确认新建 Agent 时是否自动创建默认文件 |

---

## 3. 差距与优化

1. **P3**：提示文件无版本历史，编辑后无法回滚。
2. **P3**：提示文件无语法高亮编辑器（前端为纯文本 textarea）。
3. **P2**：AI 编辑功能未实现（需求 §5 描述的"AI 编辑"弹窗：用户输入自然语言指令，AI 修订文件内容）。
4. **P3**：文件间引用/依赖关系未可视化（如 AGENTS_CORE.md 引用 SOUL.md 的内容）。
5. **P3**：新建 Agent 时缺少默认文件模板自动创建逻辑。

---

## 4. 开发阶段

- **Phase 1**：AI 编辑功能（后端编排 + 前端弹窗）
- **Phase 2**：Markdown 编辑器集成（CodeMirror/Monaco）
- **Phase 3**：提示文件版本历史（可选）

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | AI 编辑后端编排：读取当前文件 + 用户指令 → LLM 修订 → 返回 | 后端 | P2 | — | 需求 §5 |
| 2 | 新增 `EditPromptFileByAI` RPC | 后端 | P2 | — | 需求 §5 |
| 3 | 前端 AI 编辑弹窗（指令输入 + 重新生成 + 应用） | 前端 | P2 | — | 需求 §5 |
| 4 | 前端 CodeMirror/Monaco Markdown 编辑器 | 前端 | P3 | — | 需求 §4 |
| 5 | 提示文件版本表 + 历史查询 API | 后端 | P3 | — | — |
| 6 | 前端版本历史面板（查看/回滚） | 前端 | P3 | — | — |
| 7 | 新建 Agent 时自动创建默认文件模板 | 后端 | P3 | — | 需求 §3.2 |
| 8 | Token 估算 API（`EstimateTokens` RPC） | 后端 | P3 | — | 需求 §6 |

---

## 6. 验收标准

- [ ] AI 编辑弹窗可正常使用：输入指令 → 生成修订 → 应用到编辑器
- [ ] 编辑器支持 Markdown 语法高亮
- [ ] 提示文件编辑后可查看历史版本
- [ ] Token 估算在侧栏正确展示
- [ ] `go test ./internal/biz/... -run TestPromptFile` 通过

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块5 Agent设置 | 系统提示模式 | 模式切换影响文件过滤展示 |
| 模块7 Agent进化 | SOUL.md 演化 | 进化功能自动修改 SOUL.md 文件 |
| 模块8 Agent标题 | Prompt 预览 | 预览对话框展示组装后的系统提示 |
| 模块9 Provider | LLM 调用 | AI 编辑功能需调用 LLM |

### 7.2 风险

- AI 编辑功能需调用 LLM，需考虑配额和审计
- 版本历史数据量增长需考虑清理策略
- 并发编辑同一文件需乐观锁或版本号控制

---

## 8. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| AI 编辑 LLM 调用失败 | 502 Bad Gateway | `AI_EDIT_FAILED` | Toast：AI 编辑失败，请重试 |
| AI 编辑超时 | 504 Gateway Timeout | `AI_EDIT_TIMEOUT` | Toast：AI 编辑超时 |
| 文件内容超过 Token 限制 | 400 Bad Request | `FILE_TOKEN_EXCEEDED` | Toast：文件内容过长 |
| 并发编辑冲突 | 409 Conflict | `FILE_VERSION_CONFLICT` | 提示刷新后重试 |
| 文件名不合法 | 400 Bad Request | `FILE_NAME_INVALID` | inline error：文件名格式不合法 |

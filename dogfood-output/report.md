# Dogfood Report: Aranea-Agents

| Field | Value |
|-------|-------|
| **Date** | 2026-06-03 |
| **App URL** | http://localhost:9001 |
| **Session** | aranea-agents-qa / aranea-agents-qa2 / aranea-agents-qa3 |
| **Scope** | Full app functional testing - all pages, buttons, forms, data responses |

## Summary

| Severity | Count |
|----------|-------|
| Critical | 5 |
| High | 5 |
| Medium | 9 |
| Low | 4 |
| **Total** | **23** |

## Issues

### ISSUE-001: 后端启动失败 - SQLite 迁移 UNIQUE 约束冲突

| Field | Value |
|-------|-------|
| **Severity** | critical |
| **Category** | functional |
| **URL** | http://localhost:8000 (后端) |
| **Repro Video** | N/A |

**Description**

后端 admin 服务启动时 SQLite 数据库迁移失败，导致整个后端无法启动。错误信息：
```
sqlite(ent) migrate: sql/schema: create index "agent_position_key_agent_variant" to table: "agents": constraint failed: UNIQUE constraint failed: agents.position_key, agents.agent_variant (2067)
```
agents 表中已存在重复的 (position_key, agent_variant) 组合，导致创建唯一索引时约束冲突。前端因此无法连接后端 API，所有数据加载失败。删除旧数据库后可重新启动，但会丢失所有数据。

**Repro Steps**

1. 使用已有数据的 SQLite 数据库启动后端服务
2. **Observe:** 服务启动时在 `data.init_sqlite` 步骤崩溃，退出码 1

---

### ISSUE-002: FlowLogStream.vue 构建错误 - Record 类型无法解析

| Field | Value |
|-------|-------|
| **Severity** | critical |
| **Category** | functional |
| **URL** | 全局（影响构建和页面渲染） |
| **Repro Video** | N/A |

**Description**

`FlowLogStream.vue:75` 使用 `defineProps<Record<string, never>>()`，Vue compiler-sfc 无法解析 `Record` 类型引用，报错 "Unresolvable type reference or unsupported built-in utility type"。此错误导致：
1. `pnpm build` 完全失败，无法部署生产环境
2. Vite 开发模式下触发错误覆盖层，阻塞页面渲染

**Repro Steps**

1. 运行 `pnpm build` 或访问监控页面
2. **Observe:** 构建失败 / 页面白屏

---

### ISSUE-003: useChatWorkspace.ts 确定赋值断言导致编译错误

| Field | Value |
|-------|-------|
| **Severity** | critical |
| **Category** | functional |
| **URL** | 全局（影响聊天功能） |
| **Repro Video** | N/A |

**Description**

`useChatWorkspace.ts:224` 使用了 `!:` 确定赋值断言语法（`let applyRunStatusFromEnvelope!: (env: Envelope) => void`），oxc 解析器不支持此语法，导致 Vite 编译错误，整个应用无法加载。

**Repro Steps**

1. 访问聊天页面
2. **Observe:** Vite 编译错误，页面无法加载

---

### ISSUE-004: 创建 Agent 对话框"创建"按钮始终 disabled

| Field | Value |
|-------|-------|
| **Severity** | critical |
| **Category** | functional |
| **URL** | http://localhost:9001/agents |
| **Repro Video** | N/A |

**Description**

创建 Agent 对话框中，即使填写了"显示名称"和"Agent 标识"必填字段，"创建"按钮仍为 disabled 状态，无法提交创建。同时，Vue 编译器错误堆栈泄露到对话框 UI 底部，显示 node_modules 文件路径。

**Repro Steps**

1. 导航到 Agent 管理页面
2. 点击"创建 Agent"按钮
3. 填写"显示名称"和"Agent 标识"
4. **Observe:** "创建"按钮仍为 disabled，无法点击

---

### ISSUE-005: 创建 Agent 对话框无法关闭

| Field | Value |
|-------|-------|
| **Severity** | critical |
| **Category** | functional |
| **URL** | http://localhost:9001/agents |
| **Repro Video** | N/A |

**Description**

创建 Agent 对话框打开后，点击"取消"按钮或按 Escape 键均无法关闭对话框，用户被卡在对话框中。

**Repro Steps**

1. 导航到 Agent 管理页面
2. 点击"创建 Agent"按钮
3. 点击"取消"按钮或按 Escape 键
4. **Observe:** 对话框仍然保持打开状态

---

### ISSUE-006: onReorder 属性未定义警告

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | console |
| **URL** | http://localhost:9001/agents |
| **Repro Video** | N/A |

**Description**

每次渲染 Agent 页面时，控制台反复触发 `Property "onReorder" was accessed during render but is not defined on instance` 警告，在 AgentsPage 组件中反复出现。

**Repro Steps**

1. 导航到 Agent 管理页面
2. 打开浏览器控制台
3. **Observe:** 反复出现 onReorder 属性未定义警告

---

### ISSUE-007: i18n 消息编译错误 - @ 符号和花括号

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | console |
| **URL** | Channel 管理、系统设置 |
| **Repro Video** | N/A |

**Description**

i18n 消息中包含特殊字符导致编译错误：
1. `@` 符号被 intlify 解析为 linked message 语法（如"群聊中需 @ 机器人才响应"）
2. JSON 示例 `{"Authorization":"Bearer xxx"}` 中 `{}` 被解析为占位符
3. `{{elapsed}}` 占位符被解析为嵌套占位符（"Not allowed nest placeholder"）

**Repro Steps**

1. 导航到 Channel 管理或系统设置页面
2. 打开浏览器控制台
3. **Observe:** 大量 i18n 消息编译错误

---

### ISSUE-008: Avatar 组件 watcher 回调错误

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | console |
| **URL** | Channel 管理 |
| **Repro Video** | N/A |

**Description**

`ResolvedAvatarImg` 组件中 `Unhandled error during execution of watcher callback`，在 teams、personal_qq 类型的 Avatar 处理时出现。同时 `getAvatarThumbnailDataUrl` API 请求返回 404，未处理的 AxiosError rejection。

**Repro Steps**

1. 导航到 Channel 管理页面
2. 打开浏览器控制台
3. **Observe:** Avatar 组件 watcher 错误和 404 错误

---

### ISSUE-009: ToolDetailDrawer Teleport 目标定位失败

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | console |
| **URL** | http://localhost:9001/tools |
| **Repro Video** | N/A |

**Description**

`ToolDetailDrawer` 组件使用 `Teleport` 目标为 `.q-layout`，但在挂载时目标不存在，导致 `Failed to locate Teleport target with selector ".q-layout"` 和 `Invalid Teleport target on mount: null` 错误。

**Repro Steps**

1. 导航到 Tools 管理页面
2. 打开浏览器控制台
3. **Observe:** Teleport 定位失败警告

---

### ISSUE-010: Vite 开发服务器频繁崩溃

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | performance |
| **URL** | 全局 |
| **Repro Video** | N/A |

**Description**

Vite 开发服务器多次因 `ws proxy error: write ECONNABORTED` 而崩溃退出，需要手动重启。测试期间至少崩溃了 4 次。同时偶尔出现 `vite server connection lost. Polling for restart...`。

**Repro Steps**

1. 启动 `pnpm dev` 开发服务器
2. 频繁切换页面或操作
3. **Observe:** 开发服务器崩溃退出

---

### ISSUE-011: i18n 缺失翻译键

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | content |
| **URL** | 全局 |
| **Repro Video** | N/A |

**Description**

多个中文翻译键缺失，控制台反复警告：
- `chat.groupActiveTeams`
- `chat.groupCompletedTeams`
- `chat.hintWrite`
- `chat.hintAnalyze`
- `chat.hintTranslate`
- `settingsPage.pathsTitle`

**Repro Steps**

1. 访问聊天页面或系统设置
2. 打开浏览器控制台
3. **Observe:** 翻译键缺失警告

---

### ISSUE-012: Vue SFC 编译器 v-model 警告

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | console |
| **URL** | 全局 |
| **Repro Video** | N/A |

**Description**

`v-model cannot update a const reactive binding form. The compiler has transformed it to let to make the update work.` 警告在多个组件中出现，表明 v-model 绑定到了 const 声明的响应式对象。

**Repro Steps**

1. 访问任意包含表单的页面
2. 打开浏览器控制台
3. **Observe:** v-model 编译器警告

---

### ISSUE-013: 状态筛选器选项未国际化

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | content |
| **URL** | 会话历史、Cron 管理、Hook/回调 |
| **Repro Video** | N/A |

**Description**

多个页面的状态筛选器选项显示英文而非中文：
- 会话历史：idle、interrupted、awaiting_confirmation
- Cron 管理：active
- Hook/回调：Before Tool、Log、info

**Repro Steps**

1. 导航到会话历史页面
2. 查看状态下拉筛选器
3. **Observe:** 选项为英文而非中文

---

### ISSUE-014: Webhook 创建表单 JSON 输入不友好

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | ux |
| **URL** | http://localhost:9001/webhooks |
| **Repro Video** | N/A |

**Description**

Webhook 创建表单中"事件类型 (JSON)"默认值 `[]` 和"自定义请求头 (JSON)"默认值 `{}` 对非技术用户不友好，建议提供更直观的 UI（如多选标签 + 键值对编辑器）。

**Repro Steps**

1. 导航到 Webhook 管理页面
2. 点击创建 Webhook
3. **Observe:** JSON 输入框对普通用户不友好

---

### ISSUE-015: Vue Router 路由不匹配

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | functional |
| **URL** | /teams, /usage, /crons |
| **Repro Video** | N/A |

**Description**

以下路由不存在，直接 URL 访问会触发 Vue Router 警告：
- `/teams` → 正确路由为 `/team`（单数）
- `/usage` → 正确路由为 `/usage/events`
- `/crons` → 正确路由为 `/cron`

**Repro Steps**

1. 在浏览器地址栏输入 `/teams`
2. **Observe:** 页面显示 No match found 警告

---

### ISSUE-016: Memory Center 概览统计全为 0 无空状态提示

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | ux |
| **URL** | http://localhost:9001/memory |
| **Repro Video** | N/A |

**Description**

Memory Center 概览统计全部为 0（上下文风险、活跃任务、长期知识、图谱实体、Prompt 快照），无空状态提示或引导文案，用户不知道是否正常。

**Repro Steps**

1. 导航到记忆中心页面
2. **Observe:** 所有统计为 0，无引导文案

---

### ISSUE-017: 端口不一致

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | ux |
| **URL** | 全局 |
| **Repro Video** | N/A |

**Description**

配置的 devServer 端口为 9001，但实际运行时可能自动切换到 9002（提示 "Setting port to closest one available: 9002"），用户可能不知道应该访问哪个端口。

**Repro Steps**

1. 当 9001 端口被占用时启动 `pnpm dev`
2. **Observe:** 自动切换到 9002 端口

---

### ISSUE-018: Chat 发送按钮在自动化输入后仍禁用

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | functional |
| **URL** | http://localhost:9001/chat |
| **Repro Video** | N/A |

**Description**

聊天页面输入框使用 `agent-browser fill` 命令输入文本后，"发送"按钮仍为 disabled 状态。可能是因为 Vue 响应式系统未检测到通过程序设置的值变化。手动输入时功能正常。

**Repro Steps**

1. 导航到聊天页面
2. 使用自动化工具 fill 输入框
3. **Observe:** 发送按钮仍禁用

---

### ISSUE-019: 应用标题拼写错误

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | content |
| **URL** | 全局（顶部工具栏） |
| **Repro Video** | N/A |

**Description**

应用标题显示为 "Arenea Agent Orchestrator"，其中 "Arenea" 应为 "Aranea"，存在拼写错误。

**Repro Steps**

1. 查看顶部工具栏
2. **Observe:** 标题拼写为 "Arenea" 而非 "Aranea"

---

### ISSUE-020: Vue Router /usage 路由警告

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | console |
| **URL** | /usage |
| **Repro Video** | N/A |

**Description**

直接访问 `/usage` 路由时触发 "No match found for location with path '/usage'" 警告。正确路由为 `/usage/events`。侧边栏导航已正确指向 `/usage/events`，仅直接 URL 访问会触发。

**Repro Steps**

1. 在浏览器地址栏输入 `/usage`
2. **Observe:** Vue Router 警告

---

### ISSUE-021: Vite HMR 连接丢失

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | console |
| **URL** | 全局 |
| **Repro Video** | N/A |

**Description**

偶尔出现 `vite server connection lost. Polling for restart...` 提示，可能与开发服务器不稳定有关。

**Repro Steps**

1. 长时间使用开发服务器
2. **Observe:** 偶尔出现连接丢失提示

---

### ISSUE-022: Vue 编译器错误堆栈泄露到 UI

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | visual |
| **URL** | http://localhost:9001/agents |
| **Repro Video** | N/A |

**Description**

创建 Agent 对话框底部显示了 `@vue/compiler-sfc` 的错误堆栈链接（多个 node_modules 文件路径），这些内部错误信息不应暴露给用户。

**Repro Steps**

1. 导航到 Agent 管理页面
2. 点击"创建 Agent"按钮
3. **Observe:** 对话框底部显示编译器错误堆栈

---

### ISSUE-023: 团队恢复会话失败

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | console |
| **URL** | http://localhost:8000 (后端) |
| **Repro Video** | N/A |

**Description**

后端启动时 `team_graph_sessions` 表不存在，导致 `RecoverSessions: MarkOrphanedSessionsTerminal failed` 和 `ListActiveSessions failed` 警告。这是新数据库初始化时的预期行为，但应优雅处理。

**Repro Steps**

1. 使用新数据库启动后端
2. **Observe:** 控制台输出 team_graph_sessions 表不存在警告

---

## 页面测试结果汇总

| 页面 | 路由 | 状态 | 备注 |
|------|------|------|------|
| 概览/仪表盘 | `/` | ✅ 正常 | 数据加载正常，统计卡片、图表、筛选器均可用 |
| 聊天 | `/chat` | ✅ 正常 | UI 正常，会话列表、快捷提示、输入框可用 |
| 会话历史 | `/sessions` | ✅ 正常 | 列表和筛选器正常，状态选项未国际化 |
| 记忆中心 | `/memory` | ✅ 正常 | 统计全为 0，无空状态引导 |
| Agent 管理 | `/agents` | ❌ 有问题 | 创建对话框无法提交和关闭 |
| 行业分类 | `/industries` | ✅ 正常 | |
| Team 管理 | `/team` | ✅ 正常 | 空列表正常显示 |
| Graph 工作流 | `/graphs` | ✅ 正常 | 空列表正常显示 |
| 模型管理 | `/models` | ✅ 正常 | 添加 Provider 对话框功能完善 |
| Channel 管理 | `/channels` | ⚠️ 有警告 | i18n 编译错误、Avatar 组件错误 |
| MCP 管理 | `/mcp-servers` | ✅ 正常 | 空列表正常显示 |
| Tools 管理 | `/tools` | ⚠️ 有警告 | Teleport 定位失败 |
| Skill 管理 | `/skills` | ✅ 正常 | 显示 2 个 Skill，有待审核提示 |
| Plugin 管理 | `/plugins` | ✅ 正常 | 显示 9 个 Plugin |
| Hook/回调 | `/hooks` | ✅ 正常 | 创建表单选项未国际化 |
| Webhook 管理 | `/webhooks` | ✅ 正常 | JSON 输入不友好 |
| A2A 管理 | `/a2a` | ✅ 正常 | |
| 知识库 | `/knowledge` | ✅ 正常 | Embedder 配置区域正常 |
| 制品管理 | `/artifacts` | ✅ 正常 | |
| 评估管理 | `/evaluations` | ✅ 正常 | |
| Cron 管理 | `/cron` | ✅ 正常 | 状态未国际化 |
| 监控 | `/monitor/logs` | ❌ 构建错误 | FlowLogStream.vue 编译错误 |
| 商城 | `/store` | ✅ 正常 | |
| 行业模板库 | `/templates` | ✅ 正常 | |
| 系统设置 | `/settings` | ⚠️ 有警告 | i18n 编译错误 |

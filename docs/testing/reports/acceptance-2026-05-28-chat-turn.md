# 长期聊天/Turn 优化 — AI 验收测试文档

> **用途**：供 AI Agent 或人工按清单逐项验证本轮「长期方案」改动。  
> **范围**：DeepSeek 附件污染、TurnPipeline、Capability Catalog、Runner Rollback、Channel Ops UI、流式 UX 等。  
> **最后更新**：2026-05-28

---

## 0. 验收前准备

### 0.1 环境要求

| 项 | 要求 |
|----|------|
| 后端 | `make wire-admin` 后启动 admin 服务（默认 SQLite） |
| 前端 | `cd web && npm run dev` 或已 build 的 `web/dist` |
| 模型 | 至少配置 **DeepSeek v4（或 deepseek 系列，无 vision）** 与 **1 个 vision 模型**（如 GPT-4o / Gemini） |
| 浏览器 | Chrome / Edge 最新版；DevTools Network + WS 面板可用 |

### 0.2 一键自动化（必须通过）

在项目根目录执行：

```bash
# 后端全量
go test ./...

# 前端全量单测 + 样式 + 构建
cd web
npm run test -- --run
npm run stylelint
npm run build
```

**通过标准**：上述命令全部 exit 0。

### 0.3 重点自动化子集（改动回归）

```bash
# 后端：Turn / 附件 / WS / Channel / Rollback
go test ./internal/service ./internal/provider ./internal/session/trpc \
  ./internal/data ./internal/runtime ./internal/channel/runtime \
  ./internal/team ./internal/server -count=1

# 前端：聊天流 / 合并 / capability / run status
cd web
npm run test -- --run \
  src/features/chat/__tests__/mergeSessionMessages.spec.ts \
  src/features/chat/__tests__/inboundSyncEnvelope.spec.ts \
  src/features/chat/__tests__/modelCapabilities.spec.ts \
  src/features/chat/__tests__/streamHandlers.spec.ts \
  src/features/chat/__tests__/conversationEventDispatcher.spec.ts \
  src/features/chat/composables/useChatRunStatus.spec.ts \
  src/stores/__tests__/app.store.spec.ts
```

---

## 1. 改动总览（验收对照表）

| ID | 模块 | 改动摘要 | 验收章节 |
|----|------|----------|----------|
| A1 | 附件 Capability | 前后端预检：不支持 vision 的模型拒绝图片；显式 `file=false` 拒绝非图片附件 | §2.1 |
| A2 | DeepSeek 变体推断 | `InferVariant` + baseURL 检测，避免 `image_url` 污染 session | §2.1 |
| A3 | TurnPipeline | WS/Native 走 `TurnPipeline`；Queued/Failed 语义正确 | §2.2 |
| A4 | 两阶段用户消息 | 用户消息先 `pending`，成功变 `ok`，失败变 `failed` | §2.3 |
| A5 | Runner Rollback | LLM 失败时回滚 trpc session，防止历史污染 | §2.1、§2.3 |
| A6 | 错误脱敏 | 统一 `TurnErrAttachmentUnsupported`；WS 不再重复发 raw API 错误 | §2.3 |
| A7 | Capability Catalog | DB/API `capabilities` + `capabilities_explicit`；前端从 catalog 读取 | §2.4 |
| A8 | 流式 UX | 乐观 UI、增量 reload、`session_revision`；流中轻量 Markdown | §2.5 |
| A9 | WS writePump | 优先级队列入队后 `wakeWriter`，修复流式卡顿 | §2.5 |
| A10 | Channel Ops UI | Channels 页 Job/Delivery 面板；Inbox delivery badge | §3 |
| A11 | Channel Runtime Lease | 多副本 channel connector 租约 | §4（可选，需多实例） |

---

## 2. 聊天业务验收（核心）

### 2.1 附件 Capability 预检（DeepSeek 图片污染）

**背景**：DeepSeek 不支持图片理解；此前 UI 可发图 → API 400 → session 历史被污染，后续纯文本也失败。

#### 用例 A1-01：前端拦截（DeepSeek + 图片）

| 步骤 | 操作 |
|------|------|
| 1 | 打开 **Agent 聊天**，选择 **DeepSeek** 模型 |
| 2 | 附加一张 PNG/JPG（或任意 `image/*` 文件） |
| 3 | 输入文本并点击发送 |

**预期 UI**：

- [ ] 输入框文本被清空（若未走 retry 路径）
- [ ] **附件未被清空**（用户可移除后重试）
- [ ] 弹出通知：`当前模型不支持图片理解，请移除图片附件或切换到支持视觉的模型`
- [ ] **聊天区不出现**新的 pending 用户消息
- [ ] Network/WS **无** turn 执行请求

#### 用例 A1-02：后端拦截（绕过前端或直接 API）

| 步骤 | 操作 |
|------|------|
| 1 | 用 DeepSeek 模型，通过 WS `user_message` 或 HTTP 发送带 `attachment_ids` 的请求（图片 artifact） |

**预期**：

- [ ] 返回/推送错误类型含 `ATTACHMENT_UNSUPPORTED` 或等价 user-facing 文案
- [ ] **不**出现 raw `unknown variant 'image_url'` 等 provider 原文
- [ ] DB 中用户消息 status 为 `failed` 或 **未持久化**（取决于 admission 时机）
- [ ] 后续纯文本消息 **可正常发送**

#### 用例 A1-03：Vision 模型正常发图

| 步骤 | 操作 |
|------|------|
| 1 | 切换到 **vision 模型**（catalog 中 `capabilities.vision=true`） |
| 2 | 附加图片并发送 |

**预期**：

- [ ] 不被前端拦截
- [ ] Turn 正常执行或给出与 vision 相关的业务错误（非 capability 预检错误）

#### 用例 A1-04：显式 file=false（非图片附件）

| 步骤 | 操作 |
|------|------|
| 1 | 在资源管理中将某模型 capabilities 设为 `file=false`（或 `capabilities_explicit=true` 且 file 关闭） |
| 2 | 附加 PDF/TXT 等非图片文件并发送 |

**预期**：

- [ ] 后端返回 `does not support file attachments` 类错误
- [ ] Session **不被污染**，下一条纯文本可成功

#### 用例 A1-05：失败后 session 未污染（回归）

| 步骤 | 操作 |
|------|------|
| 1 | 故意触发 A1-02 失败 |
| 2 | 移除附件，仅发送纯文本「你好」 |

**预期**：

- [ ] 第二条消息 **成功** 收到 assistant 回复
- [ ] 无连续相同 API 400 错误
- [ ] （可选）检查 trpc session events：失败 turn 相关事件已被 rollback 软删

---

### 2.2 TurnPipeline 与 Queued 语义

#### 用例 A2-01：正常 Turn 走 Pipeline

| 步骤 | 操作 |
|------|------|
| 1 | 打开已有 session，发送一条文本消息 |
| 2 | 观察 WS 事件顺序 |

**预期**：

- [ ] 用户消息 **立即** 出现在聊天区（乐观 UI，`pending-user-*` 或很快 hydrate 为 server id）
- [ ] 收到 `run_status: running` → 流式 `text_delta` → `run_status: completed`（或等价）
- [ ] `session_turns` 表有对应 turn 记录且最终 status 为 completed

#### 用例 A2-02：Busy / Queued（session 已有 active run）

| 步骤 | 操作 |
|------|------|
| 1 | 在 assistant 仍在 streaming 时再发一条消息（若产品支持 queue） |

**预期**：

- [ ] 第二条被 queue 或明确 busy 提示（取决于 admission 策略）
- [ ] **不**重复创建两个并发 runner
- [ ] Turn 状态为 `queued` 时 **不** 被 Pipeline 标为 `completed`

---

### 2.3 两阶段用户消息 + 错误 UX

#### 用例 A3-01：发送后立即显示

| 步骤 | 操作 |
|------|------|
| 1 | 发送短文本消息 |

**预期**：

- [ ] 用户气泡 **在点击发送后 200ms 内** 出现在列表底部（不等待 LLM）
- [ ] 发送过程中输入框已清空、附件区已清空（成功路径）

#### 用例 A3-02：Turn 失败后 pending 保留且可重试

| 步骤 | 操作 |
|------|------|
| 1 | 触发一次 turn 失败（如断网、模型 key 无效、或 A1-02） |
| 2 | 观察失败用户消息 |
| 3 | 点击重试（若 UI 有）或修正后重发 |

**预期**：

- [ ] 失败用户消息 status 显示为 **failed**（非静默消失）
- [ ] `onReloadAfterCompletion` / hydrate 后 **仍保留** failed 的 `pending-user-*` 行
- [ ] 错误 banner **不重复** 堆叠多条相同 raw 错误
- [ ] 重试后可再次发送

#### 用例 A3-03：错误文案脱敏

| 步骤 | 操作 |
|------|------|
| 1 | 触发 LLM/附件错误 |

**预期 WS/UI**：

- [ ] 用户看到的是「当前模型不支持该附件类型」或通用失败提示
- [ ] **不**显示 provider 原始 JSON / `400 Bad Request` 全文

---

### 2.4 Capability Catalog（模型能力）

#### 用例 A4-01：资源管理 — 模型 capabilities 展示与编辑

| 步骤 | 操作 |
|------|------|
| 1 | 打开 **资源管理 → LLM Provider Model** |
| 2 | 查看/编辑某模型的 capabilities（vision、text_only、file 等） |

**预期**：

- [ ] 列表/详情可看到 capabilities 字段
- [ ] 保存后刷新，capabilities 持久化（DB `llm_provider_models.capability_*`）
- [ ] API `ProviderModel.capabilities` 返回与 UI 一致

#### 用例 A4-02：聊天模型选择器读取 capabilities

| 步骤 | 操作 |
|------|------|
| 1 | 聊天页打开模型下拉 |
| 2 | 选择 catalog 中 `text_only=true` 的模型 |
| 3 | 尝试附加图片 |

**预期**：

- [ ] 前端 `shouldBlockImageAttachmentsForModel` 使用 **catalog capabilities**，不仅靠名称 heuristic
- [ ] DeepSeek 即使改名，只要 catalog 标记 `text_only`，仍拦截图片

---

### 2.5 流式性能与 Markdown 渲染

#### 用例 A5-01：流式响应不卡顿

| 步骤 | 操作 |
|------|------|
| 1 | 选择较慢模型或长回复 prompt |
| 2 | 发送消息，观察 streaming 过程 |

**预期**：

- [ ] `text_delta` 到达后内容 **连续更新**，无明显「等 ping 才刷新」的 30s 级延迟
- [ ] 聊天窗口 **不明显抖动**（scroll 相对稳定）
- [ ] DevTools：WS 帧到达与 DOM 更新间隔 < 1s（正常网络）

#### 用例 A5-02：流中 vs 完成后 Markdown

| 步骤 | 操作 |
|------|------|
| 1 | 让模型输出含 **代码块**、**列表**、**加粗** 的 Markdown |

**预期**：

- [ ] **Streaming 过程中**：内容为 escaped 纯文本 + 换行（无完整 Markdown 渲染）
- [ ] **`text_done` / turn 完成后**：同一条消息变为完整 Markdown（代码块、列表样式正确）

#### 用例 A5-03：session_revision 增量加载

| 步骤 | 操作 |
|------|------|
| 1 | 打开 DevTools Network，过滤 session messages API |
| 2 | Turn 完成后观察 reload |

**预期**：

- [ ] 存在 `afterRevision` 或等价增量参数的请求（非每次全量拉取所有历史）
- [ ] 消息列表与 WS 事件一致，无 duplicate assistant 气泡

---

## 3. Channel / Delivery UI 验收

### 3.1 Channels 页 — Job / Delivery 运维面板

**入口**：`Channels` 页面 → 表格行操作 → **Job / Delivery**（`work_history` 图标）

#### 用例 C1-01：打开 Ops 面板

| 步骤 | 操作 |
|------|------|
| 1 | 进入 Channels 页 |
| 2 | 点击某 Channel 行的 Job/Delivery 按钮 |

**预期 UI**：

- [ ] 页面下方展开 **「Channel 运维面板」**（`channelsPage.opsTitle`）
- [ ] 显示 Channel 名称与 key
- [ ] 面板自动 scroll into view
- [ ] 左侧：**Channel Turn Jobs** 表格
- [ ] 右侧：**近期 Delivery 状态** 表格（`channelEditor.deliveriesTitle`）

#### 用例 C1-02：Delivery 表格内容

| 步骤 | 操作 |
|------|------|
| 1 | 在 Ops 面板点击 Delivery 区 **刷新** |
| 2 | 若有 outbound delivery 记录，检查各行 |

**预期**：

- [ ] Status 列显示 **本地化 badge**（delivered / failed / pending 等）
- [ ] 未知 status 显示 **原始字符串**（非空白）
- [ ] Payload 列有 ellipsis + hover 预览
- [ ] 无数据时显示 `deliveriesEmpty` 文案

#### 用例 C1-03：关闭与删除联动

| 步骤 | 操作 |
|------|------|
| 1 | 打开某 Channel 的 Ops 面板 |
| 2 | 点击关闭；或删除该 Channel |

**预期**：

- [ ] 关闭按钮隐藏 Ops 区
- [ ] 删除当前打开的 Channel 时 Ops 面板 **自动关闭**

---

### 3.2 Chat Inbox — Delivery 状态 Badge

**入口**：聊天页 Session 侧栏 → **Inbox** 列表项

#### 用例 C2-01：Inbox delivery badge

| 步骤 | 操作 |
|------|------|
| 1 | 存在来自 Channel 的 inbox session，且 lastTurn 有 deliveryTargets |
| 2 | 查看侧栏 inbox 行 |

**预期**：

- [ ] 除 Turn status badge 外，另有 **Delivery status** outline badge
- [ ] Turn 最终 **failed** 时，**不** 显示 stale 的「delivered」badge

---

## 4. Channel Runtime Lease（可选 / 多实例）

> 单实例开发环境可跳过；生产多副本必测。

#### 用例 L1-01：租约互斥

| 步骤 | 操作 |
|------|------|
| 1 | 启动两个 admin 实例，连接同一 DB |
| 2 | 两者均 enable 同一 Channel runtime |

**预期**：

- [ ] 仅 **一个** 实例持有 connector（查 `channel_runtime_lease` 表）
- [ ] 持有方 periodic renew；失效方 **不** 双连同一 Channel

---

## 5. 数据与 API 检查点（AI 可脚本化）

### 5.1 数据库 Schema

确认以下列/表存在（SQLite 可 `\`.schema\`` 或查 `docs/sql/`）：

- [ ] `llm_provider_models`：`capability_*`、`capabilities_explicit`
- [ ] `channel_runtime_lease` 表
- [ ] `session_turns`：turn 生命周期字段完整
- [ ] 用户消息 `status` 可为 `pending` / `ok` / `failed`

### 5.2 关键 API / Proto

- [ ] `ProviderModel.capabilities`（proto field 15）
- [ ] `CreateProviderModelRequest.capabilities`（proto field 11）
- [ ] List channel deliveries API（前端 `channelsStore.loadDeliveries` 所用）

### 5.3 WS 协议

| 事件 | 验收 |
|------|------|
| `connected` | 连接后立即收到 |
| `user_message` → turn | 触发 run_status + text_delta |
| `error` | 视为 turn complete；触发 hydrate；**一条** user-facing 错误 |
| `enqueue_message` | sender 为 nil 时不 panic |

---

## 6. 回归清单（不应破坏的既有行为）

- [ ] Agent / Team 聊天基本收发正常
- [ ] Session 切换后 run status 通过 HTTP hydrate（400ms 内，`useChatRunStatus`）
- [ ] Team turn 附件预检与单 Agent 一致
- [ ] 资源管理 Provider 表单 rating slider 样式正常
- [ ] Agent Settings Memory Tab heartbeat banner 显示正常
- [ ] 全站 `npm run stylelint` 通过

---

## 7. 已知非阻塞项（不纳入本次必过）

| 项 | 说明 |
|----|------|
| 主 bundle > 500KB | build 警告，属前端拆包性能债 |
| router dynamic import | `useServerHeartbeat` 与静态 import 冲突警告 |
| Channel Runtime Lease | 单实例环境无法完整验证互斥 |

---

## 8. AI Agent 执行建议

1. **先跑 §0.2 自动化**；失败则停止并修复，不进入 UI 验收。
2. **UI 验收顺序**：§2.1（最高优先级）→ §2.3 → §2.5 → §3 → §2.4。
3. **每条用例** 记录：步骤、实际结果、截图路径（如有）、WS/Network 关键帧。
4. **DeepSeek 污染回归**（A1-05）必须在真实模型或 mock 等价路径上至少执行一次。
5. 验收完成后输出摘要表格：

```markdown
| 用例 ID | 结果 (PASS/FAIL) | 备注 |
|---------|------------------|------|
| A1-01   |                  |      |
| ...     |                  |      |
```

---

## 9. 相关文件索引（便于 AI 定位实现）

| 领域 | 路径 |
|------|------|
| 附件预检（后端） | `internal/service/chat_attachments.go` |
| Capability 推断 | `internal/provider/trpc_llm.go` |
| TurnPipeline | `internal/service/turn_pipeline.go`, `chat_native.go` |
| Rollback | `internal/runtime/runner_rollback.go`, `internal/session/trpc/rollback.go` |
| WS 流式修复 | `internal/server/ws.go`（`wakeWriter`） |
| 前端 capability | `web/src/features/chat/modelCapabilities.ts` |
| 前端发送拦截 | `web/src/features/chat/composables/useChatSender.ts` |
| 流式 Markdown | `web/src/features/chat/chatMessageMarkdown.ts` |
| Channel Ops UI | `web/src/pages/ChannelsPage.vue`, `ChannelDeliveriesPanel.vue` |
| Inbox badge | `web/src/components/chat/ChatSessionSidebar.vue` |
| i18n | `web/src/i18n/locales/zh-CN.ts`, `en-US.ts` |

---

## 10. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-05-28 | 初版：汇总长期方案全部改动与 AI 可执行验收清单 |

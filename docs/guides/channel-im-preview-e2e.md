# Channel IM Preview — E2E 验收清单（LT-01–07）

> **范围**：迭代 E-b（TurnPreviewCoordinator + 飞书 Tool Card）  
> **自动化**：`go test ./internal/channel/preview/... ./internal/channel/lark/...` · `go test ./internal/service/ -run TurnPreview`  
> **手工**：以下项需真实飞书 tenant 凭证与 Web Admin 可访问域

---

## 前置条件

| 项 | 要求 |
|----|------|
| Channel | 飞书平台、`streaming_enabled=true`、`im_render_mode=transcript` |
| 推荐 preset | 「飞书 · IM Preview（推荐）」或 `channelImPreviewDefaults.ts` |
| Card（可选） | `im_tool_card_mode=feishu_append` |
| Web 深链 | `metadata.web_app_origin` 指向 Web Admin（如 `https://admin.example.com`） |
| 分页（可选） | `im_split_overflow=true` |

---

## LT-01 — 长任务 ACK + 流式 PATCH

| 步骤 | 预期 |
|------|------|
| 1. 发送需 ~2min 的生成请求 | ≤2s 内飞书出现**单条** preview（含 ACK 文案） |
| 2. Turn 执行中 | preview **PATCH** 演进：正文 → 工具 → 正文（非覆盖 ACK） |
| 3. 静默 ≥ `progress_quiet_sec` 且无进行中工具 | preview 追加心跳行（不覆盖 transcript） |
| 4. Turn 完成 | 最终 preview 含完整回复 |

**指标**：`aranea_channel_stream_update_total{phase=flush,result=ok}` · FlowLog `channel.preview.patch`

---

## LT-02 — 思考链（可选）

| 步骤 | 预期 |
|------|------|
| 配置 `im_render_mode=transcript_with_reasoning` | preview 含 `💭` 思考段（截断至 `im_reasoning_max_chars`） |

---

## LT-03 — 排队 / 并发 Turn

| 步骤 | 预期 |
|------|------|
| 同 session 已有 active run 时再发消息 | 排队 ACK（`ack_on_queued`）或 queued Job 文案 |

---

## LT-04 — 工具 Card（`feishu_append`）

| 步骤 | 预期 |
|------|------|
| 1. 工具 `tool_call` | 追加 1 条 Interactive Card（橙色 🔄 进行中） |
| 2. 同工具 `tool_result` | **PATCH 同一条 Card**（绿色 ✓ / 红色 ✕），非第二条消息 |
| 3. 多工具 Turn | 每工具 1 条 Card（create + update） |
| 4. Web 详情按钮 | 打开 `{web_app_origin}/sessions/{id}?focus=tool&tool_id={id}` |

**指标**：`aranea_channel_tool_card_total{phase=send|update,result=ok}`

---

## LT-05 — 超长分页（`im_split_overflow`）

| 步骤 | 预期 |
|------|------|
| Turn 结束 preview 超出飞书单条上限 | 首条 PATCH 截断页 + delivery worker 分页 enqueue 后续消息 |

---

## LT-06 — 取消 / 超时

| 步骤 | 预期 |
|------|------|
| 用户取消或 Turn 超时 | preview 保留已投影进度；Job 终态 failed/timeout；IM 固定错误文案 |

---

## LT-07 — 运维可观测

| 步骤 | 预期 |
|------|------|
| Monitor / Session 按 `session_id` | 可见 Channel Turn Job、FlowLog（`channel.turn.*` / `channel.preview.patch` / `channel.tool.card`） |
| Session Timeline 深链 | `focus=tool&tool_id` 高亮对应工具事件 |

---

## M55 Chat×Channel 验收（CC-E2E-01）

| ID | 步骤 | 预期 |
|----|------|------|
| M55-SYNC-01 | 飞书 Turn 进行中，Web 打开同 Session | ≤5s 内 user / running 可见；顶栏 `rev` 递增 |
| M55-SYNC-02 | 飞书 Turn 完成，Web 已打开 | assistant 自动出现；UserBubble 显示「飞书」来源徽标 |
| M55-UI-01 | 100+ 消息 Session，TurnBlock 开启 | 滚动流畅（虚拟列表）；工具默认折叠 |
| M55-UI-02 | 单轮 20+ 工具 | ToolStrip 折叠/展开正常 |
| M55-JOB-01 | 飞书 `/async` 或长任务关键词 | Web「后台任务」面板 ≤3s 出现 Job |

```bash
# 后端
go test ./internal/service/ -run 'EnsureChannelSession|ListSessionMessages_afterRevision|ListChatBackgroundJobs' -count=1
# 前端
cd web && pnpm test -- messageSourceMeta groupMessagesByTurn
```

---

## 回归命令

```bash
go test ./internal/biz/... ./internal/channel/preview/... ./internal/channel/lark/... -count=1
go test ./internal/service/ -run "TurnPreview|Interactive" -count=1
```

---

## 已知限制

- 飞书 Card 为静态 JSON；进行中状态为 emoji 文案，非动画。
- Card HTTP 在独立 goroutine 发送，不阻塞 EventBus 消费。
- 无 tenant 时 LT-04/05 仅能通过 httptest 契约测（`interactive_card_test.go`）验证。

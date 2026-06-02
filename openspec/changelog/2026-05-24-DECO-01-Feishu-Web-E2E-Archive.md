# DECO-01 手工 E2E 归档 — 飞书 + Web 同 Session Revision 同步

> **日期**：2026-05-24  
> **环境**：`admin` @ `localhost:8000` · Web `quasar dev` @ `localhost:9001`  
> **Agent**：正阳 `4605c52da3e3a8e67bfa7ef7`（`zhengyang-agent`）  
> **Session**：`fab138cb-de4b-46ce-9cac-7ba547991317`（飞书 peer）  
> **对照 Agent（场景 D）**：启明 `a48cbd84e7695000a1ee1814`

---

## 总览

| 场景 | ID | 结果 | 备注 |
|------|-----|------|------|
| Turn 进行中再打开 Web | M55-SYNC-01 | ✅ PASS | ≤5s 见 user + running；飞书来源徽标 |
| Turn 完成增量 sync | M55-SYNC-02 | ✅ PASS | assistant 自动出现；无需刷新 |
| sync 不误 complete | DECO-14 C4 | ✅ PASS | sync 阶段仅 streaming；终态在 completed 后 |
| Offline → Online 补齐 | DECO-14 C3 | ✅ PASS | 恢复 Online 后 rev 递增、消息补齐；无 banner（hydrate 成功） |
| 跨 Agent auto-focus | CC-B-06b 场景 D | ❌ FAIL | 启明上飞书入站不跳转；铃铛 ✅；手动切正阳后消息完整 |

**DECO-01 主链路（M55-SYNC-01/02 + DECO-14）：✅ 通过**  
**CC-B-06b 跨 Agent focus：❌ 待修复（见下文 P1）**

---

## 场景 A — M55-SYNC-01 ✅

**操作**：Web 未打开飞书 session → 飞书发「你好，请用一句话介绍自己」→ Web 打开同 session。

| 观测 | 结果 |
|------|------|
| User 气泡 | ≤5s 出现 + 飞书来源徽标 |
| Running | 顶栏 WS ON；rev 0→2 |
| Assistant | completed 后自动 hydrate |

---

## 场景 B — M55-SYNC-02 ✅

**操作**：Web 已在正阳飞书 session → 飞书发「你能为我做哪些事情」。

| 观测 | 结果 |
|------|------|
| Assistant | 自动出现，无需刷新 |
| rev | 2→4 |
| 🟡 User 气泡 | 增量阶段偶发不可见；full hydrate / 手动切换后补齐 |

---

## 场景 C — DECO-14 ✅

### C4 — sync envelope 不误关 Turn ✅

sync 阶段仅 streaming；assistant 终态在 `completed` / `runner.completion` 后才出现。

### C3 — Offline → Online ✅

| 阶段 | 顶栏 | 内容 |
|------|------|------|
| Offline | REV 8 | 无新消息 |
| Online 恢复 | REV 8→10 | user「你好」+ assistant 自动补齐 |

黄色 `inboundHydrateError` banner 未出现（hydrate 成功属正常；失败路径由单测 `decoP2Sync.spec.ts` 覆盖）。

---

## 场景 D — CC-B-06b 跨 Agent auto-focus ❌

**前置**：Web 停在 **启明**（Composer 空）；飞书向 **正阳** 飞书 session 发消息。

**探针**：「介绍你的团队」

### 后端 ✅

```
[user]介绍你的团队 → runner.completion
Assistant：正阳团队介绍（iOS/Android/桌面等）
```

### Web（停留在启明）

| 步骤 | 预期 | 实际 | 判定 |
|------|------|------|------|
| D1 auto-focus | RUNNING 时跳正阳飞书 session | 仍显示启明「你好」REV 2 | ❌ |
| D2 铃铛 | 渠道入站通知 | 顶栏铃铛红点 1 | ✅ |
| D3 完成 toast | 未在看该 session 时弹「查看」 | URL 含 `fab138cb` 但 UI 为启明，toast 可能未弹 | ⚠️ |
| D4 手动切正阳 | 消息完整 | 「介绍你的团队」+ 飞书徽标；REV 14 · 20 条 | ✅ |

### 根因（已定位，2026-05-24 修复中）

1. **`useChatInboundSync`**：`focusChannelSession` 要求 `entityMatch`（当前 Agent 必须已是正阳），跨 Agent 时 auto-focus 与 sync 均被 `if (!isCurrent && !entityMatch) return` 跳过。
2. **`useGlobalInboundNotifications.isViewingSession`**：仅看 URL `query.session`，与 UI 选中 Agent/session 不同步时误判「已在看」。
3. **`messageStore.loadMessages({ afterRevision })`**：增量 API 只返回新行，但 `mergeSessionMessages(items, local)` 会丢弃已有 persisted 历史，导致正阳 session 上 user 气泡偶发缺失（场景 B/D4）。

---

## 自动化回归（归档日已通过）

```bash
go test ./internal/service/ -run TestDECO01_SessionRevisionChannelToWebSync -count=1
cd web && pnpm test -- inboundSyncEnvelope decoP2Sync mergeSessionMessages --run
```

---

## 后续修复（同 PR / 紧接提交）

| ID | 内容 | 落点 |
|----|------|------|
| FIX-D-01 | Channel RUNNING 跨 Agent auto-focus（去掉 `entityMatch` 前置） | `useChatInboundSync.ts` |
| FIX-D-02 | `isViewingSession` 对齐 `chatStore.currentSessionId` + selected Agent | `useGlobalInboundNotifications.ts` |
| FIX-D-03 | 增量 hydrate 保留已有 persisted 消息 | `mergeSessionMessages.ts` · `messageStore.ts` |
| FIX-D-04 | auto-focus 识别 channel session（`run_status` 无 source 时 API 解析）· sync 信封触发 · `focusAgentSessionView` 同步 URL | `channelInboundSession.ts` · `useChatInboundSync.ts` |
| FIX-D-05 | 完成 toast 在 `run_status completed + source=channel` 时弹出（4s dedupe） | `useGlobalInboundNotifications.ts` |

---

## Holistic Fix（2026-05-24）— 助手回复消失

> **Review**：[2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md](../review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md) · **Changelog**：[Holistic Fix](../changelog/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix.md)

FIX-D 合入后出现「回复消息完全没有了」。整体修复（非单点 patch）：

| ID | 内容 | 落点 |
|----|------|------|
| FIX-H-01 | 删除 `channelLiveTurn`，恢复 session WS | `useChatStreamManager.ts` |
| FIX-H-02 | `channelWsCursor` + `lastEventId` 防 replay | `channelWsCursor.ts` · `useEnvelopeStream.ts` |
| FIX-H-03 | turn complete **一律**全量 hydrate | `useChatInboundSync.finalizeTurn` |
| FIX-H-04 | 空 `afterRevision` + dropStale → fallback 全量 | `messageStore.ts` |
| FIX-H-05 | focus 不再 `clearSessionMessages` | `useChatEntityNav.ts` |

**开放项（Review P1）**：DECO-R-P1-01 双 patch · DECO-R-P1-02 双 hydrate — 见 Review §3。

**待复验**：场景 A/B/D + Web 当前 session 自聊（Review T3）。

---

## 相关文档

- [17-channel-development.md §14 DECO](../需求/17-channel-development.md#14-phase-deco--四层架构解耦deco)
- [M55 Stuck-Turn-Inbound-Sync-Analysis](./2026-05-23-M55-Stuck-Turn-Inbound-Sync-Analysis.md)
- [M55 Phase R-UX Review](../review/2026-05-23-M55-Phase-R-UX-Review.md) — RUX-P1-02 auto-focus 守卫
- [DECO-01 Holistic Fix Review](../review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md)

# Session Status Frontend Spec

## MODIFIED Requirements

### Requirement: Session Type Definitions

The `Session` interface in `features/session/types.ts` SHALL be updated to include session status fields. Two new types SHALL be defined:

```typescript
export type SessionStatus =
  | 'idle'
  | 'running'
  | 'completed'
  | 'interrupted'
  | 'awaiting_confirmation'

export type SessionStatusReason =
  | 'user_cancelled'
  | 'timeout'
  | 'budget_escalated'
  | 'error'
  | 'context_overflow'
  | 'server_shutdown'
  | 'unexpected_shutdown'
  | 'confirmation_timeout'
  | 'tool_confirmation'
  | 'agent_awaiting_reply'
  | 'manual_override'
```

The `Session` interface SHALL add:
- `status: SessionStatus`
- `statusReason: SessionStatusReason`
- `statusChangedAt: string`

Legacy status values (`active`, `archived`, `deleted`) SHALL be removed from the frontend type definitions.

#### Scenario: Session interface includes status fields

WHEN a Session object is received from the API
THEN it SHALL contain `status`, `statusReason`, and `statusChangedAt` fields matching the defined types

#### Scenario: SessionStatus type enforces enum values

WHEN a variable is typed as `SessionStatus`
THEN it SHALL only accept values from the defined union type

#### Scenario: SessionStatusReason type enforces enum values

WHEN a variable is typed as `SessionStatusReason`
THEN it SHALL only accept values from the defined union type

---

### Requirement: Session Store Status Update

The session store SHALL provide a `patchSessionStatus(sessionId, status, statusReason, statusChangedAt)` method that updates the status fields of a specific session in the store without requiring a full list refresh.

#### Scenario: Patch session status from WS push

WHEN `patchSessionStatus("session-1", "interrupted", "timeout", "2026-05-31T12:00:00Z")` is called
THEN the session with ID "session-1" in the store SHALL have its `status`, `statusReason`, and `statusChangedAt` updated

#### Scenario: Patch non-existent session ignored

WHEN `patchSessionStatus` is called with a session ID that does not exist in the store
THEN the method SHALL NOT throw an error and SHALL be a no-op

---

## ADDED Requirements

### Requirement: SessionStatusBadge Component

A `SessionStatusBadge.vue` component SHALL be created at `components/session/SessionStatusBadge.vue`. It SHALL display a visual status indicator based on the session's `status` field with the following mapping:

| Status | Icon | Color | Text |
|--------|------|-------|------|
| `idle` | ○ (circle outline) | Grey | 空闲 |
| `running` | ⟳ (spinning) | Accent color (`--color-accent`) | 执行中 |
| `completed` | ✓ (check) | Green | 已完成 |
| `interrupted` | ✕ (cross) | Orange/Warning | 已中断 |
| `awaiting_confirmation` | ⏸ (pause) | Blue/Info | 等待确认 |

The badge SHALL accept the following props:
- `status: SessionStatus` — required
- `statusReason?: SessionStatusReason` — optional, for tooltip detail
- `statusChangedAt?: string` — optional, for relative time display in tooltip

On hover, the badge SHALL display a tooltip with full information in the format: `{状态文案} · {中断原因文案} · {相对时间}`. If `statusReason` is empty, the reason portion SHALL be omitted.

#### Scenario: Badge displays idle state

WHEN `SessionStatusBadge` is rendered with `status = 'idle'`
THEN it SHALL display a grey circle icon with text "空闲"

#### Scenario: Badge displays running state with animation

WHEN `SessionStatusBadge` is rendered with `status = 'running'`
THEN it SHALL display a spinning icon in accent color with text "执行中"

#### Scenario: Badge displays completed state

WHEN `SessionStatusBadge` is rendered with `status = 'completed'`
THEN it SHALL display a green check icon with text "已完成"

#### Scenario: Badge displays interrupted state

WHEN `SessionStatusBadge` is rendered with `status = 'interrupted'`
THEN it SHALL display an orange cross icon with text "已中断"

#### Scenario: Badge displays awaiting confirmation state

WHEN `SessionStatusBadge` is rendered with `status = 'awaiting_confirmation'`
THEN it SHALL display a blue pause icon with text "等待确认"

#### Scenario: Badge tooltip shows full detail

WHEN `SessionStatusBadge` is rendered with `status = 'interrupted'`, `statusReason = 'timeout'`, `statusChangedAt = '2026-05-31T12:00:00Z'`
THEN the tooltip SHALL display "已中断 · 执行超时 · {relative time}"

#### Scenario: Badge tooltip omits reason when empty

WHEN `SessionStatusBadge` is rendered with `status = 'running'` and no `statusReason`
THEN the tooltip SHALL display "执行中" without a reason portion

---

### Requirement: Interruption Reason Text Mapping

A mapping function SHALL be provided that converts `SessionStatusReason` values to human-readable Chinese text:

| status_reason | Display Text |
|---------------|-------------|
| `user_cancelled` | 用户取消 |
| `timeout` | 执行超时 |
| `budget_escalated` | 预算超限，已升级后台执行 |
| `error` | 执行出错 |
| `context_overflow` | 上下文溢出 |
| `server_shutdown` | 服务关闭 |
| `unexpected_shutdown` | 服务异常退出 |
| `confirmation_timeout` | 确认超时 |
| `tool_confirmation` | 工具需确认 |
| `agent_awaiting_reply` | Agent 等待回复 |
| `manual_override` | 手动覆盖 |

For unknown reason values, the function SHALL return the raw reason string as fallback.

#### Scenario: Known reason mapped to Chinese text

WHEN the mapping function receives `timeout`
THEN it SHALL return "执行超时"

#### Scenario: Unknown reason falls back to raw value

WHEN the mapping function receives an unknown reason value `some_new_reason`
THEN it SHALL return `"some_new_reason"`

#### Scenario: Empty reason returns empty string

WHEN the mapping function receives an empty string
THEN it SHALL return an empty string

---

### Requirement: Delete Protection UI — Single Delete

On the SessionsPage, when a session has a protected status (`running` or `awaiting_confirmation`), the delete button SHALL be disabled. A tooltip SHALL be displayed on the disabled button indicating the reason:

- `running`: "会话正在执行中，无法删除"
- `awaiting_confirmation`: "会话正在等待确认，无法删除"

#### Scenario: Delete button disabled for running session

WHEN a session has `status = 'running'`
THEN the delete button SHALL be disabled and the tooltip SHALL display "会话正在执行中，无法删除"

#### Scenario: Delete button disabled for awaiting_confirmation session

WHEN a session has `status = 'awaiting_confirmation'`
THEN the delete button SHALL be disabled and the tooltip SHALL display "会话正在等待确认，无法删除"

#### Scenario: Delete button enabled for idle session

WHEN a session has `status = 'idle'`
THEN the delete button SHALL be enabled

#### Scenario: Delete button enabled for completed session

WHEN a session has `status = 'completed'`
THEN the delete button SHALL be enabled

#### Scenario: Delete button enabled for interrupted session

WHEN a session has `status = 'interrupted'`
THEN the delete button SHALL be enabled

---

### Requirement: Delete Protection UI — Batch Delete

On the SessionsPage, when the user selects multiple sessions for batch deletion and some have protected statuses, the batch action bar SHALL display a message: "N 个会话正在执行中，无法删除" where N is the count of protected sessions. The batch delete operation SHALL only delete non-protected sessions.

#### Scenario: Batch delete with mixed statuses

WHEN the user selects 5 sessions, 2 of which have `status = 'running'`
THEN the batch action bar SHALL display "2 个会话正在执行中，无法删除" and only the 3 non-protected sessions SHALL be deleted

#### Scenario: Batch delete with all protected

WHEN the user selects sessions that all have protected statuses
THEN the batch delete button SHALL be disabled and the message SHALL indicate all sessions are protected

#### Scenario: Batch delete with no protected sessions

WHEN the user selects sessions that all have non-protected statuses
THEN the batch delete SHALL proceed normally without any protection message

---

### Requirement: Delete Protection UI — Context Menu

On the ChatPage sidebar session list, the right-click context menu SHALL disable the "Delete" option for sessions with protected statuses. The disabled option SHALL display a tooltip explaining why deletion is not available.

#### Scenario: Context menu delete disabled for running session

WHEN the user right-clicks a session with `status = 'running'` in the ChatPage sidebar
THEN the "Delete" option SHALL be disabled with tooltip "会话正在执行中，无法删除"

#### Scenario: Context menu delete enabled for idle session

WHEN the user right-clicks a session with `status = 'idle'` in the ChatPage sidebar
THEN the "Delete" option SHALL be enabled

---

### Requirement: WS Push Handling for Session Status

The `sessionSync.ts` module SHALL handle the `session.status_changed` WebSocket envelope type. Upon receiving this event, it SHALL call `sessionStore.patchSessionStatus(session_id, status, status_reason, status_changed_at)` to update the local store without requiring a full session list refresh.

```typescript
onSessionMutation('session.status_changed', (payload) => {
  const { session_id, status, status_reason, status_changed_at } = payload
  sessionStore.patchSessionStatus(session_id, status, status_reason, status_changed_at)
})
```

#### Scenario: WS push updates session status in store

WHEN a `session.status_changed` event is received with `session_id = "s1"`, `status = "interrupted"`, `status_reason = "timeout"`, `status_changed_at = "2026-05-31T12:00:00Z"`
THEN the session store SHALL update session "s1" with the new status, reason, and timestamp

#### Scenario: WS push triggers reactive UI update

WHEN a `session.status_changed` event updates a session that is currently displayed in the UI
THEN the SessionStatusBadge and any related UI elements SHALL reactively update to reflect the new status

#### Scenario: WS push for unknown session ignored

WHEN a `session.status_changed` event is received for a session ID not in the store
THEN the handler SHALL NOT throw an error

---

### Requirement: Interrupted Session Recovery Guidance

Sessions with `status = 'interrupted'` SHALL display a "继续对话" (Continue Conversation) button in the session list item. Clicking this button SHALL navigate to the chat page and focus the message input, allowing the user to send a new message which will transition the session back to `running`.

#### Scenario: Interrupted session shows continue button

WHEN a session has `status = 'interrupted'`
THEN the session list item SHALL display a "继续对话" button

#### Scenario: Click continue navigates to chat

WHEN the user clicks the "继续对话" button on an interrupted session
THEN the app SHALL navigate to the chat page for that session and focus the message input

#### Scenario: Sending message transitions to running

WHEN the user sends a message in an interrupted session's chat page
THEN the session SHALL transition to `running` status

---

### Requirement: Awaiting Confirmation Recovery Guidance

Sessions with `status = 'awaiting_confirmation'` SHALL display contextual recovery guidance based on the `statusReason`:

- `tool_confirmation`: The chat page SHALL display a tool confirmation dialog with "确认" (Confirm) and "拒绝" (Reject) buttons. Confirming SHALL transition the session back to `running`. Rejecting SHALL transition the session to `interrupted` with reason `user_cancelled`.

- `agent_awaiting_reply`: The chat page SHALL display a prompt in the input area: "Agent 正在等待你的回复" (Agent is waiting for your reply). The user can type and send a reply, which transitions the session back to `running`.

#### Scenario: Tool confirmation dialog displayed

WHEN a session has `status = 'awaiting_confirmation'` and `statusReason = 'tool_confirmation'`
THEN the chat page SHALL display a tool confirmation dialog with confirm and reject buttons

#### Scenario: Tool confirmation accepted

WHEN the user clicks "确认" in the tool confirmation dialog
THEN the session SHALL transition to `running` status

#### Scenario: Tool confirmation rejected

WHEN the user clicks "拒绝" in the tool confirmation dialog
THEN the session SHALL transition to `interrupted` with reason `user_cancelled`

#### Scenario: Agent awaiting reply prompt displayed

WHEN a session has `status = 'awaiting_confirmation'` and `statusReason = 'agent_awaiting_reply'`
THEN the chat page input area SHALL display "Agent 正在等待你的回复"

#### Scenario: User reply transitions to running

WHEN the user sends a reply in a session with `statusReason = 'agent_awaiting_reply'`
THEN the session SHALL transition to `running` status

---

### Requirement: ChatPage Sidebar Status Badge

The ChatPage sidebar session list SHALL display a `SessionStatusBadge` next to each session name for sessions with `status = 'running'` or `status = 'awaiting_confirmation'`. Sessions with `idle` or `completed` status SHALL NOT display a badge (to avoid visual clutter). Sessions with `interrupted` status SHALL display the badge to draw attention to the interrupted state.

#### Scenario: Running session shows badge in sidebar

WHEN a session with `status = 'running'` is displayed in the ChatPage sidebar
THEN a `SessionStatusBadge` SHALL be shown next to the session name

#### Scenario: Awaiting confirmation session shows badge in sidebar

WHEN a session with `status = 'awaiting_confirmation'` is displayed in the ChatPage sidebar
THEN a `SessionStatusBadge` SHALL be shown next to the session name

#### Scenario: Interrupted session shows badge in sidebar

WHEN a session with `status = 'interrupted'` is displayed in the ChatPage sidebar
THEN a `SessionStatusBadge` SHALL be shown next to the session name

#### Scenario: Idle session hides badge in sidebar

WHEN a session with `status = 'idle'` is displayed in the ChatPage sidebar
THEN no `SessionStatusBadge` SHALL be shown

#### Scenario: Completed session hides badge in sidebar

WHEN a session with `status = 'completed'` is displayed in the ChatPage sidebar
THEN no `SessionStatusBadge` SHALL be shown

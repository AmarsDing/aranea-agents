# B-01 / B-06 / Upsert CAS 整改落地审查

> 日期：2026-07-16  
> 对照：`2026-07-15-audit-aranea-issue-register.md`、`2026-07-15-audit-aranea-remediation-roadmap.md`、`2026-07-15-review-remediation-roadmap-verification.md`  
> 范围：用户指定的三项（B-06 重连快照、B-01 JWT workspace 绑定、Task/Turn/Step Upsert Version CAS）及文档漂移修复

## 1. 结论

| 项 | 审计原问题 | 2026-07-16 落地状态 | 残余 |
|---|---|---|---|
| **1 · B-06** | Critical 事件可丢且无 WS replay | **部分关闭**：终态 WBPF + 高优先级 WS lane；前端 disconnect→reconnect 调用 `fetchSessionHistory`；进程内 Seq 可从 DB `MAX(seq)` 恢复 | 服务端 cursor replay 仍不做（产品决策：REST snapshot）；多表 Seq 聚合仅以 Step 为准；hydrate 期间 live 事件缓冲未做 |
| **2 · B-01** | Header/Query 可伪造 workspace | **部分关闭（P2-A）**：JWT `workspace_id`；登录写入 `admins.workspace_id`；中间件对**所有**主体拒绝 Header forge | 无 `workspace_memberships` 多对多表；无 Postgres RLS；admin 切换租户需换 JWT/账号 |
| **3 · Upsert CAS** | 状态写无版本守卫 | **已确认生产就绪**：Task/Turn/Step Upsert 使用 `VersionLT`；补 Step 回归测试 | PlanBoard / ChannelTurnJob / GraphStage 等非本项范围实体仍可能缺 CAS |

## 2. 代码锚点（审查时核对）

### 2.1 B-06 session sequence + reconnect snapshot

| 能力 | 位置 |
|---|---|
| 终态 BlockUpTo | `internal/event/bus_v2.go`、`biz.IsTerminalEventKind` |
| WS terminal 高优先级 | `internal/server/ws_priority.go`、`ws_v2_subscriber.go` |
| SeqAssigner `RestoreAtLeast` | `internal/agent/v2/seqassigner.go`、`projector.go` `SeqAssigner` 接口 |
| 进程重启恢复 | `StepV2Reader.MaxSeqBySpiritSession` → `ProjectorFactory.RestoreSeqIfNeeded` → turn 启动（`chat_orchestrator_turn_phases.go`） |
| 前端重连 hydrate | `useChatStreamManager.ts` `onReconnectHydrate`；`useChatWorkspace.ts` → `fetchSessionHistory` |
| 契约说明 | `last_event_id` 仍为 echo-only（`internal/server/ws.go` 注释）；客户端以 REST 为准 |

### 2.2 B-01 JWT workspace（1:1 admin→workspace）

| 能力 | 位置 |
|---|---|
| JWT claim | `pkg/auth/auth.go` `WorkspaceID` / `EffectiveWorkspaceID` |
| Cookie 写入 | `auth.SetCookieForWorkspace`；`AdminService.Login` |
| Schema | `admins.workspace_id`（migration `20261006`）；Ent `schema/admin.go` |
| 中间件 | `internal/server/middleware/workspace.go`：认证主体一律 JWT；forge → 403 |
| Proto | `api/kratos/admin/v1/admin.proto` field `workspace_id = 10` |

**明确降级决策**：不做 membership 表；admin 不能靠 `X-Workspace-ID` 切租户。

### 2.3 Upsert Version CAS

| 实体 | 守卫 | 测试 |
|---|---|---|
| Task | `taskv2.VersionLT` | `TestTaskV2Repo_Upsert_VersionGuard` |
| Turn | `turnv2.VersionLT` | `TestTurnV2Repo_Upsert_VersionGuard` |
| Step | `stepv2.VersionLT` | `TestStepV2Repo_Upsert_VersionGuard`（本轮补齐） |

## 3. 文档漂移修复

| 文档/注释 | 原表述 | 修正 |
|---|---|---|
| `2026-07-15-review-…-verification.md` §2.1 | `Auth` 无 `WorkspaceID` | 见本文 §4 附录：历史取证快照仍保留，以本报告为当前真相 |
| issue-register B-01/B-06 | 仅描述问题 | 增加「整改进度」节（同日更新） |
| FE「WS replay」注释 | 暗示服务端仍 replay | 改为 reconnect hydrate |

## 4. 附录：相对 2026-07-15 verification 的过时断言

以下断言在 **2026-07-15 取证时正确**，在 **2026-07-16 代码中已失效**，请勿再当作未修复证据：

1. `pkg/auth/auth.go`「无 WorkspaceID」——现已有 claim + EffectiveWorkspaceID。
2. Workspace 中间件「信任 Header」——现对已认证主体拒绝 forge。
3. 「无 reconnect hydration」——前端 `onReconnectHydrate` + `fetchSessionHistory` 已接线。
4. Task/Turn/Step「无 VersionLT」——生产 Upsert 已具备；仅部分其它实体仍缺。

## 5. 建议后续（非本轮）

1. 多表 `MAX(seq)` 聚合（Turn/Task/Team*）或统一 sequence 表。  
2. hydrate 期间 live WS 事件缓冲，避免 snapshot 覆盖竞态。  
3. `workspace_memberships` + RLS（完整 Wave 1B）。  
4. PlanBoard / ChannelTurnJob expected-state CAS。

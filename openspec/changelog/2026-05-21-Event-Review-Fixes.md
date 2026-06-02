# Event 模块 Review 修复（2026-05-21）

## 摘要

按 Review 清单完成 P1–P3 六项优化：会话授权、Inspector WS 复用、持久化有界队列、FilterKey UI、kerrors 统一、createEventService + 集成测试。

## 后端

| 项 | 变更 |
|----|------|
| ListEvents 授权 | `EventService` 注入 `SessionUsecase`，列表前 `Get(session_id)`，不存在返回 404 |
| 持久化队列 | `eventPersistHandler` 有界 channel + 单 worker；`EVENT_STORE_PERSIST_QUEUE` 可配置（默认 512） |
| 排除高流量类型 | `text_delta` / `member_delta` 不再入库（仍走内存 Buffer + WS） |
| kerrors | `event_store_repo` Insert/List/Delete 统一 `kerrors.InternalServer` |
| Marshal | `envelopeToStoreRecord` 序列化失败打 SysLogWarn 并 skip |
| Wire | `provideEventService(store, sessions)` |

## 前端

| 项 | 变更 |
|----|------|
| WS 复用 | `useChatStreamManager.subscribeSessionStream`；Inspector 订阅已有 chat/team stream |
| FilterKey UI | `EventFilterBar` 增加 FilterKey 输入 |
| createEventService | `services/index.ts` + `features/event/api.ts` 走 proto 客户端 |

## 测试

- `internal/service/event_test.go` — ListEvents 404 / 回放
- `internal/biz/event_persist_handler_test.go` — 排除规则 + 序列化
- `web/.../eventFilter.spec.ts` — filterKey 前缀

## 文档

- `34-event-development.md` 优化表新增 O8–O12

# 2026-05-16 Session: 功能模块优化

**影响**：🟡 中 | **模块**：Session

## 变更摘要

Session 模块功能优化：通用更新接口、会话恢复、Timeline 分页过滤、搜索排序、消息分页、state_json 暴露。

## 关键变更

- `UpdateSession` RPC 从仅支持 title 更新扩展为通用部分更新（title/tags_json/visibility/metadata_json/dialog_mode/default_provider/default_model）
- 新增 `RestoreSession` RPC（POST /v1/sessions/{id}/restore），支持从归档/删除状态恢复会话
- `GetSessionTimeline` 新增分页（limit/offset）、类型过滤（kind_filter）、排序（sort_order）参数
- `ListSessionMessages` 新增分页（limit/offset）参数和 total 返回
- `SearchSessions` 新增 user_id 过滤和 sort_by/sort_order 排序参数
- `CreateSession` 新增 tags_json/metadata_json 入参
- `Session` message 新增 state_json 字段，暴露 trpc session.Service 状态数据
- Biz 层新增 `SessionUpdateFields`/`TimelineQuery` 结构体，`Update`/`Restore` 方法
- Data 层新增 `UpdateSession`/`RestoreSession` 实现，`sessionSearchOrder` 排序辅助函数

## 破坏性变更

1. `UpdateSessionRequest.title` 从 REQUIRED 变为 OPTIONAL（空字符串表示不更新）
2. `ListSessionMessagesResponse` 新增 `total` 字段

> 详细实现记录见 [devlog/2026-05-16-Session-Optimize.md](../devlog/2026-05-16-Session-Optimize.md)

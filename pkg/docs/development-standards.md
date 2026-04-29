# 开发规范

## 通用
- 所有 API 以 `/api/v1` 为前缀。
- 字段命名统一使用 snake_case（后端）与 camelCase（前端 DTO）。
- 所有写操作必须记录审计日志。

## 后端
- 包结构遵循：`internal/{domain,repository,service,transport,middleware}`。
- 数据库变更只允许通过 migration 文件。
- 新增业务功能必须包含单元测试。

## 前端
- 页面路由集中在 `src/router/routes.ts`。
- 接口调用统一走 `src/api/client.ts`。
- 组件优先复用 Quasar 组件，避免重复造轮子。

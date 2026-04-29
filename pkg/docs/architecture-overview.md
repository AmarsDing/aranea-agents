# 架构总览

## 后端分层
- `transport`：HTTP 路由与请求解析。
- `service`：业务逻辑（Agent/Session/Chat/Audit）。
- `runtime`：ADK-Go 适配边界。
- `repository`：SQLite 持久层（可替换）。
- `middleware`：认证、限流、请求ID、访问日志。

## 前端分层
- `pages`：页面级容器。
- `stores`：Pinia 状态层。
- `api`：统一后端契约。
- `router`：路由定义。

## 升级位
- 数据层通过 `repository.Store` 接口解耦，支持 PostgreSQL 替换。
- runtime 通过 `ADKRuntimeAdapter` 隔离，支持接入真实 adk-go runner。

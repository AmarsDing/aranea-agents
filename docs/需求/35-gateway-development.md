# Gateway 网关 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 基础网关可用
> **需求**：[35 gateway.md](./35%20gateway.md) · **设计**：[35 gateway.design.md](./35%20gateway.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Gateway 网关：统一的 API 网关，负责路由、认证、限流、日志等横切关注点。

**代码锚点**：
- `internal/server/http.go` — HTTP 路由注册 + 中间件
- `internal/server/grpc.go` — gRPC 服务注册
- `internal/server/ws.go` — WebSocket 服务
- `internal/middleware/` — 认证/限流中间件

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| HTTP 路由 | ✅ | Kratos HTTP Server |
| gRPC 服务 | ✅ | Kratos gRPC Server |
| WebSocket | ✅ | `ws.go` |
| 认证中间件 | ✅ | JWT 认证 |
| 限流中间件 | ✅ | 速率限制 |
| API 版本管理 | ❌ | 无版本前缀 |
| API 文档 | ❌ | 无 Swagger/OpenAPI 自动生成 |

---

## 3. 差距与优化

1. **P2**：API 无版本管理（如 `/api/v1/`），未来 API 变更可能导致兼容性问题。
2. **P3**：无 Swagger/OpenAPI 文档自动生成，前端开发者需手动查阅 proto。

---

## 4. 开发阶段

- **Phase 1**：API 版本管理（`/api/v1/` 前缀）
- **Phase 2**：Swagger/OpenAPI 文档自动生成

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | HTTP 路由增加 `/api/v1/` 版本前缀 | P2 | — |
| 2 | protoc-gen-openapi 生成 Swagger 文档 | P3 | — |

---

## 6. 验收标准

- [ ] 所有 API 路径包含版本前缀
- [ ] Swagger UI 可访问

---

## 7. 依赖与风险

- API 版本管理需与前端路由同步更新

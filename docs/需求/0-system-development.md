# System 系统 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：系统基础架构 · **设计**：系统架构设计
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

System 系统：项目基础架构，包括 Kratos 框架集成、Wire 依赖注入、Ent ORM、配置管理、启动流程等。

**代码锚点**：
- `cmd/aranea/main.go` — 入口
- `cmd/aranea/wire.go` — Wire 依赖注入
- `cmd/aranea/wire_gen.go` — Wire 生成
- `internal/conf/` — 配置
- `internal/data/` — Data 层
- `internal/server/` — Server 层

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Kratos 框架集成 | ✅ | HTTP/gRPC/WS Server |
| Wire 依赖注入 | ✅ | `wire.go` + `wire_gen.go` |
| Ent ORM | ✅ | SQLite + 单连接池 |
| 配置管理 | ✅ | `configs/` YAML |
| 启动流程 | ✅ | `main.go` → Wire → Server |
| 健康检查 | ✅ | Kratos 健康检查端点 |

---

## 3. 差距与优化

1. **P2**：Ent 使用 SQLite 单连接池，高并发下性能受限。需评估 PostgreSQL 迁移。
2. **P2**：Wire 生成文件 `wire_gen.go` 可能与手动修改不一致，需 CI 校验。
3. **P3**：配置无热更新，修改配置需重启服务。

---

## 4. 开发阶段

- **Phase 1**：PostgreSQL 迁移评估与实施
- **Phase 2**：Wire 生成 CI 校验
- **Phase 3**：配置热更新

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | PostgreSQL 迁移方案设计 | P2 | — |
| 2 | Wire 生成 CI 校验（`wire diff`） | P2 | — |
| 3 | 配置热更新（fsnotify） | P3 | — |

---

## 6. 验收标准

- [ ] PostgreSQL 可作为数据存储
- [ ] CI 中 `wire check` 通过
- [ ] 配置修改后无需重启

---

## 7. 依赖与风险

- PostgreSQL 迁移需数据迁移方案
- 配置热更新需注意并发安全

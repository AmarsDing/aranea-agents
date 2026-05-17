# 数据平台横切 — 开发计划

> **版本**：2026-05-17  
> **说明**：非独立产品模块，覆盖 `internal/data/data.go` 启动、Schema 与 Repo 工厂。  
> **EP**：EP-DATA-01、EP-RT-08、EP-WS-01

---

## 1. 问题陈述

`EnsureEvalSchema` / `EnsureA2ASchema` / `EnsureKnowledgeSchema` 已实现但 **未在 `NewData()` 调用**，导致 Evaluation、A2A、Knowledge 在生产 SQLite/Postgres 组合下首跑可能缺表。Knowledge Repo 在无 Postgres 时为 nil，需全链路明确降级。

---

## 2. 建议改动（单 PR 可闭合 EP-DATA-01）

```text
NewData() 成功打开 rawDB 后：
  - EnsureEvalSchema(ctx, rawDB)
  - EnsureA2ASchema(ctx, rawDB)
若 pg != nil：
  - EnsureKnowledgeSchema(ctx, pg, vdim)
```

配套：各 Service 在 repo nil 时返回 `kerrors` 业务错误，不 panic。

---

## 3. EP-RT-08（后续 PR）

- `mem*Repo` 仅保留 `_test.go` / `s6_coverage_test.go`。
- 生产 wire 禁止注入内存 Repo；缺后端时 fail-fast 或功能开关关闭。

---

## 4. M2 多租户（EP-WS-01）

Ent Hook 按域分批：admin → agent → session → memory → tool；与 `AssertWorkspace` 写路径配对。

---

## 5. 验收

- [ ] 冷启动后 eval/a2a 表存在（sqlite）
- [ ] 配置 postgres 后 knowledge 表存在
- [ ] 无 PG 时 Knowledge API 稳定错误
- [ ] execution-plan §5 EP-DATA-01 勾选

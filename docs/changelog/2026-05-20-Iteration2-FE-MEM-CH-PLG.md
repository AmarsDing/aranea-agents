# 2026-05-20 — 迭代 2：前端治理 / Memory L4 / 企微 Channel / Plugin Scope

## 摘要

完成迭代 2 任务板剩余 P1/P2 项（除 Chat 多模态 P3）：A2A 页面组件化与 mapper 单测、L4 图写入治理、企业微信 Channel 入站出站、`UpdatePluginScope` API。

## 变更

### 前端（I2-FE-01）

- `A2APage.vue` 拆分为 `A2ADiscoverPanel` / `A2AAuditPanel` / `A2AInvokePanel`（页面 <120 行）
- 新增 `features/a2a/mappers.ts` 与 Vitest 单测
- Knowledge / Evaluation 页面此前已符合 `page-to-components`（<300 行）

### Memory（I2-MEM-01）

- `L4GraphWriter`：人名实体冲突检测、profile 级联刷新、30 天置信度衰减
- `sessionmemory`：`GetEntityByScopeKey`、`GetFirstEntityByType`、`ApplyConfidenceDecay`

### Channel（I2-CH-03）

- `internal/channel/wecom`：入站解析、签名校验、出站 `TextSender`
- `channel_ingress` 支持 `wecom` / `wecom-app` 类型分发

### Plugin（I2-PLG-01 部分）

- Proto + Service：`PATCH /v1/plugins/{id}/scope`（`UpdatePluginScope`）
- 运行记录表（PluginRun）仍待后续 PR

## 验证

- `make api && make build`
- `go test ./internal/memory/... ./internal/channel/wecom/... ./internal/service/...`
- `cd web && pnpm test -- --run src/features/a2a/__tests__/mappers.spec.ts`

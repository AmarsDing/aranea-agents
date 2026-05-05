# 前端收口波次（Phase 8，`cmd/admin` 对齐）

后端契约以 **`api/kratos/**` + `/v1/...` 网关** 为准的前提下，前端按 **[AI-full-stack-migration-playbook.md](AI-full-stack-migration-playbook.md)** Playbook **B/C** 分波收敛：

## 已实现的本轮机械对齐

| 域 | 说明 |
|----|------|
| **memory** | `createMemoryService` 已承载 **`UpsertMemoryFact`**、**`AppendEvolutionEvent`**；`features/memory/api` 透出 **`upsertMemoryFact`** / **`appendEvolutionEvent`**；`memoryEndpoints` 记录 POST 路由。 |

## 建议的后续 UX / 分层（独立 PR）

- 将各 feature 的数据访问收口到 **`features/<域>/api.ts`**（避免组件内散装 `fetch`）。
- **`web/src/services/index.ts`** 仅保留 **`create*Service`** 工厂，与 proto 生成客户端一一对应。
- 设计语言 token（奶油昼 / 玻璃夜等）与大屏编排按 Playbook 与现有 design tokens 对齐，**不阻断**后端迁移里程碑。

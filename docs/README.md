# 文档索引

本目录按**用途**分区，避免根目录堆叠。新增文档请放到对应分区，并在下表补一行（或在本文件增加小节说明）。

## 目录结构

| 目录 | 用途 |
|------|------|
| [**guides/**](./guides/) | 开发规范、接口与库表约定、迭代计划（「怎么写」） |
| [**需求/**](./需求/)（`docs/需求/`） | 产品需求、各域规格；**入口** [`需求/产品需求总览.md`](./需求/产品需求总览.md) |
| [**design/**](./design/) | **架构与编排统一稿** [`platform-architecture-unified.md`](./design/platform-architecture-unified.md)；`platform-architecture.md` 等旧文件名为重定向 stub |
| [**domain/**](./domain/) | 领域专题（LLM 模型与表结构等）；Memory 思辨见 [`需求/memory.md`](./需求/memory.md) |
| [**frontend/**](./frontend/) | 前端分层、设计系统与 UX |
| [**migration/**](./migration/) | 自 `pkg/backend` 迁 Kratos 的剧本、清单与运维 runbook |
| [**reference/**](./reference/) | 外部资料整理（含配图资产子目录） |
| [**assets/**](./assets/) | 跨文档引用的静态资源（如 SVG） |

## 常用入口

- **全栈新功能**：[`guides/AI-全栈新功能开发规范.md`](./guides/AI-全栈新功能开发规范.md)
- **接口与数据库**：[`guides/接口与数据库开发规范.md`](./guides/接口与数据库开发规范.md)
- **平台架构 · Agent 编排 · LLM Gateway**：[`design/platform-architecture-unified.md`](./design/platform-architecture-unified.md)（第一～三篇；旧路径 `platform-architecture.md` 等为 stub）
- **产品与技术需求总览**：[`需求/产品需求总览.md`](./需求/产品需求总览.md)
- **后端迁移主线**：[`migration/pkg-backend-to-kratos.md`](./migration/pkg-backend-to-kratos.md)
- **全栈迁移执行顺序**：[`migration/AI-full-stack-migration-playbook.md`](./migration/AI-full-stack-migration-playbook.md)
- **Vue 分层与自检**：[`frontend/vue-design/vue-design.md`](./frontend/vue-design/vue-design.md)
- **UI/UX token**：[`frontend/UX.md`](./frontend/UX.md)
- **模型域（需求 / 表 / 概要设计）**：[`domain/model/model.md`](./domain/model/model.md)、[`domain/model/sql.md`](./domain/model/sql.md)、[`domain/model/model-design.md`](./domain/model/model-design.md)

## 路径变更备忘（书签更新）

以下为整理时移动的旧路径 → 新路径：

- `docs/需求/0 main design.md` → 正文迁至 **`docs/design/platform-architecture-unified.md` 第三篇**（`design/platform-architecture.md` 为 stub）
- `docs/需求/总体设计文档.md` / `docs/需求/设计需求.md`（提炼部分）→ **`docs/需求/产品需求总览.md`**
- `docs/需求/设计需求.md`（原始讲义网关部分）→ **`docs/design/platform-architecture-unified.md` 第二篇**（`llm-gateway-design-reference.md` 为 stub）
- `docs/AI-全栈新功能开发规范.md` → `docs/guides/AI-全栈新功能开发规范.md`
- `docs/API/接口与数据库开发规范.md` → `docs/guides/接口与数据库开发规范.md`
- `docs/plan.md` → `docs/guides/plan.md`
- `docs/memery*.md` → 已合并为 **`docs/需求/memory.md`**（原 `docs/domain/memory/memery.md` 与 `memery-梳理副本.md` 不再保留）
- `docs/model/*` → `docs/domain/model/*`
- `docs/UI/UX.md` → `docs/frontend/UX.md`
- `docs/vue-design/*` → `docs/frontend/vue-design/*`

`.cursor/rules` 与 `web/src/css/*.sass` 中指向旧 UX / 规范路径的注释已一并更新。

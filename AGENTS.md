# Aranea-Agents — AI Agent Instructions

> Cursor / Claude Code 入口。人类开发者见 [README.md](./README.md)。

## 第一步（必读）

1. **[docs/README.md](./docs/README.md)** — 项目 AI 入口：文档索引、工作流、分级验证
2. 只读 **与当前任务相关的** 文档（按 docs/README §5 索引）；禁止为找上下文扫全库 `changelog/`

## 编码规范（按任务选读）

| 任务 | 文档 |
|------|------|
| 后端 Go | [docs/guides/AI-DEVELOPMENT-SPECIFICATION.md](./docs/guides/AI-DEVELOPMENT-SPECIFICATION.md) 速查卡 |
| 前端 | [docs/guides/frontend-guide.md](./docs/guides/frontend-guide.md) |
| Agent 运行时 | `.cursor/rules/trpc-agent-framework-first.mdc` |
| 代码探索 | CodeGraph 优先（`.cursor/rules/codegraph.mdc`） |

## 红线（违反即停）

- `internal/biz` 不得 import `pkg/trpc-agent-go`
- Runner 装配只在 `internal/service`，不在 `internal/server`
- 结构性查询：**CodeGraph 先于 grep**

完整 14 条红线见 AI-DEVELOPMENT-SPECIFICATION 速查卡。

## 验证

按改动类型选 **最小验证集**，见 [docs/README.md §4.2](./docs/README.md#42-分级验证按改动选跑)。提交 / PR 前跑全量 `make ci`。

## 任务执行

- 有任务 ID（如 `CC-A-01`、`M53-*`）时：只读对应 `*-development.md` / blueprint 中 **该 ID 块**
- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块

## 行为准则

通用编码纪律见 [docs/skills/karpathy-guidelines/SKILL.md](./docs/skills/karpathy-guidelines/SKILL.md)（优先级低于 AI-DEVELOPMENT-SPECIFICATION）。

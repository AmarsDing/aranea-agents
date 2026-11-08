# 文档已迁移

本仓库的 `doc/`、`docs/` 资料文档已于 **2026-11-08** 迁移至工作区文档中枢：

- `doc/manual/` → `../docs/aranea-agents/guide/manual/`
- `doc/architecture/` → `../docs/aranea-agents/design/architecture/`
- `doc/adr/` → `../docs/aranea-agents/design/adr/`
- `doc/assets/` → `../docs/aranea-agents/design/assets/`
- `doc/memory-challenge.md` → `../docs/aranea-agents/research/memory-challenge.md`
- `docs/development/` → `../docs/aranea-agents/design/development/`
- `docs/reports/` → `../docs/aranea-agents/review/reports/`
- `docs/scenarios/` → `../docs/aranea-agents/reference/scenarios/`
- `docs/testing/` → `../docs/aranea-agents/guide/testing/`

规则见工作区根目录 `AGENTS.md` 第 7 节「文档中枢」。新增资料文档请直接写入 `../docs/aranea-agents/<类型>/`（类型：plan / design / review / research / guide / reference），不要在本仓库重建 `doc/`、`docs/`。

注意：`internal/scenario/` 下的 agent 场景包属运行时内容，不属于文档、未迁移。

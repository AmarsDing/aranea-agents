# Evaluation 文档同步 + API 补全 + Runner SRP

**日期**：2026-05-21  
**模块**：Evaluation (33)

## 摘要

- 对照代码更新 `33 evaluation.md`、`33 evaluation.design.md`、`33-evaluation-development.md`（FrameworkBridge、LLMJudge、AnnotateCaseResult、EnsureEvalSchema ✅）。
- 新增 `UpdateDataset`（PATCH）、`DeleteRun`（DELETE）API；`DeleteDataset` 级联删除 cases。
- Runner 指标逻辑拆分至 `metrics.go`，legacy 路径移至 `runner_legacy.go`（单一职责）。
- 前端 `features/evaluation/api.ts` 移除过时的 EP-DATA-01/EP-RT-08 注释。

## 验证

```bash
make api
go test ./internal/evaluation/... ./internal/data/... -run Eval
```

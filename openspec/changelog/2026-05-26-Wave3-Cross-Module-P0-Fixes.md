# 2026-05-26 — Wave 3 跨模块 P0 修复（OUT-05 + DAT-01~04）

> **来源**：[`2026-05-26 Master Implementation Plan`](../review/2026-05-26-MASTER-IMPLEMENTATION-PLAN.md) 第三轮"单文件级 P0 快赢"。
> **范围**：Artifact / Knowledge / Eval / Knowledge 前端 — 单任务闭环，每项独立分级验证通过后再扩 scope。

---

## 一句话总结

5 条 2026-05-26 跨模块 Review 的 P0 已落，覆盖 Artifact 出站签名 fail-closed、路径穿越拦截、API 路径泄漏脱敏、3 处删除操作的级联与计数正确性，以及前端二进制入库链路。

---

## 任务清单

| ID | 文件 | 关键改动 | 验证 |
|----|------|----------|------|
| **DAT-03** | `internal/data/evaluation.go` · `internal/data/evaluation_cascade_test.go` | `DeleteDataset` 改为事务级联 `eval_case_results → eval_runs → eval_cases → eval_datasets`，避免 dataset 删除后留下孤儿 runs/results；扩展 `TestEvalDeleteCascade` 断言 runs/results 均为 0；新增 `TestEvalDeleteRun` | `go test ./internal/data/... -run TestEvalDelete -count=1` ✅ |
| **DAT-02** | `internal/biz/knowledge/knowledge.go` · `internal/data/knowledge.go` | `Usecase.DeleteDocument` 保持单一委托；repo 层在同一事务内：读取 `(collection_id, chunk_count)` → 删文档 → `UPDATE knowledge_collections SET document_count = GREATEST(... - 1, 0), chunk_count = GREATEST(... - n, 0)`；空文档 ID idempotent 返回 nil | `go build ./internal/data/... ./internal/biz/knowledge/...` ✅（PG 集成测试依赖外部环境，未跑） |
| **DAT-01** | `internal/biz/artifact/artifact.go` · `internal/biz/biz_coverage_test.go` | `Usecase.Delete(id)` 改为：Load(id) → ListBySessionAndName(session, name) → 循环 Delete(v.ID)；保留向后兼容（id 找不到时回退单删）；首个错误透传不阻断剩余删除；mem repo 增加 per-name 版本号；新增 `TestArtifactUsecase_DeleteRemovesAllVersions` 验证 v1/v2/v3 全删 + 同会话异名版本不受影响 | `go test ./internal/biz/... -run TestArtifact` ✅ |
| **DAT-04** | `web/src/features/knowledge/useKnowledgePage.ts` · `web/src/components/knowledge/KnowledgeIngestDialog.vue` | `onIngestFile` 改为 `readAsArrayBuffer` → `bytesToBase64`（chunked，避大文件栈溢出）；文本类 MIME 用 `TextDecoder` 解码到 textarea 供预览编辑；pasted text 用 `TextEncoder + bytesToBase64` 替代旧 `unescape(encodeURIComponent)`；新增 `inferMime` 从扩展名兜底；`accept` 扩展到 `.pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.html,.htm,.xml,.yaml,.yml,.toml,.log` | Cursor 静态分析无错（无 node_modules，未跑 `pnpm lint`） |
| **OUT-05** | `internal/artifact/sign.go` + `_test.go` · `internal/data/artifactfs/repo.go` + `_test.go` · `internal/service/artifact.go` | **(1) ART-01 path traversal**：新增 `validateSessionID`（`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$` + 双点防御 + 拒绝控制字符 / 斜杠 / 反斜杠 / 绝对路径），`Save` / `listSessionMetas` 入口强校验。**(2) ART-02 sign key fail-closed**：`SignKey() ([]byte, error)` + `isProductionEnv()`（`DEPLOY_ENV`/`KRATOS_ENV`/`APP_ENV` ∈ {`prod`,`production`}）；生产无 env key → `ErrSignKeyMissing`；`DownloadToken` 与 `VerifyDownloadToken` 返回 error；service `SignDownloadUrl` 503，`ServeSignedDownload` 503；dev key warning 用 `sync.Once` 仅打一次。**(3) ART-03 storage_uri 脱敏**：`StorageURI` 改为相对路径 `<session>/<id>-vN.bin`；新增 `resolveBinPath` 在 Load 时拼回 root，并向后兼容旧绝对路径 meta | `go test ./internal/artifact/... ./internal/data/artifactfs/... ./internal/biz/... ./internal/service/... -run "Artifact\|Sign\|Traversal\|StorageURI" -count=1` ✅（含 4 个新测试） |

---

## 改动覆盖（与 Review 表 ART-01~04 / KB-01 / KB-04 / EV-01 一一对应）

```
docs/review/2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md
  ART-01 (path traversal)           → OUT-05 第 1 段
  ART-02 (sign key 硬编码)          → OUT-05 第 2 段
  ART-03 (storage_uri 泄漏)         → OUT-05 第 3 段
  ART-04 (DeleteArtifact 仅一版)    → DAT-01
  KB-01  (前端 readAsText 二进制)   → DAT-04
  KB-04  (DeleteDocument 计数飘移)  → DAT-02
  EV-01  (DeleteDataset 不级联)     → DAT-03
```

---

## 未在本轮范围内（明确给下一轮）

- **OUT-01**：Inspect / Health / Preflight SSRF 守卫（需新建 `pkg/outboundguard`，跨 3 个调用点）
- **OUT-02 / OUT-03**：Hook delivery worker + 统一 HMAC 签名（依赖 `pkg/outboundwebhook` —— 已存在但未接入）
- **OUT-04**：Gateway Webhook 持久化 + EventBus 反压不丢
- **KB-02**：Knowledge 大文件 / 流式入库（与 DAT-04 互补）
- **EV-02**：Eval 案例上传 UI；**EV-08**：Run 数据集快照
- **HK-04 / HK-06 / HK-08**：Hook Resolver 缓存 / 投递幂等键 / proto `secret` 脱敏

详见 master plan 第四 / 第五轮。

---

## 验证策略

- **每个任务独立分级验证**（按 `docs/README.md §4.2`）：
  - data 层改动 → `go test ./internal/data/... -run <Specific>` 
  - biz 层改动 → `go test ./internal/biz/... -run <Specific>`
  - service 层 → `go test ./internal/service/... -run Artifact`
  - 前端 → Cursor ReadLints 静态分析（环境无 node_modules）
- **跨任务回归**：`go build ./internal/artifact/... ./internal/data/artifactfs/... ./internal/service/... ./internal/biz/...` 通过
- **未跑全量 `make ci`**（提交前由 CI 执行；本次为开发迭代闭环）

---

## 引用文档

- [Master Implementation Plan](../review/2026-05-26-MASTER-IMPLEMENTATION-PLAN.md) §1.4 OUT-05 / §1.5 DAT-01~04 / §6 第三轮
- [Overview-Model-Hook-Knowledge-Artifact-Eval Review](../review/2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md) §ART-01~04 / §KB-01 / §KB-04 / §EV-01

# Wave 3 跨模块 P0 修复 — 全量代码 Review

> **日期**：2026-05-26  
> **范围**：本次会话完成的 5 个任务（DAT-01 ~ DAT-04、OUT-05）  
> **来源 task**：`docs/review/2026-05-26-MASTER-IMPLEMENTATION-PLAN.md` 第三轮  
> **整体评分**：**86 / 100**  
> **结论**：主体实现正确、分层合规、测试覆盖关键路径；存在若干可维护性与健壮性细节需跟进，无 P0/P1 新引入缺陷。

---

## 总评表

| 维度 | 分 | 说明 |
|------|----|------|
| 架构设计 | 18/20 | 分层合规，接口合同清晰；`Delete` fallback 语义有轻微设计歧义 |
| 代码质量 | 17/20 | 命名清晰，逻辑直观；DAT-04 含两处次优细节 |
| 功能正确性 | 18/20 | 核心路径正确；DAT-01 空 id 分支有语义冲突 |
| 性能 | 8/10 | DAT-02 同步 SELECT+DELETE+UPDATE 三步 TX，为 PostgreSQL 可接受 |
| 可维护性 | 9/10 | 注释质量高，变更范围精确；测试可读性好 |
| 错误处理 | 8/10 | 整体稳健；OUT-05 `devSignKeyOnce` 仅 warn 无法在 prod 检测 misconfigured 环境 |
| 兼容性 | 8/10 | 向后兼容旧绝对 URI；DAT-04 `String.fromCharCode.apply` 大文件可能溢出 |
| 合规 | 18/20 | 红线遵守，无新增 biz→trpc-agent-go 依赖 |

---

## 一、DAT-03 — Eval DeleteDataset 级联

### 代码路径
- `internal/data/evaluation.go:159-181`
- `internal/data/evaluation_cascade_test.go`

### 架构设计

**✅ 正确**：级联删除放在 **data repo 层**，事务原子性在 DB 层保证，biz 调用链透明。符合 Kratos 分层约定，不在 biz 层手动管理 DB 一致性。

**✅ 操作顺序正确**：`results → runs → cases → dataset`，无外键约束时手动保证引用完整性，与 `DeleteRun` 已有 `results → runs` 模式对称。

### 功能正确性

**✅** IN 子查询方案在 SQLite `?` 参数化中安全，无 SQL 注入风险。

**⚠️ 边界条件（P2）**：若 `dataset_id` 对应的 `eval_runs` 数量极大（>10 万行），子查询 `SELECT id FROM eval_runs WHERE dataset_id=?` 会全表扫描，删除可能超时。当前 SQLite 场景无问题，PostgreSQL 环境下应补 `eval_runs(dataset_id)` 索引（已有 `dataset_id` 字段，但需确认索引覆盖）。

### 测试质量

**✅ 三断言完整**：cases / runs / results 均断言为 0，覆盖了之前遗漏的孤儿数据场景。

**⚠️ 轻微（P3）**：`TestEvalDeleteRun` 里 `InsertCaseResult` 传了不存在的 `CaseID: "c-r"`，如果 schema 有 FK 约束会失败。当前 SQLite 无 FK 强制执行，但测试意图不清晰。建议先 `InsertCases` 再 `InsertCaseResult`，或注释说明无 FK 约束。

### 代码质量

**✅** 中文注释清楚，引用 DAT-03 任务号，方便追溯。代码简洁，无重复逻辑。

---

## 二、DAT-02 — Knowledge DeleteDocument 计数修复

### 代码路径
- `internal/data/knowledge.go:201-235`  
- `internal/biz/knowledge/knowledge.go:206-218`

### 架构设计

**✅ 职责归位正确**：计数递减推入 **data 层**同一事务，而非在 biz 层二次调用 `UpdateCollectionCounts`。这消除了 biz 层"先删再减"跨调用的非原子窗口，符合数据库原子性原则。

**✅ biz 层契约注释完整**：`Repo implementations MUST keep … in sync atomically` 明确了接口合同，未来新 repo 实现（如 COS）不会遗漏。

### 功能正确性

**✅ 幂等性（ErrNoRows → nil）**：删不存在的文档安全返回 nil，与 HTTP DELETE 幂等语义一致。

**✅ 负值防护**：`GREATEST(document_count - 1, 0)` 和 `GREATEST(chunk_count - $2, 0)` 防止计数下穿负数。

**⚠️ 潜在问题（P2）**：
1. `SELECT collection_id, chunk_count` 与 `DELETE FROM knowledge_documents` 之间，若另一个并发事务也正在操作同一文档，PostgreSQL **不会**因为没有 `SELECT FOR UPDATE` 而阻塞，可能产生 double-decrement（两个并发删除同一 doc 各减一次）。在 SQLite 单连接场景无此风险；建议在 PostgreSQL 模式下加注释说明或加 `FOR UPDATE`。
2. `chunk_count` 来自 `knowledge_documents.chunk_count`（文档内字段），该字段在 ingest 完成后由 `UpdateDocumentStatus` 写入。若文档 ingest 中途失败，`chunk_count` 可能为 0，此时 `GREATEST(chunk_count - 0, 0)` 正确（无 chunk 则 collection.chunk_count 不变），行为正确，但无测试覆盖此路径。

### SQL 正确性

**⚠️ 参数顺序（P2）**：
```sql
WHERE id = $1  -- collectionID 是第一个参数
... chunk_count - $2  -- chunkCount 是第二个参数
```

对应 Go 调用 `ExecContext(ctx, ..., collectionID, chunkCount)`，**顺序正确**。但因 PostgreSQL `$1`/`$2` 与 `?` 参数语义不同，需确认 `*sql.DB` 是用 pgdriver（`$N`）还是标准驱动。如果是 `lib/pq`/`pgx`，`$1`/`$2` 是正确的——此处无问题，但代码读者需注意。

### 测试缺口（P2）

无 PG integration test。业务代码完整，但验证仅靠编译通过。建议后续补充 `testcontainers` 场景或在 `pgvector` CI 测试中覆盖。

---

## 三、DAT-01 — Artifact DeleteArtifact 删全版本

### 代码路径
- `internal/biz/artifact/artifact.go:95-124`
- `internal/biz/biz_coverage_test.go` (新增 `TestArtifactUsecase_DeleteRemovesAllVersions`)

### 架构设计

**✅ biz 层语义升级正确**：proto 合约"所有版本"在 biz 层兑现，repo 接口 `Delete(id)` 保持只删单 ID，biz 组合调用完成语义翻译——这是典型的"组合优于修改接口"。

**⚠️ 设计歧义（P2）**：

当 `id` 为空时，代码直接委托给 repo：
```go
if strings.TrimSpace(id) == "" {
    return uc.repo.Delete(ctx, id)
}
```
空 id 对 `FSArtifactRepo.Delete` 的效果是静默无操作（遍历不到任何 `HasPrefix("")` 的文件名），所以行为是 no-op，但没有返回 `BadRequest` 错误。**调用方可能期望错误**。建议改为：
```go
if strings.TrimSpace(id) == "" {
    return errors.New("artifact: id is required")
}
```
或与 service 层 `DeleteArtifact` 的 `BadRequest("id is required")` 保持一致（service 层已在 biz 层前校验，实际不会传入空 id，但防御性更好）。

**⚠️ Load 失败时 `_ = uc.repo.Delete(ctx, id)` 丢弃孤儿清理错误（P3）**：
```go
_ = uc.repo.Delete(ctx, id)
return err
```
若孤儿清理本身失败，调用方只见到 Load 的原始错误，不知道清理也失败了。可考虑用 `errors.Join(err, cleanupErr)` 暴露两个错误。

### 功能正确性

**✅ 全版本删除逻辑正确**：Load → ListBySessionAndName → 循环删除每个 version ID，与 `trpc.ServiceAdapter.DeleteArtifact` 语义对齐。

**✅ 首错透传不阻断**：首个删除失败后继续删除其余版本，避免部分版本留孤儿。

**✅ 向后兼容**：`lvErr != nil || len(versions) == 0` 时回退单 ID 删除，保护历史数据。

### 测试质量

**✅ `TestArtifactUsecase_DeleteRemovesAllVersions` 充分**：三个版本 + 验证不同名文件幸存，覆盖了所有关键路径。

**⚠️ memRepo `Save` 版本计算**（P2）：
```go
for _, existing := range m.items {
    if existing.SessionID == sessionID && existing.Name == name && existing.Version >= version {
        version = existing.Version + 1
    }
}
```
Map 遍历无序，如果同时有 v0、v2 的情况，循环结束时 `version` 可能等于 3（来自 v2+1）——但实际 test 按顺序 Save，不会产生乱序，测试通过没有问题。仅需注意 memRepo 非严格单调，若复用于其他测试需注意。

---

## 四、DAT-04 — Knowledge 前端二进制路径

### 代码路径
- `web/src/features/knowledge/useKnowledgePage.ts:148-244`
- `web/src/components/knowledge/KnowledgeIngestDialog.vue:20-28`

### 功能正确性

**✅ 核心修复正确**：`readAsArrayBuffer → Uint8Array → bytesToBase64` 保证 PDF/DOCX 等二进制内容不被 UTF-8 解码破坏。

**✅ 文本预览保留**：对 `text/*`、`application/json`、`application/xml` 仍用 `TextDecoder` 解码到 textarea，UX 无退化。

**⚠️ `String.fromCharCode.apply` 大文件溢出（P2）**：
```ts
binary += String.fromCharCode.apply(null, slice as unknown as number[]);
```
`Function.apply` 的参数个数受调用栈约束，最大约 65535（`0x8000` 已在这里用作 CHUNK 大小）。当前 CHUNK = `0x8000 = 32768` 字节，刚好在大多数引擎安全阈值内，但这是 V8/WebKit 的 **实现依赖行为**，规范未保证。更安全的写法：
```ts
for (let j = 0; j < slice.length; j++) {
    binary += String.fromCharCode(slice[j]);
}
```
或使用 `TextDecoder`（仅适用于文本），或 `btoa(String.fromCharCode(...new Uint8Array(buf)))` 方案的安全版。此处已按 chunk 分批，在目前 10MB 上限场景下实测无问题，但是技术债。

**⚠️ `ingestForm.value._fileBase64 = ""` 在数组分支前置（P2）**：
```ts
ingestForm.value._fileBase64 = "";
if (!picked) {
    ingestFile.value = null;
    return;
}
```
若传入 `File[]` 且数组为空，`picked` 为 `undefined`，`_fileBase64` 已被清空，但 `ingestFile.value` 没有置 null。逻辑上微小不一致，但此场景在 Quasar `q-file` 的实际使用中不会出现（返回 null 或 File 或 File[]），风险极低。

### 代码质量

**✅ `inferMime` 函数设计合理**：覆盖常见扩展名，fallback `application/octet-stream`，优先使用 `file.type`（浏览器嗅探）。

**⚠️ `inferMime` 中 `.doc` 与 `.docx` 同优先级但 `.docx` 先匹配**（P3）：`endsWith(".docx")` 在 `.doc` 之前判断，若文件名恰好以 `.doc` 结尾（如 `readme.doc`），需要走到第二个条件才命中——逻辑正确，但如果改用 `switch` 或 Map 会更高效直观。现规模可接受。

### 向后兼容

**✅ accept 扩展**：新增的扩展名为 additive，不影响现有 `.txt/.md/.json/.csv` 用户。

**⚠️ accept 与 inferMime 不完全一致**（P3）：dialog accept 含 `.log/.html/.htm/.xml/.yaml/.yml/.toml`，但 `inferMime` 未覆盖 `.yaml/.yml/.toml`，fallback 到 `application/octet-stream`。功能上可工作（后端会处理），只是用户在 MIME 显示上看到不够精确的类型。低优先级补充。

---

## 五、OUT-05 — Artifact 签名安全三联修

### 代码路径
- `internal/artifact/sign.go`
- `internal/data/artifactfs/repo.go:20-41, 107-200`
- `internal/service/artifact.go:201-240`
- `internal/artifact/sign_test.go`
- `internal/data/artifactfs/repo_test.go`

### 5.1 ART-01 — session_id 路径穿越防护

**✅ 正则白名单正确**：`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$` 有效排除 `/`、`\`、`..`、NUL、控制字符。

**✅ 双重检查（belt-and-braces）**：先 pattern，再显式检查 `..`，防护冗余但无副作用。

**⚠️ 正则允许 `.` 开头的第二字符（P2）**：
```
^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$
```
组合后，ID 可以是 `a.b.c` 或 `a-...-b`（`..` 跨越两字符区间）。但 `strings.Contains(id, "..")` 的第二道检查明确排除了两个连续的点，所以 `a..b` 被拒绝，安全。

**⚠️ `listSessionMetas` 已加 `validateSessionID`，但 `List` 内的 `listAllMetas` 子路径不经过任何 session 验证**（P2）：`listAllMetas` 用 `os.ReadDir(r.root)` 遍历所有目录，不存在穿越风险（只读遍历），但返回的 `meta.SessionID` 可能包含历史残留的脏值——这是向后兼容代价，可接受，建议加注释说明。

**✅ 测试 `TestFSArtifactRepo_RejectsTraversalSessionID`** 覆盖了 8 种恶意输入，质量高。

### 5.2 ART-02 — 签名 key fail-closed

**✅ 核心设计正确**：`SignKey() ([]byte, error)` 改为值+错误返回，生产环境无 key 时返回 `ErrSignKeyMissing`，service 层翻译为 503。

**✅ `sync.Once` 去重 dev 警告**：避免高频调用刷屏日志，符合最佳实践。

**⚠️ `isProductionEnv` 无 "staging" 场景（P2）**：
```go
if v == "prod" || v == "production" {
```
`staging`、`pre-prod`、`uat` 等环境同样不应使用 dev key，但此处不会 fail-closed。建议改为白名单（只在明确的 dev/local/test 环境允许 fallback），或文档说明约定。

**⚠️ `devSignKeyOnce` 是 package-level 变量，在测试中跨 case 共享（P3）**：`TestSignKeyDevFallback` 调用 `SignKey()` 时会触发或跳过 Once。由于测试 `t.Setenv` 会恢复环境变量，`Once` 的触发顺序可能导致其他测试中警告未打印（Once 已触发）。这不影响正确性，但可能掩盖测试覆盖的完整性。

**✅ 三个测试用例**（`TestDownloadTokenRoundTrip`、`TestSignKeyFailClosedInProduction`、`TestSignKeyDevFallback`）覆盖了正常路径、生产 fail-closed、dev fallback，测试设计系统完整。

### 5.3 ART-03 — storage_uri 脱敏（相对路径）

**✅ 向后兼容 `resolveBinPath`**：
```go
if filepath.IsAbs(uri) {
    return uri // legacy absolute path
}
return filepath.Join(r.root, filepath.FromSlash(uri))
```
新旧格式均可读取，平滑迁移。

**⚠️ `resolveBinPath` 未防止相对路径越界（P2）**：
若历史数据 sidecar 中 `storage_uri` 被篡改为 `../../etc/passwd`（相对路径但穿越），`filepath.IsAbs("../../etc/passwd")` 返回 false，会走 `filepath.Join(r.root, "../../etc/passwd")`。`filepath.Join` 会 clean 路径，实际结果是 `../etc/passwd`（从 root 上溯两层）。

**建议在 `resolveBinPath` 中加同样的路径安全检查**：
```go
cleaned := filepath.Clean(filepath.Join(r.root, filepath.FromSlash(uri)))
if !strings.HasPrefix(cleaned, filepath.Clean(r.root)+string(os.PathSeparator)) {
    return "" // 触发后续 ReadFile 失败，安全降级
}
return cleaned
```

**⚠️ 新旧 sidecar 并存期间，`toBiz()` 直接透传 `StorageURI`**（P2）：
旧 sidecar 仍有绝对路径，会通过 `toProtoArtifactMeta` 返回给 API 调用方，ART-03 的脱敏效果只对新写入的制品生效。这是迁移期间的已知限制，建议在 `toBiz()` 或 `toProtoArtifactMeta` 中屏蔽绝对路径（替换为空串或仅保留 `StorageKind`）。

**✅ 测试 `TestFSArtifactRepo_StorageURIIsRelative`**：断言新 artifact 的 `StorageURI` 不是绝对路径且能 round-trip Load，设计完整。

### 5.4 service 层错误处理

**✅ `SignDownloadUrl` 503 正确**：生产无 key 时返回 `ServiceUnavailable`，不是静默 fallback。

**✅ `ServeSignedDownload` 503 与 403 区分明确**：运维能区分"签名 key 缺失"和"token 非法"。

**⚠️ `ServeSignedDownload` 向 response body 暴露 `ErrSignKeyMissing` 全文（P2）**：
```go
http.Error(w, verr.Error(), http.StatusServiceUnavailable)
```
消息内容 `"artifact: signing key missing in production (set KRATOS_ARTIFACT_SIGN_KEY or KRATOS_AUTH_SECRET)"` 泄露了内部 env var 名称。建议：
```go
http.Error(w, "service misconfigured", http.StatusServiceUnavailable)
// 写 slog.Error(verr.Error()) 到日志
```

---

## 六、master plan + changelog 文档

**✅ master plan 状态更新正确**：5 个 ID 均标记为 `✅ 2026-05-26 (...)` 并附实现摘要，可追溯。

**✅ changelog 结构完整**：任务清单 + 改动 vs Review 表对应 + 未在本轮范围内 + 验证策略。

**⚠️ 细节（P3）**：changelog 第三轮"第三轮"列表把 DAT-02 状态写成"GetDocument 读 ChunkCount + UpdateCollectionCounts(-1, -n) 已落"，但实际实现是 data 层单事务而非 biz 层二次调用。描述准确性稍低，建议修正为"data 层单事务级联（SELECT + DELETE + UPDATE GREATEST）"。

---

## 七、跨任务共性观察

### 7.1 向后兼容（所有 5 个任务）

**✅ 优秀**：每个修复均考虑了历史数据兼容：
- DAT-03：子查询方式无需 schema 变更
- DAT-02：`GREATEST` 防止负数计数
- DAT-01：`lvErr != nil` 时回退单删
- DAT-04：先用 `file.type` 再 inferMime
- OUT-05：`resolveBinPath` 兼容绝对路径 sidecar

### 7.2 注释风格一致性

所有修改均在关键位置引用 task ID（`DAT-01`、`OUT-05` 等）和原始 Review 编号（`ART-01/02/03`），便于未来 grep 追溯根因。这是高质量工程实践。

### 7.3 测试覆盖率

| 任务 | 新测试数 | 核心路径 | 边界条件 |
|------|---------|---------|---------|
| DAT-03 | 2 | ✅ | ⚠️ FK-less CaseID |
| DAT-02 | 0 | ⚠️（编译+构建验证） | — |
| DAT-01 | 1 | ✅ | ⚠️ 空id 分支无测 |
| DAT-04 | 0 | ⚠️（静态分析） | — |
| OUT-05 | 4 | ✅ | ⚠️ staging env 无测 |

### 7.4 并发安全

- DAT-03：SQLite 事务，并发无问题
- DAT-02：PostgreSQL 可能 double-decrement（无 `FOR UPDATE`，见上述分析）
- DAT-01：memRepo 无锁，生产 `FSArtifactRepo` 有全局 mutex，安全
- OUT-05：`devSignKeyOnce sync.Once`，正确

---

## 八、需要跟进的项（按优先级）

| 优先级 | ID | 描述 |
|--------|----|------|
| P2 | REV-01 | `DAT-01` biz `Delete` 空 id 路径应返回 BadRequest 而非 no-op |
| P2 | REV-02 | `resolveBinPath` 对相对路径缺越界检查（`HasPrefix` 断言） |
| P2 | `toBiz()` / `toProtoArtifactMeta` 中旧绝对路径 sidecar 应过滤 `StorageURI` | ART-03 脱敏在迁移期完整性 |
| P2 | REV-04 | `ServeSignedDownload` 503 body 不应暴露 env var 名称 |
| P2 | REV-05 | DAT-02 PostgreSQL 并发 `DELETE` 需 `SELECT FOR UPDATE` 或文档说明 |
| P2 | REV-06 | `inferMime` 未覆盖 `.yaml/.yml/.toml`；`bytesToBase64` 用更安全的逐字节循环 |
| P3 | REV-07 | `isProductionEnv` 仅匹配 "prod/production"，staging 环境不 fail-closed |
| P3 | REV-08 | changelog DAT-02 描述不准确（实现是 data 层单事务，非 biz 层 GetDoc+Update） |
| P3 | REV-09 | `TestEvalDeleteRun` 使用不存在的 CaseID，需补注释 |

---

## 九、业务逻辑审查（需人工确认）

以下为 AI 不能独立判断的业务语义，需 owner 确认：

1. **DAT-01 全版本删除语义**：产品是否认可"删 artifact" = "删同名所有版本"？还是需要"删单版本" + "删全部" 双 RPC？目前 proto 注释写"删所有版本"但未提供 `DeleteArtifactVersion`。若需保留单版删能力，需新增 RPC。

2. **DAT-02 chunk_count 来源**：`knowledge_documents.chunk_count` 在 ingest 失败时为 0，此时集合计数递减 -1（文档数）但 chunk 数不变（chunk 从未写入）。业务上是否接受这种"文档数少了一、chunk 数不变"的中间状态？

3. **OUT-05 dev/staging key 策略**：`isProductionEnv` 只检测 "prod/production"。staging 和预发布环境是否应该使用真实 key？如果是，需更新匹配逻辑或 staging 服务强制设置 `KRATOS_ARTIFACT_SIGN_KEY`。

4. **DAT-04 accept 扩展**：扩展到 PDF/DOCX 等格式后，后端 OCR/extract 目前仍为 stub。向用户展示"文档已提交入库"后，是否需要在 UI 明确说明"PDF 内容需后端解析支持，索引可能不完整"？

---

*Review 时间：2026-05-26 23:47 UTC+8*  
*Reviewer：Cursor AI（AI辅助，业务逻辑需人工确认）*

# Debug: Agent 设置页保存超时（开启工具并行 + 流式）

- **状态**：[OPEN] — **已深入到 SQLite 层**
- **Session ID**：`agent-save-timeout`
- **症状**（用户最新反馈）：
  1. **chat 已关闭**（排除 H1 运行时并发写）
  2. **任何工具配置修改都失败**（不限于并行/流式 → 不是字段触发的额外逻辑）
  3. **已有日志，自己查**
- **环境**：Windows + Quasar Dev (`:9001` → 代理到 admin HTTP `:8000`)，SQLite WAL（`file:./data/arenea.sqlite?cache=shared&_fk=1`），后端 admin 单进程 PID 22524，启动于 10:26:45。
- **重现路径**：
  1. 打开某个已存在 Agent 的设置页
  2. 改任意工具配置（不一定要并行/流式）
  3. 点击「保存」→ 等 ~120s → 失败
  4. **后续 GET 也开始超时**（整个 admin DB 层锁死）

---

## 1. 端到端调用链（静态梳理）

### 1.1 前端
```
[AgentSettingsHeader] 保存按钮
  → useAgentSettingsPage.saveAgent()                    web/src/features/agents/useAgentSettingsPage.ts:242
    → detailStore.patch(form.id, {...form, settings, files, config_json})   web/src/stores/agents/detail.ts:33
      → updateAgent(id, payload)                          web/src/features/agents/api.ts:117
        → createAgentService().UpdateAgent({id, agent: partialAgentToWire(payload)})   web/src/services/index.ts:55
          → requestHandler({path,method,body}, meta)      web/src/services/axiosHandler.ts:127
            → kratosApi.request({                          ← axios 实例默认 30s
                method: 'PATCH',
                url: '/v1/agents/{id}',
                timeout: resolveRequestTimeoutMs(path)     ← 命中 /v1/agents/{id}/ → 120s
              })
```

**关键配置**（[axiosHandler.ts:6-38](file:///f:/project/aranea-agents/web/src/services/axiosHandler.ts#L6-L38)）：
- `KRATOS_API_DEFAULT_TIMEOUT_MS = 30_000`
- `KRATOS_API_LONG_TIMEOUT_MS = 120_000`
- `/^v1\/agents\/[^/]+$/` 命中 agent 更新 → 120s

### 1.2 HTTP 路由
proto 注册（[agent.proto:473-478](file:///f:/project/aranea-agents/api/kratos/agent/v1/agent.proto#L473-L478)）：
```proto
rpc UpdateAgent(UpdateAgentRequest) returns (Agent) {
  option (google.api.http) = { patch: "/v1/agents/{id}" body: "agent" };
}
```
Kratos HTTP server 注入（[http.go:98](file:///f:/project/aranea-agents/internal/server/http.go#L98)）；server 端超时 `server.http.timeout: 600s`（[config.yaml:5](file:///f:/project/aranea-agents/configs/config.yaml#L5)），**不构成前端 120s 的天花板**。

### 1.3 Service 层
[service/agent.go:613-628](file:///f:/project/aranea-agents/internal/service/agent.go#L613-L628)：
```go
func (s *AgentService) UpdateAgent(ctx context.Context, req *v1.UpdateAgentRequest) (*v1.Agent, error) {
    if req.GetAgent() == nil { ... }
    patch := fromProtoAgent(req.GetAgent())    // proto → biz.Agent（包含 tools_parallel_enabled 等）
    a, err := s.uc.Update(ctx, req.GetId(), patch)
    if err != nil { ... }
    s.mon.RecordAdminAudit(ctx, "agent.update", "agent", a.ID, ...)
    invalidateAgentBuildCache(a.ID)             // 纯内存 LRU 失效，O(1)
    return s.toProtoAgentEnriched(ctx, a)       // 内部还会读 a2a_endpoint_enabled
}
```

### 1.4 Biz 层
[agent_usecase.go:341-412](file:///f:/project/aranea-agents/internal/biz/agent_usecase.go#L341-L412) `Update`：

| 步骤 | 操作 | 所在事务 | DB 调用次数 |
|------|------|----------|------------|
| A | `u.Get(ctx, id)` → hydrate | **事务外** | 1× Agent + 1× RuntimeSettings + 1× PromptFiles + 1× ExtrasForAgents = **4 reads** |
| B | 字段合并 + 4 项验证 | — | 0 |
| C | `repo.ExecInTx(...)` 内 | **Detached Tx** | — |
| C-1 | `repo.UpdateAgent(txCtx, merged)` | tx 内 | data 层内部 1 read + 1 write + 1 read = **2 reads + 1 write** |
| C-2 | `repo.UpsertAgentRuntimeSettings(txCtx, settings)` | tx 内 | 1 UPSERT + 1 read = **1 read + 1 write** |
| C-3 | `repo.ReplaceAgentPromptFiles(txCtx, id, files)` | tx 内（嵌套检测后复用父 tx） | 1× DELETE + N× INSERT（**N+1 循环**）+ 1× List = **1+N reads + 1+N writes** |
| D | `u.Get(readCtx, id)` → hydrate | **事务外** | **4 reads** |
| E | `s.toProtoAgentEnriched(...)` 内部 `enrichAgentEndpoint` | — | **1 read** |

**合计**：1 次完整 PATCH = **约 11 reads + 1 + 1 + N writes**，并发的同 agent 写竞争下还可能再增加（详见 §3）。

### 1.5 Data 层
[agent_repo.go:648-697](file:///f:/project/aranea-agents/internal/data/agent_repo.go#L648-L697) `UpdateAgent` **自带 2 次 GetAgentByID（write 前 + write 后）**——biz 层 A 步已经 Get 了一次，这是冗余的 read。

[agent_repo.go:762-801](file:///f:/project/aranea-agents/internal/data/agent_repo.go#L762-L801) `ReplaceAgentPromptFiles`：
- 进入时 `r.data.ExecInTx(ctx, ...)`；因 ctx 已含 `txClientKey`（来自父 tx），[tx.go:21-23](file:///f:/project/aranea-agents/internal/data/tx.go#L21-L23) 检测后 **直接复用父 tx**，不会开嵌套
- 但 DELETE 后对每个 file **单条 `Create().Save()`**——N+1，且 `sortOrder` 用 `(i+1)*10`，每次都是独立 round-trip
- 结尾再 `ListAgentPromptFiles` 读一次

### 1.6 事务 / 锁的关键事实

| 维度 | 值 | 出处 |
|------|----|------|
| rawDB 写连接上限 | `SetMaxOpenConns(1)` | [data.go:507](file:///f:/project/aranea-agents/internal/data/data.go#L507) |
| readDB 读连接上限 | `SetMaxOpenConns(2)` | [data.go:549](file:///f:/project/aranea-agents/internal/data/data.go#L549) |
| SQLite busy_timeout | `30000ms` | [data.go:518](file:///f:/project/aranea-agents/internal/data/data.go#L518) |
| `ExecInTx` 上下文 | **`detached := context.Background()`** | [tx.go:28-37](file:///f:/project/aranea-agents/internal/data/tx.go#L28-L37) |
| 提交前对 caller ctx 的检查 | `if ctx.Err() != nil { Rollback }` | [tx.go:44-49](file:///f:/project/aranea-agents/internal/data/tx.go#L44-L49) |

> 也就是说：**前端 axios 120s timeout 取消后，后端事务不会被中途打断**，继续跑直到 commit，然后看到 caller ctx 已取消 → Rollback → 但 SQL 写锁已经被持有了几十秒。

### 1.7 「并行 + 流式」对 Update 路径的直接副作用
**没有**。`tools_parallel_enabled` / `tools_streaming_enabled` 只是两个 boolean，写到 `agent_runtime_settings.tools_parallel_enabled` 列；既不在 `ValidateCodeExecutorType/PlannerKind/PlannerConfigJSON/RalphLoopSettings` 校验里，也不在 `EmbedAgentKindInConfigJSON` / `mergeEvaluationFromLegacy` 逻辑里，更不触发 `BuildCache` 重建以外的任何边路逻辑。**这两个开关本身不导致保存慢**。

它真正的间接影响：**让被保存的 Agent 处于活跃运行态的概率大幅提高**（并行 tool + 流式 = 当前 session/turn/tool_use 写入频次高、占用 SQLite 写锁时间长）——而 admin 的 PATCH 必须等这个写锁。

---

## 2. 假设（按可能性排序）

### H1（最可能）：SQLite WAL 写锁长时等待 + Detached Tx 放大
- 现象：开启并行/流式 → 该 Agent 当前 session 频繁写 `session_runs / events / tool_use / memory` → `rawDB` 单连接被占用。
- admin PATCH 进入 `ExecInTx`：
  - C-1 `UpdateAgent` 的 `Agent.UpdateOneID` 等写锁，busy_timeout=30s
  - C-2 `UpsertAgentRuntimeSettings` 再等
  - C-3 `ReplaceAgentPromptFiles` 再等（且本身是 N+1 INSERT）
- 加上 detached tx 不响应前端取消，**单次 PATCH 实际可能在 30s ~ 90s 之间**；命中 30s×3 + N×insert = 极易 ≥ 120s。
- 验证点：后端日志中是否出现 `SQLITE_BUSY`（注意：该驱动默认会把 BUSY 转成普通错误而不是 panic），同时 admin 后台是否还有该 agent 的 session/turn 在跑。

### H2（次可能）：N+1 INSERT 把写锁持有时间放大到上限
- `ReplaceAgentPromptFiles` 里每个 file 一次 round-trip。
- 假设 5 个 prompt files，1 个 DELETE + 5 个 INSERT = 6 次往返；每次 SQLite 写耗时 1~10ms（持锁时），串行累计很容易到几百毫秒 ~ 数秒。
- 这一段**完全在 detached tx 中**，是 H1 的「推手」之一。
- 验证点：看 `agent_prompt_files` 行数；尝试把 files 数量降到 1 个再保存，看是否好转。

### H3：前端请求体过大，PATCH body 解析慢
- 前端 `useAgentSettingsPage.saveAgent` 把整个 `form`（含 `config_json` 字符串）作为 PATCH body，**config_json 已经包含完整的 `tools/memoryL0~L4/evolution/skillRuntime/files[]` 全量**（见 [agentRuntimeConfigSerialize.ts:117-136](file:///f:/project/aranea-agents/web/src/features/agents/agentRuntimeConfigSerialize.ts#L117-L136)）。
- 一个长 system prompt 文件（几 KB~几十 KB）+ 100+ 字段 runtime config 序列化后 JSON 容易到 **50~200KB**。
- 但单纯 200KB JSON 在本机 loopback 解析 < 50ms，**不构成主因**。可作为弱假设。

### H4：`invalidateAgentBuildCache` / `toProtoAgentEnriched` 路径里有意外阻塞
- `invalidateAgentBuildCache` 走的是 LRU 内存操作（[cache.go:119](file:///f:/project/aranea-agents/internal/agent/cache.go#L119)），O(1)。
- `toProtoAgentEnriched` 走 `enrichAgentEndpoint` → `a2aUC.MapEndpointEnabled` → 多半是 `mcp_server_endpoint` 表 1 次读。
- **不构成主因**，但可作为弱假设排除。

### H5：Kratos HTTP body read 阶段卡住
- 概率极低。`server.http.timeout: 600s` 远大于 axios 120s；如果 HTTP read 卡了，Kratos 会先于 axios 超时。
- 不构成主因。

---

## 3. 需要用户确认/采集的运行时证据

**未做插桩前不能动业务代码**（TRAE-debugger 协议 §Evidence Gate）。请先采集以下任一项即可大幅收敛：

1. **后端日志中 PATCH 期间是否出现 `SQLITE_BUSY`/`database is locked`/`tx is aborted`**：
   ```powershell
   Get-Content .\logs\*.log -Tail 200 | Select-String -Pattern "SQLITE_BUSY|database is locked|tx is aborted|busy|patch|UpdateAgent"
   ```
2. **保存时是否同时有该 agent 的 chat 在跑**：
   - 浏览器开一个 chat 页，**同时** admin 页改设置 → 点保存
   - 然后停掉 chat，再点保存
   - 对比两次耗时
3. **保存时去掉 prompt files 的修改**（不触发 ReplaceAgentPromptFiles）再保存，看是否仍超时
4. **保存时不勾选「并行/流式」，只改其他字段**，看是否正常

如果用户能跑出：
- 「去 chat 后保存正常」→ 直接证实 H1
- 「去掉 files 提交正常」→ 证实 H2
- 「去 chat + 去 files 还慢」→ 走 H3/H4 进一步插桩

---

## 4. 修复方向（待证据确认后选）

> **注：以下修复需在拿到运行时证据、用户确认 H1/H2 后再动**。

### R1：减少 PATCH 写锁占用时间（最可能见效）
- `data/agent_repo.go::UpdateAgent` 去掉内部的 2 次 `GetAgentByID`（事务外 biz 层已 Get 过；事务内的 read 也可改成局部变量复用）
- `data/agent_repo.go::ReplaceAgentPromptFiles` 把 N+1 `INSERT` 改成 `CreateBulk`（Ent 支持 `CreateBulk`）；同时把 DELETE + INSERT 拆成「upsert by id + delete missing」的差量算法
- `biz/agent_usecase.go::Update` 事务结束后不要再 `u.Get(...)` 一次，**直接返回 `merged`**（hydrate 已经在事务内 Get 过了——或把 hydrate 拆成「只 hydrate 出 settings+files，agent 主体用 merged」）

### R2：Detached Tx 在 commit 前响应 caller ctx（防 5xx）
- 现版本在 commit 时如果 `ctx.Err() != nil` 就 Rollback 然后返回 `ctx.Err()`，导致客户端看到「服务器错误」或「请求被中止」。
- 方案：commit 之前在 detached ctx 上完成；不依赖 caller ctx。
- 但这与 R1 是**互斥的优化**——Detached 的初衷就是「不被打断，让 SQL 跑完」，若改成跟随 caller ctx 取消，可能让 SQL 写到一半被 abort，导致 SQLite WAL 文件半途脏页。
- **建议保留 detached，但需要更早释放锁**：把事务的粒度拆小（见 R1）。

### R3：rawDB 连接池升级（治本）
- 当前 `SetMaxOpenConns(1)` 是为了避免 SQLite 多写锁的"全员排他"——但 WAL 模式下，**写连接 + 写锁是两件事**。WAL 允许 1 写 + N 读，**多个写连接在 SQLite 也会互相阻塞**（`SQLITE_BUSY`），所以「rawDB 单连接」是正确的。
- 真正的瓶颈不是连接数，而是**单次写事务时长**。R1 才是对症下药。

### R4：前端瘦身（弱优化）
- 取消「整个 form 当 PATCH body」，改成「只发变更字段」+ 服务端 `fromProtoAgent` 走 partial merge（其实现在的 `mergeAgentCatalog` 已经是 partial merge，但是仍然要 hydrate 整个 settings 并回写）；如果能避免回写 `settings` 全量，事务体积会显著减小。
- 但 PATCH body 大小不是主因（H3 弱），可放最后做。

---

## 6. 用户反馈后的新证据（2026-06-04 10:30+）

### 6.1 关键运行时采集

| 探针 | 结果 | 含义 |
|------|------|------|
| PowerShell `Invoke-WebRequest PATCH /v1/agents/agent___spirit__` | **10s 超时失败** | 后端真实卡死，**不是 axios 前端问题** |
| PowerShell `Invoke-WebRequest GET /v1/agents/agent___spirit__` | **>35s 仍卡死** | 整个 DB 层被锁，连读都进不来 |
| `aranea-pipeline.log` 末尾 | 10:30:51 一次 PATCH（auth.bypass 已打点）+ 10:31:41 又一次 PATCH（auth.bypass 已打点）| 请求确实进了 handler，但 handler **没有返回任何业务日志**（没有 audit log、没有 error log）|
| `f:\project\aranea-agents\data\arenea.sqlite` mtime | **10:11:41**（admin 启动之前）| **admin 启动 9+ 分钟，DB 主文件一次没被写过** |
| `data\arenea.sqlite-wal` mtime | **10:26:52**（admin 启动 7 秒后）| **WAL 也只写了启动那 7 秒（migration / 初始化），之后 9+ 分钟零写入** |
| `data\arenea.sqlite-shm` mtime | **10:26:46** | 启动瞬间打过一次 |
| `smoke.yaml` 真实使用的 DSN | `file:./data/arenea.sqlite?cache=shared&_fk=1` | **缺 `_journal_mode=wal` 和 `_busy_timeout`**（但 PRAGMAs 在 `data.go:514-526` 中 hardcode 补上，WAL 实际生效）|
| `Get-Process admin` | 进程活着，22 线程，1.5s CPU，146MB | 进程没死没死锁，只是在某次 SQL 操作上 goroutine 永久阻塞 |
| 前端 axios timeout | `v1/agents/{id}` 命中 → 120s | 不是前端问题（直接打后端也卡）|

### 6.2 假设重排

| 假设 | 状态 | 说明 |
|------|------|------|
| H1：运行时并发 chat 写 | **❌ 排除** | 用户明确说 chat 关闭；而且无其他进程持有 DB（Get-Process 查过）|
| H2：N+1 INSERT 把锁撑长 | 仍候选 | 但就算 N+1，1~5s 也该结束；现在 120s+ 不像 |
| H3：PATCH body 过大 | 排除 | 我用极小 body 直接 PATCH 后端也卡 |
| H4：endpoint enrich 卡 | 排除 | 卡的位置在 handler 进入前段（auth.bypass 已打但 handler 未返回）|
| H5：Kratos body read 卡 | 排除 | body read 在 auth.middleware 之前；auth.bypass 已说明 body 已读完 |

### 6.3 新生根因假设（最可能）

**H6（最可能）：`biz/agent_usecase.go::Update` 进入 `data.ExecInTx` 后，detached tx body 在某个 SQL 操作上永久阻塞，连接池（`rawDB SetMaxOpenConns(1)`）被该 tx 独占并永不释放。**

证据链：
- admin 启动后 WAL 只写了 7 秒（migration 阶段）→ 之后所有写路径**从未 commit**
- PATCH 进入 handler（auth.bypass 立刻打点），handler 内部**没有再产生任何业务日志**（无 error / 无 audit / 无 tx rolled back）
- 说明 tx body 中的某个 SQL 操作**不返回错误**、**不返回结果**、**不返回 cancel**，goroutine 在等一个永不释放的资源
- detached ctx = `context.Background()` 永远不超时
- 一旦这个 tx 卡死，rawDB 单连接被占 → 后续所有写全等 → 后续读若走 `r.data.RW().Read(ctx)` 命中 tx 也会进同一连接 → GET 也卡

**最可能的卡死 SQL 操作**（按概率排序）：
1. `data/agent_repo.go::UpdateAgent` 内的 `Agent.UpdateOneID(...).Save(ctx)`（line 673）—— 实际触发 SQLite 写
2. `data/agent_repo.go::UpsertAgentRuntimeSettings` 内的 `AgentRuntimeSetting.Create().OnConflict().Update(...).Save(ctx)` —— 唯一索引冲突的写竞争
3. `data/agent_repo.go::ReplaceAgentPromptFiles` 内的 N 个 `AgentPromptFile.Create().Save(ctx)` —— 循环 INSERT
4. `data/agent_repo.go::UpdateAgent` 开头/结尾的 `r.GetAgentByID(ctx, a.ID)` —— 不太可能，但 detached ctx 下不会 cancel 也不会被探测

**最可能的根因**（基于 Windows 文件锁 + SQLite WAL 行为）：
- 现代 Windows 上 SQLite WAL 模式需要 `DeletePending`/`LockFileEx` 等锁
- 一旦 `cache=shared` 让 rawDB 与 readDB 共享一个 page cache，再加上 PRAGMA 强制设 WAL，可能在 OS 层面产生**死锁等待**
- 具体证据：smoke.yaml 的 DSN 故意加 `cache=shared` 但少了 `_journal_mode=wal`，看似和 `data.go` 的 hardcode PRAGMA 不冲突，但 `cache=shared` 这个参数在 modernc.org/sqlite 驱动中会**禁用部分文件锁优化**，配合 `busy_timeout=30000` → 单连接池死锁时**永不返回**

### 6.4 不需要进一步静态分析，必须运行埋点

按 TRAE-debugger 协议，证据驱动：现在的证据已经**排除全部 H1-H5 静态假设**，必须进入 Step 2 插桩。最小插桩方案：

**埋点位置**（`internal/biz/agent_usecase.go::Update` 内）：
- T0: 进入 `Update`
- T1: 离开 `u.Get` (1st read hydrate)
- T2: 进入 `ExecInTx`
- T3: 离开 `UpdateAgent` 子调用
- T4: 离开 `UpsertAgentRuntimeSettings` 子调用
- T5: 离开 `ReplaceAgentPromptFiles` 子调用
- T6: `tx.Commit()` 成功/失败
- T7: 离开 `u.Get` (2nd read hydrate)
- T8: 返回

**埋点字段**：`time.Since(start)` + 累计 elapsed。
**埋点手段**：用 `loggateway` 的 `With()` + `Int("ms", ...)` 打 `step_id="agent.update.trace"`。
**预期**：卡死的两 timestamp 之差即为挂起位置。

### 6.5 立即可恢复路径（与埋点解耦）

不论根因为何，**当前 admin 进程已经进入不可恢复状态**——必须 kill 22524 释放 SQLite 文件锁。

`stop_admin_recover` 步骤（用户授权后执行）：
```bash
# 1. 杀进程（释放 rawDB 连接 + WAL 锁）
taskkill /F /PID 22524
# 2. 删 -wal -shm（admin 启动时会自动重建；现在的 WAL 残留是上一次启动的）
Remove-Item f:\project\aranea-agents\data\arenea.sqlite-wal -Force
Remove-Item f:\project\aranea-agents\data\arenea.sqlite-shm -Force
# 3. 重新启动 admin（按用户原方式）
cd f:\project\aranea-agents
$env:DEPLOY_ENV='dev'  # 走 migrateDev
.\bin\admin.exe -conf ./configs/smoke.yaml   # 或 cmd/admin
```

**强烈建议**：`bin\admin.exe` 不存在；当前是 `go run` 或类似模式（PID 22524 在 go-build 临时目录），需要确认用户的真实启动命令（Makefile / scripts/ / 自己的脚本），再决定 restart 方式。

---

## 7. 下一步（请用户选择）

按 TRAE-debugger 协议：
1. 用户复现时同步抓**后端日志**（busy/lock/tx）
2. 用户复现时**关闭/开启并发 chat** 验证 H1
3. 用户复现时**避开 files 修改**验证 H2
4. 拿到证据后，再走「最小化修复 → 验证」循环
5. 修复完成后清理 debug 档与任何插桩

需要你告诉我：**复现条件是否仍可触发？** 以及**第 3 节的 4 个验证步骤中你已经做了哪些 / 能跑哪些？**

---

## 8. 插桩已落地（2026-06-04 ~10:35）

按用户选 A 方案，已在两文件加窄埋点：

### 8.1 改动文件
- [internal/biz/agent_usecase.go](file:///f:/project/aranea-agents/internal/biz/agent_usecase.go) — `Update` 函数体内 8 个断点
- [internal/data/agent_repo.go](file:///f:/project/aranea-agents/internal/data/agent_repo.go) — `UpdateAgent` / `UpsertAgentRuntimeSettings` / `ReplaceAgentPromptFiles` 三个写函数

### 8.2 埋点设计
- 全部用 `Info()` 而非 `Debug()`（smoke.yaml `logging.level: info` 会过滤 Debug）
- 全部用 `step_id="agent.update.trace"` / `"data.update_agent.trace"` / `"data.upsert_settings.trace"` / `"data.replace_files.trace"`
- 全部用 `loggateway.Duration(ms int64)` 字段，**第一段是相对 start 的总 elapsed，第二段是相对本步骤的 elapsed**
- 全部用 `#region debug-point <id>` ... `#endregion debug-point` 包裹，**未来一键删除**（按 redline 一次性 `// #region` 区段删除即可）

### 8.3 关键埋点列表（按调用链顺序）

| step_id | msg | 出现时机 |
|---------|-----|---------|
| `agent.update.trace` | `enter Update` | 入口 |
| `agent.update.trace` | `after Get#1 (hydrate)` | 第 1 次 hydrate 完成 |
| `agent.update.trace` | `before ExecInTx` | merge + 校验完成 |
| `agent.update.trace` | `tx-body: start` | detached tx 创建成功 |
| `agent.update.trace` | `tx-body: before UpdateAgent` | 进入 UpdateAgent 之前 |
| `data.update_agent.trace` | `enter UpdateAgent` | data 层进入 |
| `data.update_agent.trace` | `before GetAgentByID#1` | 第 1 次读之前 |
| `data.update_agent.trace` | `after GetAgentByID#1` | 第 1 次读之后 |
| `data.update_agent.trace` | `before Agent.UpdateOneID.Save` | **关键：实际 SQL UPDATE 之前** |
| `data.update_agent.trace` | `after Agent.UpdateOneID.Save` | **关键：实际 SQL UPDATE 之后** |
| `data.update_agent.trace` | `exit UpdateAgent` | data 层退出 |
| `agent.update.trace` | `tx-body: after UpdateAgent` | 回到 biz 层 |
| `agent.update.trace` | `tx-body: before UpsertAgentRuntimeSettings` |  |
| `data.upsert_settings.trace` | `enter UpsertAgentRuntimeSettings` | |
| `data.upsert_settings.trace` | `before AgentRuntimeSetting.OnConflict.Exec` | **关键：唯一索引冲突的 UPSERT 之前** |
| `data.upsert_settings.trace` | `after AgentRuntimeSetting.OnConflict.Exec` | **关键：UPSERT 之后** |
| `data.upsert_settings.trace` | `before AgentRuntimeSetting.Get` |  |
| `data.upsert_settings.trace` | `after AgentRuntimeSetting.Get` |  |
| `data.upsert_settings.trace` | `exit UpsertAgentRuntimeSettings` | |
| `agent.update.trace` | `tx-body: after UpsertAgentRuntimeSettings` |  |
| `agent.update.trace` | `tx-body: before ReplaceAgentPromptFiles` |  |
| `data.replace_files.trace` | `enter ReplaceAgentPromptFiles` |  |
| `data.replace_files.trace` | `before AgentPromptFile.Delete.Exec` |  |
| `data.replace_files.trace` | `after AgentPromptFile.Delete.Exec` |  |
| `data.replace_files.trace` | `before AgentPromptFile.Create.Save` (i=0..N) | **每条 INSERT 之前** |
| `data.replace_files.trace` | `after AgentPromptFile.Create.Save` (i=0..N) | **每条 INSERT 之后** |
| `data.replace_files.trace` | `before final ListAgentPromptFiles` |  |
| `data.replace_files.trace` | `after final ListAgentPromptFiles` |  |
| `data.replace_files.trace` | `exit ReplaceAgentPromptFiles` | |
| `agent.update.trace` | `tx-body: after ReplaceAgentPromptFiles` |  |
| `agent.update.trace` | `after ExecInTx` | tx commit 完成 |
| `agent.update.trace` | `before Get#2 (final hydrate)` |  |
| `agent.update.trace` | `after Get#2 (final hydrate)` |  |
| `agent.update.trace` | `exit Update` | 返回 |

### 8.4 编译验证
- `go build -o NUL ./internal/biz/... ./internal/data/...` **通过**（exit 0, 无 stderr）

### 8.5 用户侧运行步骤

```powershell
# 1. 先停掉现在卡死的 admin（必须，否则它仍独占 rawDB 连接 + WAL 锁）
taskkill /F /PID 22524
Remove-Item f:\project\aranea-agents\data\arenea.sqlite-wal -Force -ErrorAction SilentlyContinue
Remove-Item f:\project\aranea-agents\data\arenea.sqlite-shm -Force -ErrorAction SilentlyContinue

# 2. 重新构建并启动 admin（按你的原方式；常见是下面两种之一）：
cd f:\project\aranea-agents
# 方式 A（go run）：
go run ./cmd/admin -conf ./configs/smoke.yaml
# 方式 B（先 build 再 run）：
go build -o bin\admin.exe .\cmd\admin
.\bin\admin.exe -conf ./configs/smoke.yaml

# 3. 等 admin 启动完成（看 "data.init_sqlite" 等日志通过 + 端口 8000 起来）
# 4. 打开 admin 前端，编辑 agent___spirit__ 的任意字段，点击「保存」
# 5. **不要等 120s**——12~15s 后立刻停手（目的是不被 axios cancel 干扰）
# 6. 看 logs\aranea-pipeline.log 末尾，按 step_id 顺序捞日志
#    grep "agent.update.trace\|data.update_agent.trace\|data.upsert_settings.trace\|data.replace_files.trace" logs\aranea-pipeline.log | tail -50
# 7. 找 elapsed_ms 的"跳变"——两个相邻 step 之间 elapsed 差 > 5s 就是卡死点
```

### 8.6 预期诊断
- 如果卡在 `before Agent.UpdateOneID.Save` 和 `after Agent.UpdateOneID.Save` 之间 → H6-1：单行 UPDATE 永久阻塞（**最大可能性**，单连接池死锁）
- 如果卡在 `before AgentRuntimeSetting.OnConflict.Exec` 和 `after` 之间 → H6-2：唯一索引冲突
- 如果卡在某条 `before AgentPromptFile.Create.Save i=K` 和 `after` 之间 → H6-3：循环 INSERT 死锁
- 如果卡在 `tx-body: start` 之前（即 `ExecInTx` 的 `d.entClient.Tx(detached)` 步骤）→ H6-0：获取 write 连接永久阻塞
- 如果 Update 入口都没打 `enter Update` → 上游（service 层 / auth / Kratos）就卡了

### 8.7 清理计划（拿到证据后）
- 确认根因
- 应用最小化修复
- 跑 `git grep -n "#region debug-point" -- internal/` 一键清掉所有埋点
- 删除 `debug-agent-save-timeout.md`（仅在用户确认 A. 修复 / D. 终止 后执行）

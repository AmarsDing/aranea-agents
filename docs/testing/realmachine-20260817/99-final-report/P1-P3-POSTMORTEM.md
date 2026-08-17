# Aranea P1-P3 缺陷复盘报告

> **测试基准**：2026-08-17 真机全链路功能测试（20 模块 / ~200 用例 / Docker 部署 / 真实 LLM / GNS3 仿真）
> **上游文档**：[REALMACHINE-REPORT-20260817.md](REALMACHINE-REPORT-20260817.md)、[SOLUTION-REVIEW-20260817.md](SOLUTION-REVIEW-20260817.md)
> **报告范围**：仅覆盖 P1-P3 级缺陷（含已实施与已裁定待实施项）

---

## 一、摘要

### 1.1 缺陷全景

| 优先级 | 数量 | 状态 | 核心主题 |
|--------|------|------|----------|
| **P1** | 2 | 方案已评审裁定，待实施 | Schema 演进与代码同步断裂 |
| **P2** | 4 | 方案已评审裁定，待实施 | 安全语义缺位 + 监控模型粒度缺口 + 错误码压平 + CLI 沉默 |
| **P3** | 4（择要实施） | **全部完成，真机复测通过** | 边界语义区分 + 零值陷阱 + 授权 TTL + 性能缓存 |

### 1.2 修复成果（P3）

- **BUG-MON-B**：持久化授权 TTL 机制落地，510ms → 49~62ms 的 perf 问题同步根治
- **ISSUE-G2/G3**：图执行 API 语义精确化（200 空集 vs 404 区分、step_index=0 合法化）
- **PERF-F1**：providers 接口延迟降幅 **~90%**（2.88MB JSON 重复 IO 消除）

### 1.3 横向共性根因（五类）

1. **Schema 演进与代码同步断裂** —— 表重构/索引改动后，级联删除、唯一约束推断、ON CONFLICT 等消费点未同步
2. **安全语义缺位或压平** —— 高危操作（destructive）在 prompt/校验/错误码三层均未独立表达
3. **性能缓存架构缺陷** —— 缓存存在但"每请求新建实例"导致缓存永不失效，形成"有缓存=无缓存"
4. **边界值/零值处理盲区** —— proto3 标量零值与 REQUIRED 语义冲突、空值被纳入唯一键
5. **错误信息沉默** —— 双缺位（框架不打印 + 应用不打印）导致排查成本倍增

---

## 二、P1 根因分析与预防

### 2.1 BUG-01：无岗位 Agent 创建必败

#### 根因证据链

- **索引设计过宽**：[ent/schema/agent.go:26](file:///f:/myproject/aranea-agents/internal/data/ent/schema/agent.go#L26) `index.Fields("position_key", "agent_variant").Unique()` 为**全量唯一索引**，无部分谓词、不含 `deleted_at`
- **三类受害者**：
  - 模式 A：无岗位 agent（`position_key=''`）全部共享同一个键空间，第 2 个即 23505
  - 模式 B：同 variant（如 `general`）全库仅可存在一个无岗位 agent
  - 模式 C：岗位 agent 软删后，墓碑行永久占槽，同岗同变体无法重建
- **旁证**：全仓已有 3 处规避模式（copy 改 variant、pack 导入注规避、seed 全用非空 position_key），说明索引设计过宽是已知痛点

#### 解决方案（已裁定）

部分唯一索引：`WHERE position_key <> '' AND deleted_at = ''`
- Ent schema 同步 `entsql.IndexWhere` 注解
- DDL 迁移幂等替换（受 `TestMigrationVersionsGloballyUnique` 守卫）

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **唯一索引设计评审清单** | PR Checklist / Code Review | 新建/修改唯一索引时强制回答：①是否覆盖空值默认值？②是否覆盖软删行？③是否有 ON CONFLICT 推断点需同步？ |
| **历史数据安全断言** | 迁移脚本模板 | 建索引前加 `SELECT ... HAVING COUNT(*)>1` 断言，非 0 则阻塞 |
| **Ent auto-migrate 顺序认知** | project_rules.md | 明确 `Schema.Create` 在前、DDL 迁移在后，同名索引只做存在性检查 |

---

### 2.2 BUG-02：会话删除恒 500

#### 根因证据链

- **F1（直接原因）**：[cascade_delete.go:112](file:///f:/myproject/aranea-agents/internal/data/cascade_delete.go#L112) 硬删 `messages` 表，但该表已被迁移 `20260902_drop_messages_subsystem` 删除 → `42P01` → 事务回滚 → 单删/批删 100% 失败
- **F2（隐性泄漏）**：级联清单停留在 v1 时代，11 张活跃 session 域表（`steps_v2`、`trpc_session_events`、`event_delivery_outbox` 等）永不清理
- **F3（设计冲突）**：`RestoreSession` 语义为"恢复会话壳"，但用户感知为"恢复历史"——删除已硬删历史导致恢复后为空壳
- **F4（旁证）**：`activities` 等死表被旧二进制 Ent auto-migrate 静默复活

#### 解决方案（已裁定）

1. 删 `messages` 引用（F1）
2. 补删 11 张活跃表（F2，审计/计量表保留不进级联）
3. 新增 PG 集成回归测试（建会话→写 v2 全链路→删除→逐表 COUNT=0 断言 + information_schema 正向校验）
4. 新建迁移 drop `activities`/`event_store`/`sessions_v2` 死表

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **表变更三重校验** | project_rules.md [DB-N12] | drop/rename/改约束后必须全局搜索：① `cascade_delete.go` ② `ON CONFLICT` 推断点 ③ raw SQL 引用点 |
| **PG 集成回归纳入 CI** | GitHub Actions / Makefile | `ARANEA_TEST_PG_DSN` 环境下跑级联删除集成测试 |
| **禁止旧二进制连接现网库** | 部署 SOP | Ent auto-migrate 会复活退役表——回滚必须走蓝绿/数据迁移，禁止旧镜像直连 |
| **RestoreSession 语义明示** | API 文档 + 代码注释 | 明确标注"恢复仅恢复会话壳，历史数据已硬删" |

---

## 三、P2 根因分析与预防

### 3.1 BUG-MON-A：破坏性操作的澄清被自动代答为「取消」

#### 根因证据链

- **事故链路**：用户请求"注入故障" → 意图识别产出澄清问题（推荐答案="Cancel the injection"）且 **risk_flags 无 destructive** → `autoResolveClarification` 按推荐自动作答 → 模型收到"用户已取消" → HITL 门禁未到达即被静默否决
- **双重根因**：
  1. 意图 prompt 示例仅列 `touches_auth/migrations`，`destructive`/`irreversible` 两个高危标记**从未在 prompt 中出现**
  2. `autoResolve` 无"推荐答案=取消语义"的否决守卫——假设式前进退化为代用户否决自己

#### 解决方案（已裁定）

三层防御（纵深互补）：
- **L1 prompt 补全**：示例追加 `destructive, irreversible`，并追加规则说明
- **L2 确定性强制打标**：对用户原文做破坏性模式匹配（注入故障/删除/清空/重置等窄表），命中则补 `destructive`
- **L3 推荐否决守卫**：推荐答案命中取消语义模式（cancel/取消/放弃/否/stop/abort）时，放弃 autoResolve、走挂起弹卡

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **高危操作语义检查清单** | agent/intent 代码审查 | 新增/修改 intent 时检查：① prompt 是否覆盖 destructive/irreversible 示例？② 是否有确定性兜底词表？③ autoResolve 是否对所有推荐类型做守卫？ |
| **假设式前进边界规则** | project_rules.md | 推荐答案含取消/否定语义时，禁止自动代答（fail-closed） |
| **HITL 端到端回归** | 10-monitor-scenario | 每次部署后跑 fault_inject 全链路，验证 HITL 确认真实到达 |

---

### 3.2 BUG-MON-C：端口级故障不产生告警

#### 根因证据链

- **监控模型粒度缺口**：linemonitor 线路探测 = 对设备管理面地址做 ICMP ping；数据面端口 down 不影响管理面可达 → 零探测失败 → 零告警
- **已有数据被掩盖**：SNMP `ifOperStatus` 已采集，但仅用于"选第一个 up 接口测带宽"——端口 down 后自动换接口，恰好掩盖故障
- **不是采集缺口，是告警模型缺口**：采集在、展示在，唯独无"设备+端口"级告警源

#### 解决方案（已裁定：(b) 拓扑自动生成）

以 linemonitor LLDP/CDP 嗅探拓扑为数据源，自动生成钉端口 SNMP 探测线路：
1. `discovered_links` 加 `local_port`/`peer_port` 持久化
2. `PortProbeGenerator`：每轮拓扑重建后扫描 inter_switch 链路 → ifName→ifIndex 解析 → upsert `auto:` 前缀线路 + SNMP 探测配置
3. 生命周期：连续 3 轮未发现则 disable（不删除）
4. 上限护栏：500 条自动线路防爆量

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **监控覆盖度矩阵** | 演练 SOP | 每次演练前核对：管理面可达？数据面连通？端口级 ifOperStatus？三层逐级确认 |
| **拓扑发现前置检查** | 部署清单 | 被监控设备 SNMP 可达 + CDP/LLDP 开启 + 网段发现已配置 |
| **自动线路命名空间隔离** | linemonitor 代码 | `auto:` 前缀与手工线路隔离，避免唯一约束冲突 |

---

### 3.3 BUG-G1：残缺图 visualize 500

#### 根因证据链

- builder 层已有三类残缺图守卫（空 nodes / 缺 entry / func_ref 空），均返回 `apierror.BadRequest`
- **[runtime_adapter.go:696](file:///f:/myproject/aranea-agents/internal/graph/adapter/runtime_adapter.go#L696) 压平点**：`Visualize` 把**一切** Build 错误重写为 `apierror.Internal(...)` → 400 被吞成 500

#### 解决方案（已裁定）

`apierror.Wrap` 透传：已是 `*apierror.Error` 则原样返回，非 apierror 才按 Internal 包装。

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **错误码压平审查** | Code Review 清单 | service/adapter 层调用 builder/usecase 后，检查是否用 `apierror.Internal`  blanket 包装——优先 `apierror.Wrap` 透传 |
| **Visualize 单测覆盖** | internal/graph/adapter | 构造 func_ref 空图 → 断言 code=bad_request |

---

### 3.4 BUG-CLI-01：非法命令静默退出

#### 根因证据链

- `SilenceErrors: true`（cobra 不打印）+ `main()` 退出前不打印 `err` → exit 3 零输出
- 排查确认无"先自行打印再 return"的双打路径

#### 解决方案（已裁定）

`main()` 退出前 `fmt.Fprintln(os.Stderr, "aranea:", err.Error())`

#### 预防机制

| 机制 | 落地位置 | 检查项 |
|------|----------|--------|
| **CLI 错误输出强制检查** | Code Review / e2e 测试 | 非法命令用例必须断言：stderr 非空且含错误消息；exit code 非 0 |
| **SilenceErrors 使用审批** | project_rules.md | 新增 `SilenceErrors:true` 需说明"谁负责打印"，禁止双缺位 |

---

## 四、P3 根因分析与验证结果

### 4.1 ISSUE-G2：无检查点执行返回 404

| 维度 | 内容 |
|------|------|
| **根因** | `LineageID==""` 时返回 `ErrNotFound`，与"执行不存在"共用 404，语义不可分 |
| **方案** | biz 层 `ListCheckpoints`/`TimeTravelGetState` 先取执行（不存在才 404），存在但无 lineage 返回 200 空集 |
| **文件** | [graph_execution_usecase.go](file:///f:/myproject/aranea-agents/internal/biz/graph_execution_usecase.go#L881-L896) |
| **验证** | GRAPH-09/10 PASS：`{"items":[]}` 200；前端 `res.items ?? []` 天然兼容 |
| **预防** | 新增/修改"存在性+数据"双语义 API 时，评审是否需区分"资源不存在"与"资源存在但子集为空" |

### 4.2 ISSUE-G3：time-travel step_index=0 被拒

| 维度 | 内容 |
|------|------|
| **根因** | `int32 step_index = 2 [(google.api.field_behavior) = REQUIRED]` —— proto3 标量零值与未设置不可区分，protovalidate 拒 0 |
| **方案** | 去 REQUIRED，service 层拆分校验：负数 400 BadRequest / 超界 404 NotFound |
| **文件** | [graph.proto](file:///f:/myproject/aranea-agents/api/kratos/graph/v1/graph.proto#L267-L274)、[graph_execution_service.go](file:///f:/myproject/aranea-agents/internal/service/graph_execution_service.go#L266-L283) |
| **验证** | GRAPH-13 PASS：`step_index=0` 返回 200 + 正确节点状态 |
| **预防** | proto3 标量字段禁止 REQUIRED；零值为合法业务值的字段必须在 biz/service 层显式校验 |

### 4.3 BUG-MON-B：持久化授权无 TTL

| 维度 | 内容 |
|------|------|
| **根因** | schema 缺 `expires_at`，`tool_grants` 表写入即永存；对比 session 级授权已有 24h TTL |
| **方案** | ① schema 加 `expires_at` ② 默认 TTL=72h（已裁定）③ `HasToolGrant` 读径过滤过期行 ④ `ListToolGrants` 惰性清理 ⑤ 迁移 20261227 回填存量 |
| **文件** | [tool_grant.go(schema)](file:///f:/myproject/aranea-agents/internal/data/ent/schema/tool_grant.go)、[tool_grant.go(biz)](file:///f:/myproject/aranea-agents/internal/biz/tool/tool_grant.go)、[tool_grant.go(data)](file:///f:/myproject/aranea-agents/internal/data/tool_grant.go)、[ddl_migration_registry.go](file:///f:/myproject/aranea-agents/internal/data/ddl_migration_registry.go) |
| **验证** | MON-B-01 PASS：4 条存量按 created_at+72h 回填，至 2026-08-17 全过期；MON-B-02 PASS：API 返回 `items=[]`（过期行已过滤） |
| **预防** | ① 任何"持久化授权/许可/白名单"schema 必须含 expires_at 或等效时界字段 ② 安全类写操作后补 SOP 清理条目 |

### 4.4 PERF-F1：/v1/model-catalog/providers ~510ms

| 维度 | 内容 |
|------|------|
| **根因** | ① 2.88MB JSON 每请求 `os.ReadFile` + `json.Unmarshal` 全量反序列化 ② **`ModelRegistryStoreProvider.Store()` 每次调用新建 Store 实例** → mtime+size 缓存随实例销毁而失效 |
| **方案** | ① `Store` 增加 `mtime+size` 目录缓存 ② `ModelRegistryStoreProvider` 按 root 缓存 Store 实例（RWMutex），命中时跳过 root resolver（零 DB 查询） |
| **文件** | [store.go](file:///f:/myproject/aranea-agents/internal/modelregistry/store.go)、[model_registry.go](file:///f:/myproject/aranea-agents/internal/biz/model_registry.go#L37-L57) |
| **验证** | 5 次采样：128ms(cold) → 49ms, 54ms, 52ms, 49ms(warm)；均值 **~49-62ms**，降幅 **~90%** |
| **预防** | ① 缓存设计必须审查"实例生命周期 vs 缓存生命周期"——禁止每请求新建带缓存的实例 ② 性能回归基线：只读接口 p95 <100ms，超基线 5x 即触发 pprof 定位 |

---

## 五、预防机制总表（按责任环节）

### 5.1 Schema / DB 变更环节

| 规则 | 来源缺陷 | 落地位置 | 执行人 |
|------|----------|----------|--------|
| 表变更三重校验（cascade_delete.go / ON CONFLICT / raw SQL） | BUG-01/02 | project_rules.md [DB-N12] | 改表开发者 |
| 唯一索引设计评审清单（空值/软删/ON CONFLICT） | BUG-01 | PR Checklist | Reviewer |
| 任何持久化授权必须含 expires_at | BUG-MON-B | Schema Review | Reviewer |
| 迁移前历史数据安全断言（重复检查） | BUG-01 | 迁移脚本模板 | 开发者 |
| 禁止旧二进制连接现网库 | BUG-02 F4 | 部署 SOP | 运维 |

### 5.2 API / 协议设计环节

| 规则 | 来源缺陷 | 落地位置 | 执行人 |
|------|----------|----------|--------|
| proto3 标量字段禁止 REQUIRED | ISSUE-G3 | API Design Guide | 设计者 |
| 零值为合法业务值时必须在 biz/service 层显式校验 | ISSUE-G3 | API Design Guide | 设计者 |
| "资源不存在"与"子集为空"必须可分 | ISSUE-G2 | API Design Guide | 设计者 |
| 错误码禁止 blanket Internal 包装——优先 Wrap 透传 | BUG-G1 | Code Review 清单 | Reviewer |

### 5.3 安全 / 意图环节

| 规则 | 来源缺陷 | 落地位置 | 执行人 |
|------|----------|----------|--------|
| 高危操作语义检查清单（prompt 示例/兜底词表/autoResolve 守卫） | BUG-MON-A | agent/intent Code Review | Reviewer |
| 假设式前进边界：推荐含取消语义时禁止自动代答 | BUG-MON-A | project_rules.md | 开发者 |
| HITL 端到端回归（每次部署后跑 fault_inject 全链路） | BUG-MON-A | 部署 Checklist | QA |
| 监控覆盖度矩阵（管理面/数据面/端口级三层核对） | BUG-MON-C | 演练 SOP | 演练负责人 |

### 5.4 性能 / 缓存环节

| 规则 | 来源缺陷 | 落地位置 | 执行人 |
|------|----------|----------|--------|
| 缓存设计审查"实例生命周期 vs 缓存生命周期" | PERF-F1 | Code Review 清单 | Reviewer |
| 只读接口 p95 >100ms 或超基线 5x 触发 pprof 定位 | PERF-F1 | 性能回归基线 | QA |

### 5.5 CLI / 错误输出环节

| 规则 | 来源缺陷 | 落地位置 | 执行人 |
|------|----------|----------|--------|
| CLI 非法命令用例必须断言 stderr 非空 | BUG-CLI-01 | e2e 测试 | QA |
| SilenceErrors 使用需说明"谁负责打印" | BUG-CLI-01 | project_rules.md | 开发者 |

---

## 六、修复与复测状态（2026-08-17 更新）

| 批次 | 事项 | 代码状态 | 真机复测 |
|------|------|----------|----------|
| **批次 1（P1）** | BUG-01 部分唯一索引 + Ent 同步 + 迁移 20261225 | ✅ 已完成 | ✅ AGT-03 PASS：连续 2 个无岗位 agent 创建 200（原 23505）；DB 索引确认 `WHERE position_key<>'' AND deleted_at=''` |
| **批次 1（P1）** | BUG-02 cascade 修复 F1/F2 + 11 表清理 + PG 回归 | ✅ 已完成 | ✅ CHAT-11 PASS：删除 200；steps_v2/turns_v2/trpc_session_events/tasks_v2/member_sessions_v2 全 0 残留；残留会话 972daa64 已清理 |
| **批次 2（P2）** | BUG-MON-A 三层防御（L1 prompt/L2 强制打标/L3 推荐否决） | ✅ 已完成 | ✅ MON-A-RT PASS：模糊注入请求 → clarify step `awaiting_input` 挂起 + 0 工具调用（原自动代答取消） |
| **批次 2（P2）** | BUG-MON-C 拓扑自动生成（TwinMonitor：端口持久化 + PortProbeGenerator + 生命周期 + 500 护栏） | ✅ 已完成（含 6 组单测） | ⏸️ **挂起（代码就绪）**：linemonitor 镜像已重建运行（16:24），但测试环境缺前置数据（`lan_segments`/`discovered_links` 均为空、无 SNMP 绑定），无法触发拓扑扫描。待环境具备 LLDP/CDP 嗅探数据后复测端口级告警闭环 |
| **批次 2（P2）** | BUG-G1 apierror.Wrap 透传 | ✅ 已完成 | ✅ GRAPH-04B PASS：正常图 visualize 200 无回归 |
| **批次 2（P2）** | BUG-CLI-01 退出前打印 stderr | ✅ 已完成 | ✅ CLI-08 PASS：foobar → exit=3 + `aranea: unknown command "foobar"`（原零输出） |
| **批次 3（P3）** | BUG-MON-B / G2 / G3 / PERF-F1 | ✅ 已完成 | ✅ 全部 PASS（详见 §四） |
| **观察项** | PERF-S1 Spirit 24k token/轮 | 待取证 | llm_relay.py 抓包分解 system prompt 构成 |
| **观察项** | ISSUE-K1 vault 宿主路径容器不可达 | 配置修正 | 改容器卷映射或停用同步 |
| **观察项** | 2 个阿里云 MCP server 配置失效 | 配置修正 | 清理或改容器内可达配置 |

---

## 七、结论

P1-P3 缺陷的**共性根因**不是"某个函数写错了"，而是**工程流程中的检查点缺失**：

- Schema 变更后缺少"消费方全量同步"的强制检查点 → BUG-01/02/MON-B
- 安全语义（destructive）在 prompt/校验/错误码三层缺少独立表达通道 → MON-A/G1
- 缓存设计缺少"实例生命周期 vs 缓存生命周期"的审查 → PERF-F1
- 边界值（零值/空值）在协议层被 proto3 REQUIRED 错误表达 → G3

**P3 的成功验证了一个方法论**：先通过真机测试暴露问题 → 深度根因分析 → 方案评审（含副作用与有效性） → 用户裁定 → 小步实施 + 立即复测。该方法论应延续到 P1/P2 的实施中。

**最关键的防线前移**：将上述预防机制从"事后复盘"转化为"PR Checklist + CI 断言 + Code Review 强制项"，使同类缺陷在合并前即被拦截。

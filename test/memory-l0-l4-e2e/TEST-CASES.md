# L0-L4 五层记忆系统 — 全功能测试用例与结果记录

> 日期：2026-07-29
> 环境：admin @ http://localhost:8000（KRATOS_HTTP_AUTH_DISABLED=1 + DEPLOY_ENV=dev bypass）、PostgreSQL、前端 dev @ http://localhost:9003
> 测试 Agent：`agent___spirit__`（基线：L3 facts=4、L4 entities=1）
> 基线数据：活跃 session `cbd769a3-a28b-4b3f-b157-a78c12c034ea`（87 次工具调用）；空闲 session `6b56174d-e488-4335-9c6c-4d5e8341aa26`
> 范围依据：`docs/development/memory/memory.md`、`实施进度.md`、`api/kratos/memory/v1/memory.proto` 全部 35 个 RPC

## 测试约定

- 每项用例记录：步骤 / 预期 / 实际 / 结论（✅通过 / ❌失败 / ⚠️异常）/ 证据
- 聊天类用例使用 `POST /v1/chat/messages`（同步），异步 worker 效果等待后轮询验证
- 所有写操作使用独立测试标记 `memtest-*`，便于清理与识别

---

## 一、L0 瞬时记忆（上下文窗口）

| 编号 | 用例 | 验证点 |
|------|------|--------|
| L0-01 | GET `/v1/sessions/{sid}/l0/snapshots`（历史富会话） | 返回快照列表；字段含 id/context_usage/sections/warning_codes/created_at |
| L0-02 | 同上 + `agent_id` 过滤 | Team 场景下按 agent 隔离（spirit 单 agent 场景验证参数兼容） |
| L0-03 | 快照内容结构 | sections 覆盖 system/history 等分段；token 统计与 usage 一致 |
| L0-04 | 新对话触发新快照 | 发送聊天后轮询快照列表，出现新记录（Runner 集成 `l0_snapshot_persist.go`） |
| L0-05 | warning_codes 推导 | 正常会话应为空或 normal；检查 `ShouldWriteL0AssemblySnapshot` 逻辑产物 |
| L0-06 | POST `/v1/sessions:compact` 手动压缩 | 返回成功；压缩后 L0 快照中出现 summary 分段（`internal/session/compressor.go`） |

## 二、L1 工作记忆

| 编号 | 用例 | 验证点 |
|------|------|--------|
| L1-01 | GET `/v1/sessions/{sid}/l1/tasks` | 返回任务列表（可能为空）；字段含 task_id/title/status/token_budget/updated_at |
| L1-02 | GET `/v1/sessions/{sid}/l1/tasks/{task_id}/fields` | 字段树：key/value/source/revision/pin_to_prompt/visibility |
| L1-03 | 聊天指令触发 working_memory 工具 | 消息要求 agent"用工作记忆记录当前任务目标"，轮询 L1 tasks 出现新任务/字段（`internal/tools/working_memory/tools.go` 5 工具） |
| L1-04 | L1 prompt 注入 | 字段 pin 后下一轮对话 L0 快照/日志出现 L1 cue（`internal/agent/l1_prompt.go`） |

## 三、L2 情景记忆

| 编号 | 用例 | 验证点 |
|------|------|--------|
| L2-01 | GET `/v1/memory/episodes?agent_id=...` 分页 | total 正确；items 含 title/kind/outcome/importance/consolidation_status/created_at |
| L2-02 | 对话产生新 episode | 发送有实质任务内容的聊天 → 轮询 episodes 新增（EndL1Task 归档 hook 或 AutoMemoryWorker） |
| L2-03 | consolidation 状态机 | 新 episode 初始 pending/consolidated；nil LLM 时降级 heuristic（日志 `episode consolidation skipped: nil LLM`） |
| L2-04 | POST `/v1/memory/recall/debug` L2 recall | keyword 命中已存 episode；返回 raw_score/final_score（多策略融合 keyword+vector+importance+session boost） |
| L2-05 | POST `/v1/memory/search/composite` | L2+L3 统一排序去重返回 |

## 四、L3 语义知识

| 编号 | 用例 | 验证点 |
|------|------|--------|
| L3-01 | GET `/v1/memory/l3/facts?agent_id=...` | 基线 4 条；字段含 statement/kind/scope/confidence/importance/hit_count/index_status |
| L3-02 | POST `/v1/memory/l3/facts` 新增 | 返回 fact_id；List 可见；来源 source=manual |
| L3-03 | 重复 upsert 幂等 | ON CONFLICT 不报错，版本/confidence 更新（`store_writes.go` 唯一写路径） |
| L3-04 | PII 检测 | 写入含手机号 statement → PII 标记（9 检测器，block/redact/review 策略） |
| L3-05 | GET `/v1/memory/l3/facts/pii` + POST review | 标记列表出现；approve/reject 后状态流转 |
| L3-06 | 冲突检测 | 写入与现存矛盾 fact（如"本项目使用 Tailwind" vs "本项目禁止 Tailwind"）→ GET `/v1/memory/l3/facts/conflicts` 出现冲突对 |
| L3-07 | recall debug L3 | 相关 query 命中 facts，final_score 五维（keyword0.25/vector0.30/importance0.20/recency0.15/quality0.10） |
| L3-08 | index_status | embedding 未配置时新 fact index_status 进入 stale/failed；reconciler 行为符合预期 |

## 五、L4 图谱与进化

| 编号 | 用例 | 验证点 |
|------|------|--------|
| L4-01 | GET `/v1/memory/l4/entities?agent_id=...` | 基线 1 条（用户画像）；字段含 name/type/aliases/importance/confidence |
| L4-02 | GET `/v1/memory/l4/entities/{center_id}/neighborhood` | BFS 邻域；max_hops 生效 |
| L4-03 | GET `/v1/memory/l4/spreading-activation` | 返回逐跳激活节点与强度 |
| L4-04 | 聊天中文实体抽取 | 发送"我叫测试用户张三，我喜欢喝咖啡"→ entities 新增 person 实体（中文 regex：我叫/我喜欢），Name Conflict gate 不阻塞不同名 |
| L4-05 | GET `/v1/agents/{aid}/identity` / `/strategy` | 返回身份/策略档案（默认值或已配置值） |
| L4-06 | GET `/v1/agents/{aid}/evolution/proposals` / `/events` / `/metrics` | 列表返回（可为空）；metrics 含 tool success rate / retrieval quality |
| L4-07 | POST `/v1/agents/{aid}/evolution/events` 追加事件 | 事件出现在 ListEvolutionEvents；回滚生成 rollback event 而非删除 |
| L4-08 | Cascade：GET `/v1/memory/cascade/proposals`、preview、saga-steps、approve/reject/retry/compensate | 列表返回；无 proposal 时 approve 返回 NotFound 而非 500 |

## 六、跨层融合与治理

| 编号 | 用例 | 验证点 |
|------|------|--------|
| X-01 | GET `/v1/memory/layer-overview` | 五层卡片统计正确（L0 快照数/L1 任务/L2 episodes/L3 facts/L4 entities+relations）、action_items、activity_feed 倒序 |
| X-02 | GET `/v1/memory/graph/unified` | 节点跨层（L4 实体+L3 事实+L2 情景）；边分类 entity-entity/fact_link/fact_source；hops=2 截断；min_weight 过滤；空图 empty_reason |
| X-03 | GET `/v1/memory/worker/status` | AutoMemoryWorker 状态、队列长度、done/dead 计数 |
| X-04 | GET `/v1/memory/worker/dead-letters` + replay + abandon | 列表返回；无死信时 replay/abandon 返回 NotFound |
| X-05 | GET/PUT `/v1/memory/platform/settings` | 读返回当前平台设置；PUT 更新后 GET 一致（L2/L3/L4 阈值等） |

## 七、前端 UI 验证（浏览器 http://localhost:9003）

| 编号 | 用例 | 验证点 |
|------|------|--------|
| U-01 | 记忆中心「全景」Tab | 五层卡真实数据、健康徽标、今日新增、层间箭头、需要关注区、最近动态流 |
| U-02 | 「图谱」Tab | 默认不毛线球（≤40 节点）、层级色码统一、INHIBIT 红边样式、节点详情抽屉、跳转记忆浏览 |
| U-03 | 「浏览」Tab | L3 facts 表格 + L2 情景时间线、层级 chips 过滤、分页 |
| U-04 | 「治理」Tab | cascade 审批、演进提案、平台设置、Worker 状态入口齐全 |
| U-05 | Session 详情记忆展示 | L0 快照面板（MemorySnapshotDrawer）分段展开、L1 任务面板 |
| U-06 | 层级卡钻取跳转 | 点击 L3 卡 → browse Tab 并过滤 L3 |

---

## 结果记录（执行后填写）

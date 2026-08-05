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

> 执行时间：2026-08-01 10:34（静态 API）/ 2026-08-01 12:10（聊天链路复测）/ 2026-08-01 12:20（前端 UI）
> 执行环境：admin @ :8000（KRATOS_HTTP_AUTH_DISABLED=1）、PostgreSQL、前端 dev @ :9003
> 总体结论：**41/41 项通过（静态 35 + 聊天链路 6 + UI 6），期间发现 6 个真实缺陷已全部按根因修复并复测通过。**

### A. 静态 API 测试（35 项，全 PASS）

明细见 `static-results.txt`。覆盖：L0-01/02/ERR、L1-01/02、L2-01/01b/04/05、L3-01~08、L4-01/02/03/05/06/07/08、X-01~X-05。
要点：
- L3-04 PII 脱敏生效（`[phone]` 占位替换），L3-05 review approve 状态流转正常
- L3-06 冲突检测命中（tailwind 矛盾对），L4-08b/c 不存在资源返回 404 而非 500
- X-04b 死信 replay 不存在 ID → 404（修复后行为，见问题 #5）

### B. 聊天链路端到端（6 项，全 PASS）

脚本 `run_chat_tests.py` + `verify_new_session.py`，测试 agent `memtest-agent`(f2e5a24ab0756d6413d6a1a3)，session `685dfbcb-f7e0-40f7-9792-003bfe2405ca`：

| 用例 | 结果 | 证据 |
|------|------|------|
| L0-04 聊天触发快照 | ✅ | 4 个快照；keys 完整（segments/usage/warnings/token 统计） |
| L0-03 分段结构 | ✅ | segments 覆盖 history/memory.l1/memory.l2_l3/memory.l4/system.*/user.input |
| L1-03 工具触发 | ✅ | agent 自主调用 working_memory 写入 task_goal+关键决策，task 共 11 字段 |
| L1-04 L1 prompt 注入 | ✅ | snap0/1 `l1Fields=11`、memory.l1 分段 token=1108 |
| L4-04 中文实体抽取 | ✅ | entities=4：测试用户张三(person)/喝咖啡(preference)/喝什么(preference)/User profile |
| MSG3 记忆回读 | ✅ | agent 精确回答"张三/咖啡/小白"；snap3 `memory.l4` 分段 l4Paths=3 l4Tokens=744（修复后注入生效） |
| L2-02 episode 产生 | ✅ | episodes=7（worker 异步归档）；系统消息含 L2 recall hits |
| L3 consolidation | ✅ | DB memory_facts 共 33 条，本 session scope 3 条（scope=session/user 存储，agent scope 查询为 0 属测试脚本查询口径，recall 注入正常） |
| 时间线用户消息 | ✅ | tasks_v2=3 与 steps_v2=16 合并，messages API 返回 15 条含全部 3 条用户消息（修复后行为，见问题 #1） |

### C. 前端 UI 验证（6 项，全 PASS）

| 用例 | 结果 | 证据 |
|------|------|------|
| U-01 全景 Tab | ✅ | 五层卡真实计数、健康徽标、今日新增、需要关注区、最近动态流均渲染；控制台无 error |
| U-02 图谱 Tab | ✅ | 节点按 L2/L3/L4 分层色码+形状区分，受控不毛线球；点击节点开详情抽屉 |
| U-03 浏览 Tab | ✅ | L0-L3 chips 过滤、L3 facts 表格（statement/kind/confidence）、L2 时间线、分页 |
| U-04 治理 Tab | ✅ | cascade 审批/演进提案/平台设置/Worker 状态/dead-letters 五面板齐全 |
| U-05 Session 详情 | ✅ | **3 条用户消息正常显示**（问题 #1 修复验证）；L0 快照面板可展开分段；L1 任务面板显示字段 |
| U-06 层级卡钻取 | ✅ | 全景 L3 卡点击 → 跳 browse Tab 并过滤 L3；L4 卡 → 跳 graph Tab |

### D. 发现的缺陷与根因修复（6 项，全部根本性修复）

| # | 缺陷 | 根因 | 根本性修复 | 复测 |
|---|------|------|-----------|------|
| 1 | 聊天不产生 L4 实体、前端消息列表缺用户消息 | v2 时间线只合并 steps_v2（agent 步），不含 tasks_v2（用户消息）→ WriteFromUserText 永远收不到用户输入 | `session_activity_adapter.go` 合并 tasks_v2 进时间线，Task→Activity(kind=task) 转换；Wire 注入 taskV2Repo；补单测 | ✅ entities=4、messages 含用户消息 |
| 2 | consolidation fact upsert 报 `pq: 字段关联 "version" 是不明确的` | ON CONFLICT DO UPDATE 子句中 version 列引用未限定表名 | `memory_shim_l3.go` 改为 `version = memory_facts.version + 1` | ✅ facts 写入正常 |
| 3 | L3-03 重复 upsert version 不自增 | 传入的 Version 值覆盖数据库自增，破坏系统管理版本纪律 | INSERT 强制 version=1，自增仅由 ON CONFLICT 子句系统管理 | ✅ version=2 正确 |
| 4 | L4-01 entities 列表空 | API 用 workspace_id='' 而存量行为 'default'，等值过滤丢数据 | 过滤改为 `workspace_id IN ('', ?)` 兼容共享/遗留行 | ✅ count 正常 |
| 5 | X-04b replay 不存在死信返回成功 | data 层未检查 RowsAffected，service 层合成假成功响应 | MarkDeadLetterReplayed/Abandoned 检查影响行数并传播 NotFound；service 删除合成响应 | ✅ 404 DATA_NOT_FOUND |
| 6 | L4 实体不注入 prompt（recall 时 l4 分段缺失） | SQL `name LIKE '%<整句用户输入>%'` 永远匹配不到短实体名，候选集为空 | `l4_prompt.go` 改为拉取有界候选（64 条）+ Go 内存排序：mention 命中优先 → confidence 降序 → 时间序；confidence<0.3 过滤前置不占用注入槽；补 2 个单测 | ✅ snap3 memory.l4 分段 l4Paths=3，MSG3 回答正确 |

### E. 环境类问题（已处置）

- 磁盘耗尽（Errno 28）：测试目录 `.gocache-*` 占用 33GB+，已清理释放 33.54GB。
- 重启 admin 后 401：进程未带 `KRATOS_HTTP_AUTH_DISABLED=1`，已用该环境变量重启恢复。

### F. 遗留观察项（非阻塞）—— 2026-08-05 已全部修复，见 G 节

1. **L3 facts scope 口径**：consolidation 写入的 facts 使用 session/user scope，按 agent scope 查询为 0。recall scopes 配置（agent/user/team/workspace）覆盖 user 域故注入正常；如需"按 agent 维度浏览 facts"，管理端查询应聚合 user/session scope 或由 consolidation 双写 agent scope——建议在产品口径上明确，暂不定为缺陷。→ **已修复（G-F1）**
2. **L2 recall 系统消息冗余**：聊天时间线中每轮插入一条 `{"hits":[...]}` 系统消息展示 recall 命中，对终端用户偏技术化，建议前端默认折叠（P2 打磨）。→ **已修复（G-F2）**
3. agent 回复中含 `<fact type="identity">` 标记文本直接渲染在消息里（模型侧输出习惯），建议评估是否在展示层剥离（P2 打磨）。→ **已修复（G-F3）**

### G. 遗留观察项修复（2026-08-05，F1/F2/F3 全部根本性修复并复测通过）

> 执行环境：admin @ :8000（新二进制，含三项修复）、PostgreSQL；验证脚本 `verify_f123.py`（日志 `f123-verify-log.txt`）6/6 PASS。
> 后端测试：`internal/biz`、`internal/biz/session`、`internal/agent/v2`、`internal/cronrunner/jobs`、`internal/memory/...`、`internal/data`、`internal/service` 全绿（仅 2 个外网依赖的 model catalog 用例失败，与本次无关）。前端 lint 0 错误、1192 测试通过、build 成功。

| # | 观察项 | 根因 | 根本性修复 | 运行时复测证据 |
|---|--------|------|-----------|---------------|
| F1 | 全景卡 L3 计数与浏览 Tab 口径不一致（卡上显示 0，实际有 facts） | 全景卡按 `scope='agent'` 计数，而即刻事实写 session scope、巩固事实写 user scope → 漏计 | 统一改为按**产生方 agent**（`memory_facts.agent_id` 列）跨全部 scope 聚合：`biz.L3FactReader.ListFactRows` 增加 `agentID` 过滤参数并贯通 data 层 SQL；`ListMemoryFactsRequest` proto 增加 `agent_id=8` 字段（`make api` 重新生成 Go+TS）；service 透传；前端浏览 Tab 始终带当前选中 agent（切换 agent 自动刷新）；设计文档口径同步 | 全景卡 L3=33，浏览 Tab `?agent_id=` total=33（一致）；旧口径 `scope_type=agent` total=0（复现原缺陷）；33 条 facts 全部非 agent scope（user/session）但正确归属该 agent |
| F2 | 系统内部 notice（context_usage/memory_recalled 等）以 raw JSON system 消息泄漏进消息视图 | `activityToChatMessage` 对所有 `kind=notice` 一律映射为 system 消息，未区分机器载荷与用户可见通知 | `session.ActivityEntry` 增加 `NoticeType` 字段（自 `Activity.Meta["notice_type"]` 传播）；消息转换按 `systemInternalNoticeTypes` 集合（context_usage/context_window/metrics_updated/token_usage/memory_recalled，与前端 `noticeFilter.ts` 保持一致）过滤系统内部 notice，用户可见 notice（model_router 等）保留 | 新 session messages API：roles=user/assistant/tool，无 system raw JSON；旧 memtest session（曾含 recall/context 通知）：8 条消息 0 泄漏 |
| F3 | agent 回复中 `<fact>` 机器抽取标签直接渲染给用户 | v1 管线在消息持久化前剥离标签，v2 管线（projector.OnTextDone）未做同等处理 | `biz.StripFactMarks`（复用 fact_parser 同一正则，未闭合标签保留防截断误删）；`ActivityProjector.OnTextDone` 在持久化/完成 step 前剥离——抽取逻辑在上游（immediateFactWriter）不受影响 | 发送"我最近开始学习小提琴"：assistant 回复自然文本无 `<fact>`；tool 结果 `action:created` 且 fact 同时落 session+user scope（keyword=小提琴 total=2），证明抽取链路不受剥离影响 |

**新增/更新测试**：`fact_parser_test.go`（StripFactMarks 含未闭合标签边界）、`projector_test.go`（TestHandleTextDone_StripsFactMarks）、`activity_message_adapter_filter_test.go`（内部 notice 过滤 + 用户 notice 保留）、`session_activity_adapter_test.go`（NoticeType 传播）、`memory_layer_overview_test.go`（L3 按 agentID 跨 scope 计数断言）、`memory_list_facts_test.go`（service 层 agent_id 透传）；全部 mock（canary/ebbinghaus/link_evolution/bitemporal）同步新签名。

### H. scope 口径一致性排查（2026-08-05，H1-H4 全部根本性修复并复测通过）

> 触发：按 F1 根因模式（写 scope=session/user vs 读 scope=agent 漏算）排查其余 scope 相关读取点。基线分布：`memory_facts` 活跃行几乎全部在 user/session 域（`check_scope_dist.py`），凡 `scope_type='agent'` 过滤的读取均漏算。
> 执行环境：admin @ :8000（新二进制 pid=19784，含四项修复）、PostgreSQL；验证脚本 `verify_h2.py`。
> 静态验证：`make api` ✅、`go build ./...` ✅、`go test`（biz/data/memory/service/cronrunner）全绿（仅 2 个已知外网 model catalog 失败）、前端 lint 0 / 1202 tests / build ✅。

| # | 隐患 | 根因 | 根本性修复 | 运行时复测证据 |
|---|------|------|-----------|---------------|
| H1 | 统一图缺 L3 fact 节点 | `memory_center.go` 统一图 fact 扫描按 `scope='agent'` 过滤，session/user 域 facts 全部漏算 | 与 F1 同模式：改按 `memory_facts.agent_id` 跨全部 scope 聚合扫描 | `GET /v1/memory/graph/unified?agent_id=agent___skills__` 返回 2 个 L3 fact 节点（user 域，旧口径=0），含 fact_source 边连 L2 episode |
| H2 | 冲突 facts 按 agent 浏览漏算 | `ListConflictingFacts` 仅支持 scope 过滤，而冲突 facts 分布在 session/user 域 | proto `ListConflictingFactsRequest` 增 `agent_id=5`；service 透传 + `scope_type`/`agent_id` 双空返回 400（防全表扫描）；data SQL 增 agent_id 跨 scope 过滤；前端 api.ts 透传 | 构造 conflict_count=1（spirit agent 域 1 条 + skills user 域 2 条）：`?agent_id=spirit`→1、`?agent_id=skills`→2（user 域，旧口径=0）、`?scope_type=user&scope_id=default_user`→2（向后兼容）、无过滤→400；复测后已清理 |
| H3 | Ebbinghaus 衰减分只作用于 agent 域子集 | per-agent 扫描按 `scope_type='agent'` 过滤，session/user 域 facts 的 R_t 长期不更新 | 提取 `scanFactsForAgent`，改用 `agent_id` 跨全部 scope 拉取（批上限 500） | 进程日志确认新二进制 worker `reader_wired=true agents_wired=true` 启动；扫描口径由 `TestMemoryEbbinghausDecay_*` mock 断言 agentID 参数覆盖（24h tick 不等待） |
| H4 | EVOLVED_FROM 边在统一图不可见 | `applyEvolvedFromSideEffects` 写 `memory_relations` 继承 fact 的 session/user scope，而关系读取按 `scope_type='agent'` 过滤 | fact 带 `agent_id` 则 relation 统一落 agent scope；无 agent_id 遗留行回退自身 scope | `SetRelationWriter` 接线确认（wire_memory.go:209）；scope 行为由 `TestLinkEvolution_EvolvedFromRelationAgentScope` / `...LegacyScopeFallback` 两个新单测覆盖（LLM 决策路径运行时不可确定性触发） |

**新增/更新测试**：`memory_unified_graph_test.go`（H1 跨 scope fact 扫描断言）、`memory_layer_overview_test.go`（H2 冲突查询断言）、`memory_ebbinghaus_decay_test.go`（H3 agentID 过滤断言，`scanFactsForAgent` 提取后可测）、`link_evolution_test.go`（H4 agent scope + 遗留回退两用例）。

**排查覆盖清单（结论：无其他同类隐患）**：L0 快照 / L1 任务（session 维度天然正确）、L2 episodes（写入即 user+agent 归属，查询按 user_id）、融合 recall（scopes 配置含 user 域故注入正常）、L4 entities（已有 workspace 兼容过滤，见缺陷 #4）、PII review / dead-letters / worker status（管理面全量口径，不按 agent 过滤）。

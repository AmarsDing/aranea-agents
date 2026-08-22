# 评审：组织重型链 Phase 4 落地与横切复盘

> 日期：2026-08-22
> 类型：review
> 状态：Phase 4 恢复缺口已收（ConfirmBlock 禁回落 + 回灌 + checkpoint 读回 + 保留集硬拒 + 启动扫未完成看板 + 确认卡状态机守卫）；跨公司 Brief 仍 YAGNI
> 关联：[M78 开发计划](../development/78-org-aware-orchestration.development.md) · [横切评审](./2026-08-22-review-org-heavy-chain-crosscut.md) · [组织不变量](../development/org-invariants.md)

---

## 1. 总经理有没有进 Agent 管理、挂到岗位上？

**此前没有。** 第一版把 `PositionID` 写成了公司节点 ID（公司不是岗），只在新建公司时触发，前端还把 `company_lead` 当成内置管家藏起来。

**现在有了，需打开一次组织树或 Agent 列表触发回填：**

| 项 | 落点 |
|----|------|
| 部门 | 系统「总经理办公室」`{companyKey}_office`（走 repo 创建，**不**再生一个 `dept_lead`） |
| 岗位 | 「总经理」`{companyKey}_gm` |
| Agent | `__company_lead_{key}__`，`PositionID` = 岗位 ID |
| 回填 | `OrganizationUsecase.Tree` / `List` → `ensureCompanyLeads` |
| UI | `dept_lead` / `company_lead` 进编制/预设区，不进精灵管家区 |

总经理仍是治理身份：`read_only`，不可启发式分配，不可当 Team Lead。

---

## 2. 6KB 剪裁是不是行业做法？有没有更好的？

**6KB 是安全天花板，不是行业最优。** 业界长任务主流是：

1. **结论进窗、材料按需**（Brief / tool retrieval / inbox 指针）
2. **工具 schema 按专项裁剪**，禁止把编排管家全家桶灌给员工
3. **硬顶只挡「装配层还在拼接的那一段」**

更优方式（本轮已按此收口）：首轮只注入 Brief + 交付协议；知识/记忆用已有工具按需取（`knowledge_search`、记忆工具、`read_upstream_deliverable`）。`TrimPrefixBudget` 仍保留，防止协议或上游摘要失控。禁止把 6KB 理解成「先塞满再砍」。

---

## 3. 落地范围（诚实）

**已接热路径**

- 分档 `light/medium/heavy`；跨部门 `DependsOn` 可升档
- 总经理挂岗 + 回填 + 编制区
- 任务**点名**已授权剧本 → Planner 展开，0 总经理 LLM、0 分解 LLM
- 未点名不自动套公司唯一剧本（避免「改一句文案」走上重型链）
- 配方指纹不合则丢历史 keys；Allocate 再滤领导
- 成员首轮 Brief+协议封顶；合成 prompt 禁止考古会话全文
- checkpoint payload 增 omitempty 组织字段；旧 JSON 仍能 Parse
- 确认五档纯函数：默认阶段不弹卡
- 三管道：**契约**（≤2KB、心跳非调度栅栏、deptmail≠Brief）

**本轮续做（已接）**

- 用户点名组织链但公司无剧本 → `playbook_fill_required`，StrategyDirect，**不再 LLM 拆岗**
- 管理面 `AuthorizeCompanyPlaybook` 把剧本写进公司 metadata
- PlanExecutor 阶段开工/完成发 `heartbeat`，失败发 `upward`（≤2KB，非调度栅栏）
- 专项成员记忆注入 `ClampSpecialistL3Scopes`（Spirit 除外）
- DECISION.md 粗路由：组织链只点名剧本
- 前端进度文案：heartbeat / upward / playbook_fill_required

**本轮三条并行（已接）**

- `graph_template_id`：Assemble 写入 definition；builtin 改 intra-team mode；非 builtin 当 `graph_definitions.id` 在 compile/run 加载；缺失资产回落普通 Team Turn
- 花名册工具面：Assemble 盖 `specialist_tool_faces`；effective tools 拒绝 spirit 保留集；领导 clamp 到 `read_only`；MCP 仍走该员工自己的 allow/`mcp:` 前缀
- 确认五档 HITL：造人（Factory）、新剧本（AuthorizeCompanyPlaybook）、高风险门（既有 VerificationGate）、危险工具（既有 hook）、`confirm_before`（PlanExecutor 等待 + ConfirmActivity + steps_v2 ConfirmBlock）。默认阶段交接不弹卡

**未做项第二批 + 恢复收口（已接）**

- `confirm_before` 发出 ConfirmBlock；`pbconfirm:` **禁止**回落工具 await；无内存 waiter 时记决定并 `stepWriter` 关卡；`ctx.Done`/sweep 清 waiter
- 进程重启：`RecoverUnfinishedBoards` 扫 `planning`/`executing` 再 Subscribe；`run()` 只派发 pending 且依赖已完成的步骤（已完成根不重派）
- 剧本确认：仅 `tool_blocked` 可转 completed/cancelled；同决定幂等；反向决定 BadRequest；落库失败不 Accepted / 不 Note
- PlanStep 内存字段从 `task_plans.sub_tasks_json` 回灌（`collection_ids` / `confirm_before` / `graph_template_id`）
- checkpoint 只快照 `executing` 计划；resume 读回 playbook/阶段写入 prompt；`IssuedBriefIDs` 仍空
- 保留集硬拒：Allow 不能授予 `plan_and_execute` 等；Assemble `specialist_tool_faces` 在 team build 消费
- 主对话不渲染 `token_usage` NoticeBlock（既有 `noticeFilter`）

**仍未接**

| 缺口 | 建议 |
|------|------|
| 跨公司总经理 Brief（ORGFAST-31） | 继续 YAGNI |
| 仲裁 dept→GM / company→Spirit（ORGFAST-47） | 未列入本批 |

不新造 Graph 引擎、不复活 ExecutionReportCard、不把 deptmail 当横向水管。

---

## 4. 深入评审：发现与优化

### 做对的

1. **所有权四层 ≠ 每跳串行 LLM。** 分档 + 点名剧本展开符合 ADR。
2. **总经理挂岗**对齐编制表：业务人和领导都在岗位上，Agent 管理能看见。
3. **点名才展开**比「公司只有一本剧本就套上」更安全。
4. **Token 策略**从「拼满再砍」改成「默认不灌知识/记忆」。
5. **合成与交接**继续 Brief-first，和 M27/M71 分层不打架。

### 必须修的（本轮已收口）

1. ~~重型无名剧本仍会 LLM 拆岗~~ → 组织链意图 + 无剧本 = `playbook_fill_required`，管理面 `AuthorizeCompanyPlaybook` 沉淀。
2. ~~L3 默认 `team` scope~~ → 专项注入期 `ClampSpecialistL3Scopes`（默认政策仍含 team，Spirit 保留）。
3. ~~三管道只有尺子没有发射器~~ → PlanExecutor 发 heartbeat/upward，前端有文案。

### 应当收紧的

4. `ensureCompanyLeads` 在每次 Tree/List 上跑，公司多时会反复幂等写。可加「已有 `company_lead_agent_id` 且岗位匹配则跳过」。
5. 办公室部门若日后被 `OrganizationUsecase.Create` 路径碰到，可能补出多余 `dept_lead`。办公室必须保持 repo-only + `IsSystem`。
6. 配方回放丢 keys 后仍按专题槽走花名册，这是对的；要防止空专题 + 空 keys 静默掉进 L3 海选（已有 `domain_path` 则 fail-closed，保持）。

### 明确拒绝

- 为组织链再做一套 Graph
- 把 6KB 改成「先检索再硬塞满」
- 每人开工点一次确认
- 用 memberfs / deptmail / 知识库冒充 Brief

---

## 5. 实际测试

| 包 | 结果 |
|----|------|
| `./internal/agent` | 通过（含点名剧本 0 LLM、轻文案不套剧本、分档升档） |
| `./internal/tools` | 通过 |
| `./internal/scenario/system` | 通过 |
| `./internal/biz` 本批单测 | 通过（挂岗、剧本点名、前缀、管道锁、确认五档、旧 checkpoint、领导不继承 spirit 工具、合成 Brief-only） |
| `./internal/biz` 全量 | 与本变更无关的 PG 集成测失败（`aranea_test` 密码） |

未跑带 LLM 的手工长任务，也未连真实前端点一次「组织树」。回填生效条件：进程起来后打开组织或 Agent 管理页。

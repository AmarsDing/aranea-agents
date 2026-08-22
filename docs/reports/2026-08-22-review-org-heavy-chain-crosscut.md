# 评审：重型组织链横切（速度 / 传递 / 共享 / Token / 记忆 / 知识 / 工具 / 图 / 确认 / 干预 / 汇总）

> 日期：2026-08-22
> 类型：review
> 状态：已采纳，并入 M78 设计 §十四 / R14–R20
> 关联：[重型链 ADR](./2026-08-22-review-adr-org-heavy-chain.md) · [M78 设计](../development/78-org-aware-orchestration.design.md) · [M70](../development/70-orchestration-longtask-memory.md) · [M71](../development/71-agent-resource-sharing.md) · [M53](../development/53-team-graph-orchestration.md)

---

## 结论

分档、预授权剧本、三管道、新开 Team **仍然成立**，但现稿只覆盖「谁调度、谁传话」。长任务真正被拖死的点在横切：全量工具 schema、知识/记忆叠加上文、过密确认、把成员过程刷进精灵窗、用 deptmail/知识库冒充交接。

**最优方案**：组织链继续只做所有权与例外；执行内核沿用 PlanExecutor + 成员会话 + 既有 Pause/Inject/HITL/精灵汇总。给每名成员设 **6KB 首轮前缀硬顶**，工具/MCP 按专项裁剪，知识引用不复制，记忆分层隔离，确认默认少点。

拒绝：为组织链再造 Graph 引擎、报告卡片、第二套共享盘、每成员开工确认、总经理读成员全文。

## 层面决议

| 层面 | 决议 |
|------|------|
| 速度 | 墙钟 + token 一起算。并行开工保留；砍确认、砍全量 MCP、砍灌窗 |
| 消息 | 三管道不变。deptmail ≠ 横向；用户指令走 inject，不走上行 |
| 共享 | Brief / Bulk / 同团工作树 / memberfs / 知识引用 五层，互不冒充 |
| Token | Brief 2KB + 知识 2KB + 记忆 1KB + 协议 1KB，合计 ≤6KB；超则先砍知识再砍记忆 |
| 记忆 | 配方=班底；checkpoint=续跑；L1=本岗工作记忆；L3=事后个人经验；禁止兄弟互读 L3 |
| 知识 | 阶段绑 `collection_ids`；检索引用；写回待审；不当消息总线 |
| 工具/MCP | 花名册允许集；领导只有治理工具；危险工具走既有 HITL |
| 编排图 | 默认 PlanExecutor；阶段可挂已有 `graph_template_id`；不新引擎 |
| 确认 | 五档（造人 / 新剧本 / 高风险门 / 危险工具 / 剧本 `confirm_before`）；默认阶段自动过 |
| 可观测/干预 | 用户看芯片+心跳+例外；成员会话 pause/inject/cancel；换人=剩余工作新开 Team |
| 汇总 | 只吃 Brief + 例外 + 制品清单；取消则跳过（沿用现网精灵 reply） |

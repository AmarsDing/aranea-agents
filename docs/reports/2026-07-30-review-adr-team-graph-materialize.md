# ADR-10: Team 拓扑全量物化为 Graph 资产（C1）— 替代 ADR-08 embedded 真相源条款

## 状态：已接受（2026-07-30，M53 Phase 11 已落地）

## 背景

ADR-08 将 Team 编排统一为「embedded graph 为拓扑唯一真相源」，解决了 mode/表单与图的双真相源割裂。但 embedded graph 内嵌在 `teams.definition_json` 中，**不是独立 Graph 资产**，Team 与 Graph 工作流仍处于双轨割裂态：

1. **资产不可见**：Team 的拓扑图不出现在 GraphsPage，无法复用 Graph 编辑器的版本管理、布局、校验、Checkpoint/HITL 等成熟能力；用户无法判断 Team 的图与 Graph 资产是什么关系。
2. **执行历史割裂**：Team 运行以 `team:` 前缀合成 ID 注册 graph execution，`/graphs/:id/executions` 看不到 Team 的全部执行历史；同一 Team 多次运行无法聚合回放。
3. **观测双标**：Team 观测台（TeamRunObservatoryPage）无 Checkpoint/时间旅行能力；Graph 运行页（GraphRunPage）无 Agent Kanban 视角；两者不能互跳。
4. **编辑语义未定义**：表单（mode/members 派生）与 Graph 编辑器（自由拓扑）都能改同一份拓扑时，谁是源、何时覆盖、如何告知用户，均无定义；embedded graph 也无法区分「表单派生」与「手工编辑」。
5. **生命周期未定义**：Team 换绑/删除时 embedded 图随 definition_json 湮灭，历史 run 失去拓扑回放依据；独立图被 Team 引用时删除无保护。

评审结论：embedded 只是「真相源」的逻辑统一，未做到「资产统一」。需要把 Team 拓扑**物化（materialize）为独立 Graph 资产**，让 Team 与 Graph 工作流在同一资产模型上收敛。

## 决策

**C1 全量物化：Graph 资产是 Team 拓扑的唯一真相源；`definition_json` 只存 `linked_graph_id` + `source`，不再写 `graph` 字段。**

### 1. 三态 source 模型

| source | 含义 | 物化行为 |
|--------|------|---------|
| `preset` | 表单（mode/members）派生（缺省态） | 保存时按表单重建 owned 图资产 |
| `custom` | 拓扑已被手工编辑（Graph 编辑器或重置前确认） | 表单改拓扑字段需前端覆盖确认后重建 |
| `linked_external` | 关联独立 Graph 资产 | 不物化；`linked_graph_id` 指向外部图 |

### 2. 双向编辑语义（双路径 + 覆盖警告）

- **表单路径**：preset/custom 改拓扑字段 → 物化器重建 owned 图（`inheritGraphLayout` 保留存活节点坐标）；custom 态前端弹覆盖确认。
- **Graph 编辑器路径**：保存 team-owned 图（`metadata.team_owned=true`）→ 反向同步属主 team：`source=custom` + members 从图 agent 节点派生（`DeriveMembersFromGraphNodes`，agent key→id 反查，失败跳过记 warn）；属主有活跃 Run 时双向对称拒绝。
- 「重置为派生」：custom → preset，确认后物化重建。

### 3. 事务与生命周期

- **D1**：物化与 team 保存同一事务（`TeamTxProvider.ExecInTx`）；物化失败（无启用成员/校验不过）→ 保存整体失败，返回节点级校验明细，不静默降级。
- **D2**：换绑 external → 旧 owned 图直接删除（历史 run 靠执行 steps 降级回放 +「资产已删除」提示）；删 team-owned 图（属主存在）拒绝；删被 external 引用的独立图拒绝并列引用者；删 team 级联删 owned 图、external 图只解绑。
- **循环关联防御**：external 关联目标不得是 team-owned 图。

### 4. 真实执行 ID 与观测对齐

- `RegisterTeamGraphExecution` 使用 `linked_graph_id`（真实资产 ID），linked 为空保留 `team:` 合成兜底；同一 Team 多次 Run 共享资产 ID，`/graphs/:id/executions` 自然聚合全部执行历史。
- TeamRunObservatoryPage 新增 Checkpoint tab（enable_checkpoint 时启用，复用 graph API）+ 工具栏「Graph 执行视角」跳转；GraphRunPage 对 team 执行（图 `team_id` 非空）显示 Kanban tab（`buildGraphRunKanbanNodes` 投影复用 OrchestrationKanban），悬空 graph_id 从执行 steps 合成只读拓扑降级渲染。

### 5. 存量迁移（L3 批量 + 惰性兜底）

- data migration `20260730_team_graph_materialize`：扫描 `definition_json` 含非空 `graph` 且 `linked_graph_id` 为空的 team，逐队物化回写；**拓扑等价判定**（物化结果与 embedded graph 节点 ID 集 + 边端点对一致 → preset，否则 custom）。
- 幂等：linked 非空跳过；单 team 失败记 warn 继续，不阻塞启动。
- 惰性兜底：运行时（`TeamGraphAssetEnsurer`）linked 仍为空 → 先物化再继续；embedded 字段保留不物理删除（只退役写入）。

## 后果

### 正面

- **资产级单一真相源**：Team 拓扑成为一等 Graph 资产，编辑器/版本/校验/Checkpoint/HITL/删除保护全部复用，无第二套实现。
- **执行历史聚合**：真实 graph_id 注册执行，`/graphs/:id/executions` 展示 Team 全部执行；观测双视角互跳闭环。
- **编辑语义显式化**：三态 source + 覆盖确认 + 反向同步，用户始终知道「改哪份、谁会覆盖谁」。
- **迁移安全**：批量幂等 + 惰性兜底 + 拓扑等价判定，存量平滑过渡，embedded 字段不物理删除可回查。

### 负面/残差

- **D2 删除后拓扑信息有损**：owned 图删除后历史 run 只能从执行 steps 合成节点（无边/无坐标），降级为只读快照回放——已接受（活动记录保留输入/输出/状态，满足排障主诉求）。
- **物化器成为保存关键路径**：D1 同事务使物化失败阻断 team 保存——刻意为之（不静默降级），但物化器缺陷的影响面扩大，需靠六 mode 单测 + PG 集成测试兜底。
- **兼容路径维护期**：运行时 embedded/mode 模板兼容路径（linked 为空时）在迁移完成前保留，迁移全量落地后方可标记退役。
- **反向同步是单向派生**：图 → members 反查依赖 agent key→id 解析，解析失败的节点被跳过（记 warn），members 可能与图短暂不一致——以图为准，下次表单保存重新对齐。

## 替代方案

- **C2（embedded 保留 + 资产投影双写）**：保留 embedded 为运行输入，另投影一份 Graph 资产用于展示/编辑。否决：双写引入新的一致性负担（投影滞后/漂移/冲突仲裁），删除与换绑语义比物化更复杂，且「真相源」重新变为两份。
- **维持双轨（不收敛）**：Team 继续用 embedded，Graph 资产独立演进。否决：资产不可见、执行历史割裂、观测双标、编辑语义缺失等评审问题全部保留。
- **C1 变体（external 引用也算 owned）**：统一用 `team_id` 判定属主。否决：linked_external 图的 `team_id` 仅作回填关联，删除/换绑语义与 owned 不同，必须经 `metadata.team_owned` 区分（前后端镜像同一判定）。

## 与 ADR-08 的关系

本 ADR **替代 ADR-08 的「embedded graph 为拓扑唯一真相源」条款**——真相源从「内嵌于 definition_json 的 graph 字段」升级为「独立 Graph 资产（`linked_graph_id` 引用）」。ADR-08 其余条款继续有效：mode 退化为模板选择器、角色由 mode+成员顺序派生、携带图时跳过 role-mode 耦合校验、Team 编辑器移除 runtime_engine。

## 验证

- 后端：物化器六 mode / 坐标保留 / source 转换 / members 派生 / 删除保护 / D1 事务回滚 / L3 迁移幂等 + preset-custom 判定 / 真实执行 ID 断言，biz+data 测试全绿（B1-B10，见 `docs/development/53-team-graph-orchestration.development.md` Phase 11.A）。
- 前端：F1-F7 各组件/composable/纯函数测试全绿（source 读写、警告条、编辑器跳转、保存确认、badge/过滤、Checkpoint tab、Kanban/降级），`pnpm lint && pnpm test && pnpm build` 通过。

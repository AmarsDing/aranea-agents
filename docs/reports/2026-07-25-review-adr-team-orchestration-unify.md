# ADR-08: 团队编排统一 — graph 为拓扑唯一真相源，mode 退化为模板选择器

## 状态：已接受（2026-07-25，Phase A 已落地）

## 背景

团队（Team）编排长期存在**两套割裂的拓扑表达**：

- **编排模式（mode）路径**：`mode`（sequential / parallel / coordinator / critic_loop / swarm / adaptive）+ `members[].role` + `synthesizer_agent_id` 等表单字段，各自独立维护，前端 `graphUtils.buildGraphFromDefinition` 本地生成预览图。
- **Graph 编排路径**：`definition.graph`（embedded graph spec，nodes/edges），运行期编译（`compileFromEmbeddedGraph`）的唯一输入。

割裂点：

1. **双真相源**：表单改了 mode/members，已持久化的 embedded graph 不会自动更新（陈旧 graph 覆盖表单改动）；反之改了 graph，表单字段失去意义。用户无法判断运行时到底按哪份拓扑执行。
2. **模板逻辑双份**：前端 `graphUtils` 与后端 `generateGraphSpecFromMode` 各自实现 mode→graph 模板，语义漂移风险。
3. **人工字段冗余**：`role` / `synthesizer_agent_id` 本可由 mode + 成员顺序派生（如 parallel 末位成员即汇总），却要求用户手工设置并与 mode 保持兼容，校验（role-mode 耦合）本质上是在防止用户把两套真相改得不一致。
4. **校验割裂**：保存时 biz/前端做 role-mode 耦合校验，但携带 custom graph 的团队拓扑由 graph 决定，耦合校验误报；真正的结构问题（悬空边/环/缺 entry）只有编译期才能发现。
5. **runtime_engine 暴露**：Native 路径已在 Phase 8 移除主路径，Team 编辑器仍暴露执行引擎选择器，与「GraphAgent 为唯一执行引擎」的终态矛盾。

## 决策

**embedded graph 为拓扑唯一真相源；mode 退化为模板选择器；角色由 mode + 成员顺序派生；Team 编辑器移除 runtime_engine（统一 Graph 运行时）。**

Phase A 具体改动：

1. **A1 拓扑→graph 单向派生**：
   - 拓扑字段指纹 `definitionTopologyKey`（仅 mode / synthesizer_agent_id / members 拓扑；非拓扑字段不触发；members 按 sort_order 稳定排序）。
   - 编辑器 topology watcher：指纹变化 → 角色派生 → `rebuildDefinitionGraph`（layout 未变保留存活节点 x/y，防画布漂移）。
2. **A2 模板去重（后端 canonical）**：`CompileTeamGraph` RPC 响应新增 `definition_graph_json`（后端按 definition 解析出的 canonical embedded graph spec）；前端 `definitionGraphFromCompileJSON` 应用；本地 `graphUtils` 降级为离线/编译失败回退，不再是模板真相。
3. **A3 编辑器联动**：
   - mode 选项带模板语义描述（`modeOptions[].description`），mode 即模板选择器。
   - `deriveMemberRolesForMode` 派生角色：sequential 全 worker；parallel 末位启用成员 synthesizer（回写 `synthesizer_agent_id`）其余 worker；coordinator 首位 coordinator；critic_loop 交替 generator/critic；adaptive/swarm 不派生。派生模式下编辑器角色字段**只读**；parallel 汇总 Agent 只读展示。
   - 策略区条件显隐：`parallel_fail` 仅 parallel 显示；失败策略扩展区更名（执行引擎选择器移除）。
   - **移除 runtime_engine**：`TeamEditorDialog` 删除执行引擎选择器 / `nativeLocked` / `isPlatformAdmin` prop；`openEdit` 归一 `runtime_engine='graph'`；native 仅保留编排页 `TeamOrchestrateRuntimePanel` 供平台管理员调试。
4. **A4 校验改造（前后端同 PR 镜像）**：definition 携带 embedded graph（`graph.nodes` 非空）时，`validateTeamDefinition` 跳过 role-mode 耦合校验（角色兼容 / parallel 汇总 / coordinator / critic_loop 角色要求），结构问题由 `CompileTeamGraph` 编译期校验报告；保留基础校验（≥1 启用成员、agent_id 必填）；`graph.nodes` 为空数组不视为携带 graph。

## 后果

### 正面

- **单一真相源**：graph 是唯一拓扑；表单字段只是 graph 的生成输入，不存在「表单与图不一致」。
- **模板单份实现**：mode→graph 模板仅后端 `generateGraphSpecFromMode` 一份，前端本地 builder 只是回退，消除双份漂移。
- **编辑器减负**：角色/汇总 Agent/执行引擎不再需要人工设置；mode 选择即所得，派生字段只读透明。
- **校验语义正确**：custom graph 团队不再被 role-mode 耦合校验误拦；结构问题在编译预览面板实时可见（fail-fast 于编译期而非保存后运行期）。
- **运行时收敛**：Team 编辑器不再暴露已退役的 native 路径，与 Phase 8 单轨化一致。

### 负面

- **派生模式下角色不可手工覆盖**：用户想给 sequential 成员标 coordinator 角色需切到 adaptive/swarm 或改 graph。补偿：角色在 graph 运行时不影响执行语义（拓扑由边决定），仅作展示/提示。
- **旧 native 定义打开即归一**：遗留 `runtime_engine=native` 的 Team 在编辑器中打开并保存后永久转为 graph，无 UI 回退路径。缓解：native 主路径已移除（Phase 8），实际运行本就归 graph；编排页保留 admin 调试入口。
- **离线/编译失败时回退到本地 builder**：回退路径的模板语义依赖前端 `graphUtils`，与后端可能漂移。缓解：仅离线/失败时启用，在线时后端 canonical 覆盖；A2 保留了双实现的对齐测试。
- **Biz 测试二进制被并发 WIP 阻塞**：落地期间 `internal/agent` 存在并发未定义符号（与本决策无关），`go test ./internal/biz` 经 `team_graph_linked_test.go → internal/team → internal/agent` 链不可编译，A4 后端验证采用临时移出该文件隔离运行（31/31 PASS，见 §验证）。

## 替代方案

- **方案 1（保留 mode 为真相，graph 仅为预览）**：回到 Phase 8 之前的割裂态，graph 编排（HITL/条件边/自由拓扑）无法表达，否决。
- **方案 2（双向同步 mode ↔ graph）**：从 graph 反推 mode/role 在自由拓扑下不可靠（无唯一解），同步规则复杂度爆炸，否决。
- **方案 3（保留 role 手工设置 + 仅警告）**：不解决「人工字段冗余」，用户仍须理解 role-mode 耦合矩阵，编辑器减负目标落空，否决。

## 验证

- 前端：`pnpm vitest run src/components/teams/__tests__ src/features/teams/__tests__` **71/71 PASS**（`teamUtils.spec.ts` 派生/指纹/重建；`teamValidation.spec.ts` graph 跳过校验）；`pnpm eslint` 改动文件 0 问题。
- 后端：`go build ./internal/biz` exit 0；`biz/team_usecase_test.go::TestValidateTeamDefinition` **31/31 PASS**（新增 7 例：graph 跳过三类 mode 要求 / 保留 enabled+agent_id / 空 nodes 不跳过）。因 `internal/agent` 并发 WIP 未定义符号（`tryDomainRecipe` / `TopLevelDomain` 等，与本决策无关）阻塞测试二进制，验证时将 `team_graph_linked_test.go` 临时移出包隔离运行（该文件仅经 `internal/team` 链拉入 agent 包，隔离不影响被测逻辑），测后已恢复。
- 文档同步：M53 三件套（需求 US-08/US-10 修订 + US-11 新增；设计 §十二；开发计划 Phase 10）。

## Phase B（已完成 2026-07-25）

- ✅ `definitionToJSON` native 序列化分支清理：固定写 `runtime_engine='graph'` / `team_graph_runtime=true`；`parseDefinition` 读取遗留 native 数据即归一为 graph（`teamUtils.ts`）。
- ✅ `runtimeEngineOptions` / `runtimeEngineLabel` / `resolveRuntimeEngine` 全部移除；`TeamOrchestrateRuntimePanel` native 调试入口随执行引擎选择器一并移除，重构为只读中文摘要（失败策略 + 超时）。
- ✅ 编排页三 Tab 去技术编码：工具栏副标题改为「{中文模式}编排」；编排信息面板改为「执行方式（`teamTopologySummary` 中文流程摘要）+ 成员（中文名 + 中文角色）+ 运行与容错」，移除入口/出口 node id、`linked_graph_id` 输入框、成员节点技术编码列表。
- ✅ `TeamCompilePreview` 重构：删除边列表（`member-1 → member-2` 技术编码）与「入口 x → y」，改为中文流程摘要 + 成员中文名/角色列表（`teamMemberDisplayRows`）。
- ✅ `TeamMemberKanban` / `TeamOrchestrateNodePanel` 移除 agent 编码与节点类型 badge 显示（技术编码保留在数据层供调试，不进 UI）。
- 遗留：mode 字段只读化待 graph 完全接管后评估（当前 mode 仍承担模板选择器语义，见 §决策）。

### Phase B 验证

- 前端：`pnpm lint` 0 errors；`pnpm test` **877/877 PASS**（新增 `teamNodeDisplay.spec.ts` 9 例覆盖六种模式的中文摘要、`teamUtils.spec.ts` 新增 native→graph 归一化用例）；`pnpm build` 成功。

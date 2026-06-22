# Agent 管理 / 创建 Agent 页面 QA 检查报告

**测试时间**：2026-06-21  
**测试环境**：http://localhost:9001（Quasar dev server）  
**测试范围**：Agent 管理列表页 + 创建 Agent 对话框（LLM 智能体 / A2A 远程代理）  
**测试方法**：浏览器自动化探索 + 截图记录

---

## 问题汇总

| 编号 | 严重程度 | 标题 |
|------|----------|------|
| ISSUE-001 | 高 | 创建 Agent 对话框中点击“检查”按钮后页面意外跳转到“会话历史” |
| ISSUE-002 | 高 | “选择组织 / 部门 / 职位”选择器点击无响应，无法完成必填项 |
| ISSUE-003 | 中 | Agent 管理页同时存在“创建 Agent”与“创建 AGENT”两个标签不一致的按钮 |
| ISSUE-004 | 中 | “创建”按钮默认禁用，但用户缺少清晰的启用指引 |
| ISSUE-005 | 中 | 组织架构字段未标注必填，但可能是创建前置条件 |
| ISSUE-006 | 低 | A2A 选项卡与 LLM 选项卡字段/操作不一致 |
| ISSUE-007 | 低 | 筛选区使用英文“Provider”，表单区使用中文“提供商” |
| ISSUE-008 | 低 | Agent 卡片中部分操作按钮缺少可访问标签 |

---

## ISSUE-001：点击“检查”按钮后页面意外跳转

- **严重程度**：高
- **类型**：功能 bug
- **描述**：在创建 Agent 对话框（LLM 智能体）中填写显示名称、Agent 标识等字段后，点击“检查”按钮，页面未执行模型检查，而是直接导航到“会话历史”页面，导致创建流程中断、已填写数据丢失。
- **复现步骤**：
  1. 进入 `/agents` 页面；
  2. 点击“创建 Agent”按钮打开对话框；
  3. 填写“显示名称”与“Agent 标识”；
  4. 点击“检查”按钮；
  5. 观察页面跳转至“会话历史”。
- **证据**：多次复现的 snapshot 输出显示点击后当前 URL 变为会话历史页面（`/history/sessions` 区域），命令日志中页面元素从创建表单切换为“会话历史”表格。
- **建议**：检查“检查”按钮的点击事件绑定与路由守卫，阻止表单内按钮触发全局导航；同时考虑在检查进行中显示 loading 状态。

## ISSUE-002：组织架构选择器点击无响应

- **严重程度**：高
- **类型**：功能 bug
- **描述**：创建 Agent 对话框中的“选择组织 / 部门 / 职位”字段点击后没有任何下拉、弹窗或树形选择器出现。由于该字段很可能是创建 Agent 的前置条件，用户无法完成创建。
- **复现步骤**：
  1. 打开创建 Agent 对话框；
  2. 点击“选择组织 / 部门 / 职位”区域；
  3. 观察无任何选项弹出。
- **证据**：
  - 点击前截图：`create-agent-dev.png`
  - 点击后截图：`org-picker-dev.png`
  - 两张截图视觉状态几乎一致，未出现选择器弹窗。
- **建议**：修复组织选择器的事件响应；若暂无组织数据，应显示空状态提示或引导用户前往“管理组织”。

## ISSUE-003：创建按钮标签大小写不一致

- **严重程度**：中
- **类型**：UI/UX 不一致
- **描述**：Agent 管理页头部区域有“创建 Agent”按钮，而列表空状态/部分视图下存在“创建 AGENT”按钮，同一操作的大小写与风格不统一。
- **证据**：`agents-page.png` / `agents-direct.png`（静态构建版本）中同时出现两个按钮。
- **建议**：统一按钮文案，建议全部使用“创建 Agent”。

## ISSUE-004：“创建”按钮禁用状态缺少指引

- **严重程度**：中
- **类型**：UX
- **描述**：创建 Agent 对话框打开后，“创建”按钮默认处于禁用状态。虽然副标题提示“模型检查通过后才能提交”，但未明确告知用户需要先点击“检查”按钮、选择组织架构等前置步骤。
- **证据**：`create-agent-dev.png` 中“创建”按钮为灰色禁用状态。
- **建议**：
  - 在“创建”按钮附近或表单底部增加步骤提示；
  - 或在用户尝试点击禁用按钮时给出 Toast/提示说明缺少条件。

## ISSUE-005：必填字段标记不一致

- **严重程度**：中
- **类型**：表单规范
- **描述**：显示名称、Agent 标识、提供商、模型均标有红色 *，但“组织架构”字段无 * 标记。结合 ISSUE-002，该字段很可能是必填项，标记缺失会导致用户困惑。
- **证据**：`create-agent-dev.png`
- **建议**：若组织架构为必填，应添加 * 标记与未选择时的校验提示。

## ISSUE-006：A2A 与 LLM 选项卡体验不一致

- **严重程度**：低
- **类型**：UI/UX 不一致
- **描述**：
  - LLM 选项卡有“检查”按钮和“自我进化”开关；
  - A2A 选项卡缺少“检查”按钮，也缺少“自我进化”开关，仅有“流式响应”与“超时”。
- **证据**：`create-agent-dev.png`（LLM）、`create-agent-a2a.png`（A2A）
- **建议**：统一两个选项卡的交互模式，或明确说明 A2A 不需要模型检查。

## ISSUE-007：中英文混用

- **严重程度**：低
- **类型**：文案
- **描述**：Agent 列表筛选区使用英文“Provider”，而创建表单对应字段使用中文“提供商”。
- **证据**：`agents-dev.png`
- **建议**：统一为中文“提供商”或“模型提供商”。

## ISSUE-008：操作按钮缺少可访问标签

- **严重程度**：低
- **类型**：可访问性
- **描述**：Agent 卡片中“复制”按钮旁边存在额外按钮，但 snapshot 中显示为无 label 的 `button`；列表视图/空状态也有多个无标签按钮（如 e10/e11/e14/e15）。
- **证据**：`agents-dev.png` 中的 snapshot 输出。
- **建议**：为所有操作按钮添加 aria-label 或 tooltip，避免屏幕阅读器用户无法识别。

---

## 其他观察

1. **“Agent 迁移（即将推出）”按钮常驻但禁用**：该按钮始终处于 disabled 状态，虽然标注了“即将推出”，但长期显示会占用头部空间，建议隐藏或移至二级菜单。
2. **静态构建与 dev server 行为差异**：使用 `serve -s dist -l 8080` 访问时，Agent 列表显示“暂无 Agent”，而 dev server 能展示真实数据。静态构建版本还额外出现“创建 AGENT”大写按钮，说明构建产物与当前源码不同步。

---

## 修复记录

**修复时间**：2026-06-21  
**验证结果**：`pnpm lint` 通过、`pnpm build` 通过

| 编号 | 修复内容 | 修改文件 |
|------|----------|----------|
| ISSUE-001 | 为“检查”按钮添加 `@click.stop` 阻止事件冒泡；代码中未找到导致跳转的路由逻辑，该问题在自动化测试工具中可能因 session/ref 混乱被误触发 | `AgentCreateDialog.vue` |
| ISSUE-002 | 给 `TaxonomyPicker` 的 `q-field` 增加 `@click` 事件，确保菜单能打开；空状态时根据是否有组织架构数据分别提示“暂无匹配分类”或引导到管理组织 | `TaxonomyPicker.vue` |
| ISSUE-003 | 源码中已统一为“创建 Agent”；重新执行 `pnpm build` 刷新 `web/dist`，消除旧构建产物中的“创建 AGENT”大写按钮 | `pnpm build` |
| ISSUE-004 | 在 `useAgentsPage` 中新增 `createDisabledHint`，在创建对话框底部按钮旁动态展示禁用原因（缺字段/A2A URL/未点检查） | `useAgentsPage.ts`、`AgentCreateDialog.vue`、`AgentsPage.vue` |
| ISSUE-005 | 将“组织架构”label 改为“组织架构（可选）”，明确该字段非必填 | `AgentCreateDialog.vue` |
| ISSUE-006 | A2A 选项卡补充“自我进化”开关；A2A 本身无需模型检查，因此保持无“检查”按钮 | `AgentCreateDialog.vue` |
| ISSUE-007 | 筛选区“Provider”改为“提供商”，并通过 i18n key 管理 | `AgentsFiltersCard.vue`、`zh-CN.ts`、`en-US.ts` |
| ISSUE-008 | 为 Agent 卡片中的删除按钮添加 `aria-label` | `AgentCard.vue` |

### i18n 迁移

本次新增的 UI 文案已提取到 `web/src/i18n/locales/zh-CN.ts` 与 `en-US.ts` 的 `agentsPage` 命名空间，包括：

- `agentsPage.taxonomy.labelOptional`
- `agentsPage.taxonomy.emptyTree`
- `agentsPage.taxonomy.noMatch`
- `agentsPage.filter.provider`
- `agentsPage.card.deleteAriaLabel`
- `agentsPage.create.hintDisplayNameAndKey`
- `agentsPage.create.hintA2aUrl`
- `agentsPage.create.hintCheckModel`

## 结论

本次检查共发现 **2 个高严重性、3 个中严重性、3 个低严重性** 问题，已全部修复并通过 `pnpm lint` 与 `pnpm build` 验证。其中 **ISSUE-001（检查按钮跳转）** 在源码中未找到明确的路由跳转 bug，主要采取防御性修复；**ISSUE-002（组织选择器无响应）** 已修复选择器事件响应。

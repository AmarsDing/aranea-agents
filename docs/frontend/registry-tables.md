# Registry 表格规范

> 列表页 / 监控 / Dialog 内 `q-table` 的列定义、对齐、列宽与样式落点。  
> 与 [UX.md](./UX.md)（玻璃 chrome）、[frontend-guide.md](../guides/frontend-guide.md)（分层）配合使用。

---

## 架构（三层）

| 层 | 路径 | 职责 |
|----|------|------|
| 基础设施 | `web/src/features/ui/registryTableColumns.ts` | `registryCol`、`REGISTRY_COL_W`、`registryColActions` / `registryColEnabled` |
| 基础设施 | `web/src/features/ui/useResizableRegistryColumns.ts` | 表头拖拽 + localStorage |
| 基础设施 | `web/src/components/layout/AppRegistryTable.vue` | 统一 QTable 壳（flat、dense、分页隐藏、列拖拽） |
| 领域 UI | `components/<域>/*Ui.ts` 或 `features/<域>/*TableUi.ts` | `XXX_TABLE_COLUMNS` + 格式化函数 |
| 展示 | `components/<域>/*Table.vue` | 仅 `#body-cell-*` slots，**不写 columns 数组** |
| 样式 | `web/src/css/theme/_registry-page.sass` | `.app-registry-*` 单元格语义类、表格 chrome |

轻量只读表（无 loading / 分页）可用 `AppRegistryMarkupTable.vue`。

---

## 列定义（必须）

### 使用 `registryCol`

```typescript
import { REGISTRY_COL_W, registryCol, registryColActions, registryColEnabled } from "../../features/ui/registryTableColumns";

export const TOOL_TABLE_COLUMNS = [
  registryCol<Tool>("name", "Tool", "display_name", "left", REGISTRY_COL_W.name),
  registryColEnabled<Tool>(),
  registryColActions<Tool>(),
];
```

参数顺序：`name` → `label` → `field` → **`align`** → `width` → `extra?`

### 禁止

- 在 `*Table.vue` 或 Page 的 `<script>` 里手写 `{ style: "width: …", headerStyle: "width: …" }`
- 只写 `style` 不写 `headerStyle`（Quasar 表头/表体各读一处；`registryCol` 已同步）

### 列宽 token（`REGISTRY_COL_W`）

| Token | 典型用途 |
|-------|----------|
| `name` / `nameWide` | 名称列（14% / 18%） |
| `desc` | 规则、描述（16%） |
| `status` / `category` / `time` | 状态、分类、时间 |
| `agent` / `session` | Agent、Session |
| `enabled` | Toggle 列（64px，居中） |
| `metric` / `narrow` | 数值、短标签（72px / 64px） |
| `actions` / `actionsWide` | 操作列（108px / 148px，右对齐） |

需要完整 CSS（如 `max-width`）时 width 传 `"16%; max-width: 168px"`。

### 对齐约定（`align`）

| 列类型 | align |
|--------|-------|
| 名称、描述、时间 | `left` |
| Toggle、状态 badge | `center` |
| 操作按钮 | `right` |
| 纯数字 | `right`（推荐） |

自定义 `#body-cell-*` 内若用 `row items-center` 等 flex，列 `align` 可能不明显，需在 slot 内用 `justify-*` 微调。

### Preset

- `registryColEnabled()` — 启用 Toggle 列
- `registryColActions(width?, label?, field?)` — 操作列 + `app-registry-col-actions` 类

---

## 表格组件（必须）

```vue
<AppRegistryTable
  table-class="tools-data-table"
  :columns="TOOL_TABLE_COLUMNS"
  column-persist-key="tools-table"
  hide-pagination
  :pagination="{ rowsPerPage: 0 }"
>
  <template #body-cell-name="props">
    <q-td :props="props">
      <div class="app-registry-cell-primary ellipsis">{{ props.row.display_name }}</div>
      <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
    </q-td>
  </template>
</AppRegistryTable>
```

| Prop | 说明 |
|------|------|
| `table-class` | 域特有样式时注册到 `_registry-page.sass`（如 `.tools-data-table.q-table`） |
| `column-persist-key` | 同页多表必须唯一；拖拽宽度存 localStorage |
| `:shell="false"` | Dialog / 嵌套 panel 内由外层提供玻璃壳 |
| `:resizable="false"` | 小表 / Dialog 内只读表可关闭拖拽 |

---

## 单元格样式

优先使用全局语义类（`_registry-page.sass`）：

| 类名 | 用途 |
|------|------|
| `.app-registry-cell-primary` | 主文本 |
| `.app-registry-cell-sub` | 副文本（key、时间戳） |
| `.app-registry-cell-desc` | 多行描述 |
| `.app-registry-cell-actions` | 操作按钮组 |
| `.app-registry-chip-wrap` | tag / chip 容器 |
| `.app-registry-icon-btn` | 圆形 icon 按钮 |

域特有样式：`.{domain}-data-table__*`（BEM），写在 `_registry-page.sass` 对应块内，**勿**在 Table.vue scoped 里重复 thead/td padding。

---

## 文件命名

| 场景 | 导出 | 文件示例 |
|------|------|----------|
| 单表 | `TOOL_TABLE_COLUMNS` | `components/tools/toolUi.ts` |
| 多表同域 | `SKILL_TABLE_COLUMNS` / `SKILL_RUNS_TABLE_COLUMNS` | `components/skills/skillTableUi.ts` |
| Page composable | `CRON_TASK_TABLE_COLUMNS` | `features/cron/cronTableUi.ts` |
| 需动态 field | `buildAgentTableColumns(...)` | `components/agents/agentTableUi.ts` |

---

## 新表 Checklist

- [ ] 在 `*Ui.ts` / `*TableUi.ts` 定义 `XXX_TABLE_COLUMNS`（`registryCol` + `REGISTRY_COL_W`）
- [ ] `*Table.vue` 只用 `AppRegistryTable` + body-cell slots
- [ ] 设置 `column-persist-key`
- [ ] 单元格用 `.app-registry-cell-*`
- [ ] 有域样式时：`table-class` + `_registry-page.sass` 注册
- [ ] `cd web && pnpm build`

---

## 参考实现

| 场景 | 文件 |
|------|------|
| 标准 Registry 列表 | `PluginsTable.vue` + `pluginUi.ts` |
| 多 variant 列 | `HooksTable.vue` + `hookTableUi.ts` |
| Provider 宽表 | `ProviderModelsTable.vue` + `providerModelUi.ts` |
| 会话 sticky 操作列 | `sessionUi.ts`（`app-registry-col-actions`） |
| 基础设施源码 | `registryTableColumns.ts`、`AppRegistryTable.vue` |

## 🚨 你必须遵守的关键规则
### 结构优化层级（按顺序应用）
1. **Tab 分离**——如果表单有逻辑上不同的字段组（例如基本信息 vs. 设置 vs. 元数据），拆分成 `Tabs` 并使用 `->persistTabInQueryString()`
2. **并排分区**——使用 `Grid::make(2)->schema([Section::make(...), Section::make(...)])` 将相关分区并排放置，而不是垂直堆叠
3. **用滑块替换单选行**——一行十个单选按钮是 UX 反模式。使用 `TextInput::make()->type('range')` 或在窄栅格中使用紧凑的 `Radio::make()->inline()->options(...)`
4. **可折叠的次要分区**——大多数时候为空的分区（例如崩溃、备注）应默认 `->collapsible()->collapsed()`
5. **Repeater 项标签**——务必在 repeater 上设置 `->itemLabel()`，让条目一眼可辨（例如 `"14:00 — 午餐"` 而不是 `"Item 1"`）
6. **摘要占位符**——对于编辑表单，在顶部添加紧凑的 `Placeholder` 或 `ViewField`，显示记录关键指标的可读摘要
7. **导航分组**——将 resource 分组到 `NavigationGroup` 中。每组最多 7 项。默认折叠不常用的组

### 输入替换规则
- **1–10 评分行** → 原生 range 滑块（`<input type="range">`），通过 `TextInput::make()->extraInputAttributes(['type' => 'range', 'min' => 1, 'max' => 10, 'step' => 1])`
- **静态选项的长 Select** → 选项 ≤10 个时用 `Radio::make()->inline()->columns(5)`
- **栅格中的布尔开关** → `->inline(false)` 防止标签溢出
- **字段很多的 Repeater** → 如果条目本身有独立意义，考虑提升为 `RelationManager`

### 克制规则（信号优先于噪声）
- **默认用最简标签：** 先用短标签。只在字段意图模糊时才加 `helperText`、`hint` 或 placeholder
- **最多一层引导：** 对直白的输入，不要同时叠加 label + hint + placeholder + description
- **避免图标泛滥：** 在单个页面中，避免给每个分区都加图标。把图标留给顶层 Tab 或高显眼度的分区
- **保留不言自明的默认值：** 如果字段本身已清楚，保持不变
- **复杂度阈值：** 只有当高级 UI 模式能明显降低操作成本（更少点击、更少滚动、更快扫描）时才引入

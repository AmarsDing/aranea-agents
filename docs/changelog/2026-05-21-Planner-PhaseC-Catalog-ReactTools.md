# Planner Phase C — StandardCatalog 余量 + ReAct 工具关联

**日期**：2026-05-21

## A2UI

- `a2uiChildren.ts`：`explicitList` + `template.dataBinding` 子节点解析。
- `A2UIComponentNode` 新增 List、Card、Modal、Tabs、Divider、Video、TextField（只读）、CheckBox（只读）。

## ReAct

- `reactPlannerToolLink.ts`：从 `/*ACTION*/` 正文提取 `functions.*` 等工具名提示；将助手消息之后、下一条实质性助手消息之前的 `chat.activity/v1` 行关联到 ACTION 步骤。
- `ChatReactSteps` 在 ACTION 步骤下内嵌 `ChatExecutionCard`（与 51 `tool_call` envelope 投影一致）。

## 测试

- `a2uiChildren.spec.ts`、`reactPlannerToolLink.spec.ts`

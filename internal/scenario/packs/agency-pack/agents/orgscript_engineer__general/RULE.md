## 🚨 你必须遵守的关键规则
### 严格的语言语义
- OrgScript 不是图灵完备语言；不要像通用编程语言那样对待它。它是一种描述语言。
- 在 v0.1 中仅使用支持的块：`process`、`stateflow`、`rule`、`role`、`policy`、`metric`、`event`。
- 仅使用支持的语句：`when`、`if`、`else`、`then`、`assign`、`transition`、`notify`、`create`、`update`、`require`、`stop`。
- 遵循规范结构，保持严格的缩进和格式。

### 健壮的解析器架构
- 在为语法分析器或 AST 验证器贡献代码时，始终生成稳定的 JSON 诊断代码。
- 在任何 CLI 贡献中维护 CI 友好的退出代码（`0` 表示干净，`1` 表示错误）。
- 以 EBNF 语法作为语法验证的唯一真相源。

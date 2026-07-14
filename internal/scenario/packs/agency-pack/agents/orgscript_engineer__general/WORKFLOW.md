## 🔄 你的工作流程
### 步骤 1：流程分析与语法检查
- 阅读纯文本 SOP 或业务逻辑需求。
- 识别触发器、状态转换、条件、角色和边界。
- 与 `spec/language-spec.md` 和 `grammar.ebnf` 交叉参考以确保语法可行性。

### 步骤 2：实现与代码生成
- 起草 `.orgs` 文件，保持最大的人类可读性。
- 如果在解析器包上工作：更新 `packages/parser` 中的分词器/AST 节点或 `packages/cli` 中的 CLI 处理器。

### 步骤 3：验证与规范格式化
- 运行 `orgscript format <file>` 格式化为规范结构。
- 运行 `orgscript validate <file>` 断言有效语法和 AST 形状。
- 运行 `orgscript check <file>` 确认 lint 和零诊断错误。

### 步骤 4：导出生成
- 通过 `orgscript export mermaid <file>` 和 `orgscript export markdown <file>` 测试下游产物。
- 将生成的 Mermaid 结构嵌入相关文档。

# Testing 测试文档

> AI 存放文件时必须遵守以下规范。

---

## 目录结构

```
docs/testing/
├── README.md                    # 本说明文件
├── test-strategy.md             # 测试策略（分层架构 + 覆盖率目标 + 测试矩阵）
├── test-loop-process.md         # 测试闭环流程（开发→测试→报告→修复→再测试→发布）
├── test-report-template.md      # 测试报告模板
├── test-data/                   # 测试数据（JSON 样例）
│   ├── sample-agent-config.json
│   ├── sample-chat-messages.json
│   ├── sample-error-codes.json
│   ├── sample-graph-definition.json
│   ├── sample-team-config.json
│   ├── sample-test-users.json
│   ├── sample-tool-definitions.json
│   └── sample-webhook-config.json
└── reports/                     # 测试报告存档
    └── report-YYYYMMDD-HHMMSS.md
```

## AI 存放规则

### 测试数据文件（test-data/）
- **命名格式**：`sample-<entity-name>.json`
- **前缀必须**：所有 JSON 文件必须以 `sample-` 开头
- **新增数据**：添加新测试数据时，文件名遵循 `sample-<entity-name>.json` 格式
- **内容要求**：JSON 数据必须是合法的、可被测试代码直接引用的样例数据

### 测试报告（reports/）
- **命名格式**：`report-YYYYMMDD-HHMMSS.md`
- **模板**：使用 `test-report-template.md` 模板生成
- **保留策略**：最近 10 份报告保留，更早的可归档

### 验收报告（reports/）
- **命名格式**：`acceptance-YYYY-MM-DD-<topic>.md`
- **用途**：专项功能的验收测试清单，迭代完成后归档到 reports/

### 策略与流程文档
- `test-strategy.md`：测试策略（测什么、怎么分层、覆盖率目标、测试矩阵）
- `test-loop-process.md`：执行流程（谁在什么时候做什么、AI 自动化规则）
- 两者分工明确，**禁止重复**命令列表——流程文档引用策略文档的章节

### 禁止事项
- 禁止在 test-data/ 中放置非 JSON 文件
- 禁止在根目录放置专项验收文档（应归入 reports/）
- 禁止创建与 test-strategy.md 内容重叠的测试策略文档

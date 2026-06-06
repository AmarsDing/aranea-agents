# Reports 调研报告文档

> 存放项目调研、分析、审计类报告。每份报告独立成文。

---

## 目录结构

```
docs/reports/
├── README.md                                    # 本说明文件
├── 2026-05-31-analysis-competitive.md           # 竞品分析报告
├── 2026-05-31-requirements-competitive-gap.md   # 竞品差距需求清单
├── 2026-05-31-audit-frontend.md                 # 前端审计报告
└── 2026-06-05-audit-archive-completion.md       # 归档完成度审计
```

## AI 存放规则

### 命名格式
**必须**使用 `YYYY-MM-DD-<type>-<topic>.md` 格式：

| 组成 | 说明 | 示例 |
|------|------|------|
| `YYYY-MM-DD` | 报告产出日期 | `2026-05-31` |
| `<type>` | 报告类型 | `analysis` / `audit` / `requirements` / `review` / `research` |
| `<topic>` | 报告主题 | `competitive` / `frontend` / `archive-completion` |

### 报告类型（type）词汇表

| type | 含义 | 用途 |
|------|------|------|
| `analysis` | 分析报告 | 竞品分析、技术选型分析、市场分析 |
| `audit` | 审计报告 | 代码审计、架构审计、完成度审计 |
| `requirements` | 需求报告 | 差距需求、调研需求 |
| `review` | 评审报告 | 设计评审、代码评审总结 |
| `research` | 调研报告 | 技术调研、可行性调研 |

### 文档结构要求
每份报告必须包含：
1. **标题**：`# <type>: <topic>`
2. **元信息**：日期、作者、版本
3. **摘要**：一段话概括核心发现
4. **正文**：结构化内容
5. **结论/建议**：可操作的下一步

### 禁止事项
- 禁止使用日期后缀命名（如 `competitive-analysis-2026-05-31.md`）
- 禁止在文件名中使用空格
- 禁止放置非报告类文档（测试报告放 `docs/testing/reports/`）

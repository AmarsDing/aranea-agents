## 🚀 高级能力
### AI 爬虫分类法

并非所有 AI 爬虫都一样。按用途对它们进行分类以做出明智的访问决策：

| 爬虫 | 运营方 | 用途 | 访问建议 |
|---------|----------|---------|----------------------|
| GPTBot | OpenAI | 训练 + ChatGPT 浏览 | 允许（驱动引用） |
| ClaudeBot | Anthropic | 训练 + Claude 回复 | 允许（驱动引用） |
| PerplexityBot | Perplexity | 实时搜索 + 引用 | 允许（直接流量来源） |
| Google-Extended | Google | Gemini 训练（非搜索） | 业务决策 |
| Applebot-Extended | Apple | Apple Intelligence 功能 | 业务决策 |
| CCBot | Common Crawl | 开放数据集，许多下游用途 | 业务决策 |
| Bytespider | ByteDance | 训练数据采集 | 通常屏蔽 |

### 内容可用性分层

| 层级 | 格式 | AI 可访问性 | 用途 |
|------|--------|-----------------|---------|
| Tier 1 | llms.txt + Markdown 端点 | 最高 —— 直接摄取 | 核心产品页、文档、FAQ |
| Tier 2 | 干净语义化 HTML + schema | 高 —— 易解析 | 博客文章、指南、落地页 |
| Tier 3 | 服务器渲染 HTML（无 JS） | 中 —— 可解析但有噪音 | 动态列表、目录 |
| Tier 4 | JS 渲染 SPA 内容 | 低 —— 需无头渲染 | 仪表板、交互工具 |
| Tier 5 | 仅 PDF 或基于图片 | 极低 —— 有损提取 | 遗留文档（迁移至 Tier 1-2） |

### 跨波次前置条件检查清单

```markdown
### Wave 1（SEO）前置条件
- [ ] robots.txt 允许 Googlebot、Bingbot
- [ ] Sitemap.xml 最新且已提交
- [ ] 页面在无 JavaScript 时可渲染（或使用 SSR/SSG）
- [ ] 所有关键页面有语义化标题层级

### Wave 2（AI 引用）前置条件
- [ ] robots.txt 允许 GPTBot、ClaudeBot、PerplexityBot
- [ ] llms.txt 已发布且最新
- [ ] 关键页面在 token 预算内
- [ ] 合格页面有 FAQPage 和 HowTo schema

### Wave 3（智能体任务完成）前置条件
- [ ] agent-permissions.json 已发布
- [ ] /mcp-actions.json 端点已上线（或已规划）
- [ ] 关键任务流使用原生 HTML 表单（而非仅 JS 控件）
- [ ] 提供访客流（首次交互无需强制认证）
```

### 与互补代理的协作

此代理构建所有三个波次依赖的基础：

- 在 Wave 1 前置条件验证后交接给 **SEO Specialist** —— 他们处理排名、链接建设和内容策略
- 在 Wave 2 前置条件验证后交接给 **AI Citation Strategist** —— 他们处理引用审计、丢失提示分析和修复包
- 与 **Frontend Developer** 配对实施 Markdown 端点、SSR/SSG 迁移和语义化 HTML 清理
- 与 **DevOps Automator** 配对进行 robots.txt 部署、爬取日志监控和 llms.txt 自动重新生成

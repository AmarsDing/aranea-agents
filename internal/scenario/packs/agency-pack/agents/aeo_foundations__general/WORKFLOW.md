## 🔄 工作流程
1. **基础审计**
   - 抓取 robots.txt —— 检查 AI 爬虫指令（GPTBot、ClaudeBot、PerplexityBot、Google-Extended、Applebot-Extended）
   - 检查站点根目录是否存在 llms.txt 和 llms-full.txt
   - 检查 AGENTS.md、agent-permissions.json 和 /mcp-actions.json
   - 审查服务器访问日志中的 AI 爬虫活动和被阻止的请求
   - 为发现层打分（0-6 分）

2. **可解析性评估**
   - 在禁用 JavaScript 的情况下测试关键页面 —— 核心内容是否仍可见？
   - 估算 10-20 个最重要页面的 token 计数
   - 验证标题层级（H1 → H6）是语义化的，而非装饰性的
   - 检查 JS 渲染内容是否有 Markdown 或干净 HTML 替代
   - 验证目标页面的 schema 标记（FAQPage、HowTo、Article、Product）
   - 为可解析性层打分（0-6 分）

3. **能力检查**
   - 验证 agent-permissions.json 是否声明可用操作
   - 检查 WebMCP 发现端点是否存在（Wave 3 就绪）
   - 审查关键任务流是否以机器可读格式声明
   - 为能力层打分（0-3 分）

4. **修复实施**
   - 阶段 1（第 1-3 天）：robots.txt AI 爬虫规则 —— 即时、零风险
   - 阶段 2（第 3-7 天）：llms.txt 和 llms-full.txt —— 为 AI 消费策划站点地图
   - 阶段 3（第 7-14 天）：Token 预算合规 —— 拆分、分块或摘要超预算内容
   - 阶段 4（第 14-21 天）：Schema 标记和结构化内容 —— FAQPage、HowTo、干净 HTML
   - 阶段 5（第 21-30 天）：agent-permissions.json 和能力声明

5. **验证与维护**
   - 实施后重跑基础审计 —— 目标得分 75%+
   - 查询 AI 系统（ChatGPT、Claude、Perplexity）验证内容被摄取
   - 每周检查爬取日志以发现新 AI user agent
   - 安排季度 llms.txt 审查以保持发现文件最新
   - 监控新发现标准，在达到实质性采用时采纳

# Agency-Agents 资源迁移审查清单

> **用途**：供用户审查 agency-agents 资源导入后的公司-部门-岗位(agent)分级结构是否准确。
> **状态**：Phase 5 验证已完成，数据已正确加载到数据库。本清单用于 Phase 2 审查。
> **日期**：2026-07-14

---

## 总览

| 维度 | 数量 | 说明 |
|------|------|------|
| 公司 | 3 | digital_content_media / digital_tech / healthcare |
| 部门 | 26 | 分布在 3 家公司下 |
| 岗位/Agent | 239 | 每个岗位对应 1 个 agent（variant=general） |
| 系统 Agent | 4 | __spirit__ / __memory__ / __skills__ / __system_admin__ |
| 部门主管 Agent | 26 | 每个部门 1 个（auto-generated, variant=dept_lead） |
| Prompt 文件 | 1486 | 1426 agency + ~60 系统 |
| 已中文化 Agent | 140 | 名称+描述均为中文 |
| **待中文化 Agent** | **99** | 名称或描述仍为英文 |

---

## 公司 1：数字内容与媒体传播公司 (digital_content_media)

**图标**：megaphone
**描述**：创意构思→内容创作→视觉设计→媒体发布→付费推广→销售转化→财务→客服的完整业务闭环
**部门数**：10 | **岗位数**：129

### 1.1 创意策划部 (creative_planning)
**描述**：人类学、地理学、历史学、叙事学、心理学、统计学等学术研究支撑创意策划
**岗位数**：6

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | anthropologist | 学术人类学家 | 文化研究、田野调查与人类学视角分析专家 | ✅ |
| 2 | geographer | 学术地理学家 | 空间分析、地理信息与地缘研究专家 | ✅ |
| 3 | historian | 学术历史学家 | 历史分析、史料解读与历史叙事专家 | ✅ |
| 4 | narratologist | 学术叙事学家 | 叙事结构、故事理论与文本分析专家 | ✅ |
| 5 | psychologist | 学术心理学家 | 心理学研究、行为分析与认知科学专家 | ✅ |
| 6 | statistician | Statistician | Expert in quantitative research methodology, experimental design, and statistical inference — pressure-tests claims, designs sound studies, and separates real signal from noise, chance, and bias | ❌ 待翻译 |

### 1.2 品牌设计部 (brand_design)
**描述**：UI/UX设计、品牌守护、视觉叙事等品牌视觉与体验设计
**岗位数**：9

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | brand_guardian | 品牌守护者 | 品牌认知、一致性与品牌定位专家 | ✅ |
| 2 | image_prompt_engineer | 图像提示词工程师 | AI 图像生成提示词、摄影风格指令专家 | ✅ |
| 3 | inclusive_visuals_specialist | 包容性视觉专家 | 多元化呈现、偏见消除与真实 AI 图像生成专家 | ✅ |
| 4 | persona_walkthrough | Persona Walkthrough Specialist | Simulate cognitive walkthroughs of web pages from a defined persona's psychological perspective — captures emotional reactions and rational thought at each scroll position, then delivers structured CRO reports grounded in LIFT, Cialdini, and Fogg frameworks | ❌ 待翻译 |
| 5 | ui_designer | UI 设计师 | 视觉设计、组件库与设计系统专家 | ✅ |
| 6 | ux_architect | 用户体验架构师 | 技术架构、CSS 系统与前端实现指导专家 | ✅ |
| 7 | ux_researcher | 用户体验研究员 | 用户测试、行为分析与可用性研究专家 | ✅ |
| 8 | visual_storyteller | 视觉叙事师 | 视觉叙事、多媒体内容与品牌故事专家 | ✅ |
| 9 | whimsy_injector | 创意注入师 | 品牌个性、微互动与趣味体验设计专家 | ✅ |

### 1.3 内容创作部 (content_creation)
**描述**：内容创作、图书联合、播客、PR传播、邮件策略等多平台内容创作
**岗位数**：13

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | aeo_foundations | AEO Foundations Architect | Expert in AI Engine Optimization infrastructure — implements llms.txt, AI-aware robots.txt, token-budgeted content, structured Markdown availability, and agent discovery files | ❌ 待翻译 |
| 2 | agentic_search_optimizer | Agentic Search Optimizer | Expert in WebMCP readiness and agentic task completion — audits whether AI agents can actually accomplish tasks on your site | ❌ 待翻译 |
| 3 | ai_citation_strategist | AI 引用策略师 | AEO/GEO、AI 推荐可见度与引用审计专家 | ✅ |
| 4 | book_co_author | 图书联合作者 | 思想领导力书籍、代笔写作与出版策略专家 | ✅ |
| 5 | china_market_localization_strategist | China Market Localization Strategist | Full-stack China market localization expert | ❌ 待翻译 |
| 6 | content_creator | 内容创作者 | 多平台内容策略、编辑日历与文案专家 | ✅ |
| 7 | email_strategist | Email Marketing Strategist | Expert email marketing strategist for CRM-driven campaigns, lifecycle automation | ❌ 待翻译 |
| 8 | global_podcast_strategist | Global Podcast Strategist | Expert podcast growth specialist focused on show positioning, audience development | ❌ 待翻译 |
| 9 | multi_platform_publisher | Multi-Platform Publisher | Expert orchestrator for one-click Chinese blog publishing. Routes to 知乎/小红书/CSDN/B站/公众号/掘金 | ❌ 待翻译 |
| 10 | podcast_strategist | 播客策略师 | 播客内容策略与平台运营专家 | ✅ |
| 11 | pr_communications_manager | PR & Communications Manager | Strategic public relations and communications specialist | ❌ 待翻译 |
| 12 | private_domain_operator | 私域运营专家 | 企业微信、私域流量与社群运营专家 | ✅ |
| 13 | video_optimization_specialist | Video Optimization Specialist | Video marketing strategist specializing in YouTube algorithm optimization | ❌ 待翻译 |

### 1.4 媒体运营部 (media_operations)
**描述**：SEO、小红书/B站/抖音/知乎/微博/快手等平台运营与社交媒体策略
**岗位数**：19

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | app_store_optimizer | 应用商店优化专家 | ASO、转化率优化与应用曝光专家 | ✅ |
| 2 | baidu_seo_specialist | 百度 SEO 专家 | 百度优化、中国 SEO 与 ICP 合规专家 | ✅ |
| 3 | bilibili_content_strategist | Bilibili 内容策略师 | B站算法、弹幕文化与 UP 主成长专家 | ✅ |
| 4 | douyin_strategist | 抖音运营策略师 | 抖音平台、短视频营销与算法增长专家 | ✅ |
| 5 | growth_hacker | 增长黑客 | 快速用户获取、病毒循环与实验驱动增长专家 | ✅ |
| 6 | instagram_curator | Instagram 运营专家 | 视觉叙事、社区运营与 Instagram 策略专家 | ✅ |
| 7 | kuaishou_strategist | 快手运营策略师 | 快手平台、老铁生态与下沉市场增长专家 | ✅ |
| 8 | linkedin_content_creator | 领英内容创作者 | 个人品牌、思想领导力与领英专业内容专家 | ✅ |
| 9 | reddit_community_builder | Reddit 社区运营 | 真实互动、价值内容与 Reddit 营销专家 | ✅ |
| 10 | seo_specialist | SEO 专家 | 技术 SEO、内容策略与外链建设专家 | ✅ |
| 11 | short_video_editing_coach | 短视频剪辑教练 | 后期制作、剪辑流程与平台规格优化专家 | ✅ |
| 12 | social_media_strategist | 社交媒体策略师 | 跨平台策略、营销活动与社媒整体规划专家 | ✅ |
| 13 | tiktok_strategist | TikTok 策略专家 | 病毒内容、算法优化与 TikTok 增长专家 | ✅ |
| 14 | twitter_engager | Twitter 运营专家 | 实时互动、思想领导力与推特策略专家 | ✅ |
| 15 | wechat_official_account | 微信公众号运营专家 | 粉丝互动、内容营销与微信公众号策略专家 | ✅ |
| 16 | weibo_strategist | 微博运营策略师 | 微博热搜、话题营销与粉丝互动专家 | ✅ |
| 17 | x_twitter_intelligence_analyst | X/Twitter Intelligence Analyst | Social intelligence specialist for X/Twitter research, trend detection | ❌ 待翻译 |
| 18 | xiaohongshu_specialist | 小红书运营专家 | 生活方式内容、趋势策略与小红书增长专家 | ✅ |
| 19 | zhihu_strategist | 知乎运营专家 | 思想领导力、知识驱动互动与知乎权威建立专家 | ✅ |

### 1.5 付费推广部 (paid_promotion)
**描述**：PPC策略、搜索词分析、程序化购买、付费社交等付费媒体推广
**岗位数**：7（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | auditor | 付费媒体审计师 | 200+ 维度账户审计与竞争对手分析专家 |
| 2 | creative_strategist | 广告创意策略师 | RSA 文案、Meta 创意与 PMax 素材专家 |
| 3 | paid_social_strategist | 付费社交策略师 | Meta/LinkedIn/TikTok 跨平台付费社交专家 |
| 4 | ppc_strategist | 竞价广告策略师 | Google/Microsoft/Amazon 广告、账户结构与出价专家 |
| 5 | programmatic_buyer | 程序化广告购买专家 | GDN、DSP、合作媒体与 ABM 展示广告专家 |
| 6 | search_query_analyst | 搜索词分析师 | 搜索词分析、否定关键词与意图映射专家 |
| 7 | tracking_specialist | 追踪与埋点专家 | GTM、GA4、转化追踪与 CAPI 实施专家 |

### 1.6 跨境电商部 (cross_border_ecommerce)
**描述**：中国电商运营、跨境电商、直播带货等电商运营
**岗位数**：4（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | carousel_growth_engine | 轮播图增长引擎 | TikTok/Instagram 轮播图创作与自动发布专家 |
| 2 | china_ecommerce_operator | 中国电商运营专家 | 淘宝/天猫/拼多多与直播电商运营专家 |
| 3 | cross_border_specialist | 跨境电商专家 | 亚马逊/Shopee/Lazada 与跨境履约全链路专家 |
| 4 | livestream_commerce_coach | 直播带货教练 | 主播培训、直播间优化与转化提升专家 |

### 1.7 销售部 (sales_dept)
**描述**：外呼策略、商机策略、售前工程、提案策略等销售与客户策略
**岗位数**：9

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | account_strategist | 客户策略师 | 拓客留存、QBR 与利益相关者地图专家 | ✅ |
| 2 | coach | 销售教练 | 销售代表成长、通话辅导与管道审查促进专家 | ✅ |
| 3 | deal_strategist | 商机策略师 | MEDDPICC 资格认定、竞争定位与赢单策略专家 | ✅ |
| 4 | discovery_coach | 销售发现教练 | SPIN、Gap Selling 与 Sandler 问题设计专家 | ✅ |
| 5 | engineer | 售前工程师 | 技术演示、POC 范围确定与竞争技术定位专家 | ✅ |
| 6 | offer_lead_gen_strategist | Offer & Lead Gen Strategist | Top-of-funnel architect who designs irresistible offers and lead magnets | ❌ 待翻译 |
| 7 | outbound_strategist | 外呼销售策略师 | 基于信号的精准找客、多渠道序列与 ICP 定位专家 | ✅ |
| 8 | pipeline_analyst | 销售漏斗分析师 | 预测、漏斗健康度、商机速度与 RevOps 专家 | ✅ |
| 9 | proposal_strategist | 提案策略师 | RFP 响应、赢单主题与叙事结构专家 | ✅ |

### 1.8 财务部 (finance_dept)
**描述**：财务追踪、财务分析、FP&A、税务策略、CFO等财务管理
**岗位数**：5（全部待翻译 ❌）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | bookkeeper_controller | Bookkeeper & Controller | Expert bookkeeper and controller specializing in day-to-day accounting operations | ❌ 待翻译 |
| 2 | financial_analyst | Financial Analyst | Expert financial analyst specializing in financial modeling, forecasting | ❌ 待翻译 |
| 3 | fpa_analyst | FP&A Analyst | Expert Financial Planning & Analysis (FP&A) analyst | ❌ 待翻译 |
| 4 | investment_researcher | Investment Researcher | Expert investment researcher specializing in market research, due diligence | ❌ 待翻译 |
| 5 | tax_strategist | Tax Strategist | Expert tax strategist specializing in tax optimization, multi-jurisdictional compliance | ❌ 待翻译 |

### 1.9 客户支持部 (customer_support)
**描述**：客户支持、数据分析、基础设施维护等客户支持与运营
**岗位数**：6（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | analytics_reporter | 数据分析报告员 | 数据分析、仪表板与业务洞察专家 |
| 2 | executive_summary_generator | 高管摘要生成师 | C 级沟通、战略摘要与决策支持专家 |
| 3 | finance_tracker | 财务追踪专员 | 财务规划、预算管理与业务绩效分析专家 |
| 4 | infrastructure_maintainer | 基础设施维护工程师 | 系统可靠性、性能优化与基础设施运营专家 |
| 5 | legal_compliance_checker | 法律合规检查员 | 合规审查、监管要求与风险管理专家 |
| 6 | support_responder | 客户支持专员 | 客户服务、问题解决与支持运营专家 |

### 1.10 专项服务部 (special_services)
**描述**：参谋长、业务策略、变革管理、客户成功、运营、HR、法务、供应链等跨领域专项服务
**岗位数**：51（26 个待翻译 ❌，25 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | accounts_payable_agent | 应付账款 Agent | 支付处理、供应商管理与自主支付专家 | ✅ |
| 2 | agentic_identity_trust | 智能体身份与信任架构师 | Agent 身份、认证与信任验证专家 | ✅ |
| 3 | agents_orchestrator | 多智能体编排师 | 多 Agent 协调、工作流管理与复杂项目统筹专家 | ✅ |
| 4 | automation_governance_architect | 自动化治理架构师 | 自动化治理、n8n 与工作流审计专家 | ✅ |
| 5 | business_strategist | Business Strategist | Senior management consulting specialist for competitive analysis, market entry strategy | ❌ 待翻译 |
| 6 | change_management_consultant | Change Management Consultant | Expert change management specialist using ADKAR, Kotter, and Prosci frameworks | ❌ 待翻译 |
| 7 | chief_financial_officer | Chief Financial Officer | Strategic finance executive who governs capital allocation, treasury operations | ❌ 待翻译 |
| 8 | chief_of_staff | Chief of Staff | Master coordinator for founders and executives | ❌ 待翻译 |
| 9 | civil_engineer | Civil Engineer | Expert civil and structural engineer with global standards coverage | ❌ 待翻译 |
| 10 | corporate_training_designer | 企业培训设计师 | 企业培训、课程开发与学习系统设计专家 | ✅ |
| 11 | cultural_intelligence_strategist | 文化智能策略师 | 全球 UX、多元呈现与文化排斥规避专家 | ✅ |
| 12 | customer_service | Customer Service | Friendly, professional customer service specialist for any industry | ❌ 待翻译 |
| 13 | customer_success_manager | Customer Success Manager | Strategic customer success specialist for onboarding, health scoring, QBR | ❌ 待翻译 |
| 14 | data_consolidation_agent | 数据整合 Agent | 销售数据聚合与仪表板报告专家 | ✅ |
| 15 | developer_advocate | 开发者布道师 | 社区建设、开发者体验与技术内容创作专家 | ✅ |
| 16 | document_generator | 文档生成专家 | PDF/PPTX/DOCX/XLSX 代码生成与专业文档创建专家 | ✅ |
| 17 | french_consulting_market | 法国咨询市场导航师 | ESN/SI 生态与法国 IT 自由职业专家 | ✅ |
| 18 | government_digital_presales_consultant | 政务数字化售前顾问 | ToG 项目售前与数字政府转型方案专家 | ✅ |
| 19 | grant_writer | Grant Writer | Expert grant writing specialist for nonprofits, research institutions | ❌ 待翻译 |
| 20 | healthcare_customer_service | Healthcare Customer Service | Empathetic healthcare customer service specialist | ❌ 待翻译 |
| 21 | healthcare_marketing_compliance | 医疗营销合规专家 | 中国医疗广告法规合规专家 | ✅ |
| 22 | hospitality_guest_services | Hospitality Guest Services | Comprehensive hospitality guest services specialist | ❌ 待翻译 |
| 23 | hr_onboarding | HR Onboarding | Comprehensive HR onboarding specialist for employee orientation | ❌ 待翻译 |
| 24 | identity_graph_operator | 身份图谱运营专家 | 多 Agent 系统实体去重与身份一致性专家 | ✅ |
| 25 | korean_business_navigator | 韩国商务导航师 | 韩国商业文化、品议流程与人际关系机制专家 | ✅ |
| 26 | language_translator | Language Translator | Real-time Spanish ↔ English translation specialist | ❌ 待翻译 |
| 27 | legal_billing_time_tracking | Legal Billing & Time Tracking | Comprehensive legal billing and time tracking specialist | ❌ 待翻译 |
| 28 | legal_client_intake | Legal Client Intake | Comprehensive legal client intake specialist | ❌ 待翻译 |
| 29 | legal_document_review | Legal Document Review | Comprehensive legal document review specialist | ❌ 待翻译 |
| 30 | loan_officer_assistant | Loan Officer Assistant | Comprehensive loan officer assistant for mortgage and lending professionals | ❌ 待翻译 |
| 31 | lsp_index_engineer | 语言服务器/索引工程师 | LSP 实现、代码智能与语义索引专家 | ✅ |
| 32 | ma_integration_manager | M&A Integration Manager | Mergers and acquisitions integration specialist | ❌ 待翻译 |
| 33 | mcp_builder | MCP 构建专家 | Model Context Protocol 服务器与 AI Agent 工具链专家 | ✅ |
| 34 | medical_billing_coding_specialist | Medical Billing & Coding Specialist | Expert medical billing and coding specialist | ❌ 待翻译 |
| 35 | model_qa | 模型 QA 专家 | ML 审计、特征分析与可解释性专家 | ✅ |
| 36 | operations_manager | Operations Manager | Business operations specialist who applies Lean, Six Sigma | ❌ 待翻译 |
| 37 | organizational_psychologist | Organizational Psychologist | Applied organizational psychologist who diagnoses team dynamics | ❌ 待翻译 |
| 38 | personal_growth_mentor | Personal Growth Mentor | Cross-domain personal development mentor | ❌ 待翻译 |
| 39 | pricing_analyst | Pricing Analyst | Specialized pricing analyst who develops optimal pricing models | ❌ 待翻译 |
| 40 | real_estate_buyer_seller | Real Estate Buyer & Seller | Comprehensive real estate agent assistant | ❌ 待翻译 |
| 41 | recruitment_specialist | 招聘专家 | 人才获取、招聘运营与雇主品牌专家 | ✅ |
| 42 | report_distribution_agent | 报告分发 Agent | 自动化报告交付与按区域定时发送专家 | ✅ |
| 43 | retail_customer_returns | Retail Customer Returns | Comprehensive retail customer returns specialist | ❌ 待翻译 |
| 44 | sales_data_extraction_agent | 销售数据提取 Agent | Excel 监控与销售指标提取（MTD/YTD）专家 | ✅ |
| 45 | sales_outreach | Sales Outreach | Consultative B2B sales outreach specialist | ❌ 待翻译 |
| 46 | salesforce_architect | Salesforce 架构师 | 多云 Salesforce 设计、Governor Limits 与集成专家 | ✅ |
| 47 | strategy_duel_agent | Strategy Duel Agent | Conducts live strategy duels using game theory and the 36 Chinese stratagems | ❌ 待翻译 |
| 48 | study_abroad_advisor | 留学顾问 | 国际教育、申请规划与留学目的地专家（美/英/加/澳） | ✅ |
| 49 | supply_chain_strategist | 供应链策略师 | 供应链管理、采购策略与优化专家 | ✅ |
| 50 | workflow_architect | 工作流架构师 | 工作流发现、流程映射与规格说明专家 | ✅ |
| 51 | zk_steward | 知识卡片管理员 | 知识管理、Zettelkasten 与笔记系统专家 | ✅ |

---

## 公司 2：软件工程与数字科技产品公司 (digital_tech)

**图标**：code
**描述**：产品规划→项目管理→架构设计→开发实现→测试→部署运维→安全保障→合规审计的完整工程闭环
**部门数**：13 | **岗位数**：107

### 2.1 产品部 (product_dept)
**描述**：产品经理、趋势研究、反馈综合等产品规划与管理
**岗位数**：5（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | behavioral_nudge_engine | 行为助推引擎 | 行为心理学、助推设计与用户激励专家 |
| 2 | feedback_synthesizer | 用户反馈综合分析师 | 用户反馈分析、洞察提取与产品优先级专家 |
| 3 | manager | 产品经理 | 全生命周期产品管理：发现、PRD、路线图、GTM |
| 4 | sprint_prioritizer | Sprint 优先级规划师 | 敏捷规划、功能优先级与 Sprint 管理专家 |
| 5 | trend_researcher | 市场趋势研究员 | 市场情报、竞品分析与机会识别专家 |

### 2.2 项目管理部 (project_management)
**描述**：制片人、项目协调、高级项目经理等项目全生命周期管理
**岗位数**：7

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | experiment_tracker | 实验追踪专家 | A/B 测试、假设验证与数据驱动决策专家 | ✅ |
| 2 | jira_workflow_steward | Jira 工作流管理员 | Git 工作流、分支策略与 Jira 关联交付规范专家 | ✅ |
| 3 | meeting_notes_specialist | Meeting Notes Specialist | Extract structured decisions, action items from meeting transcripts | ❌ 待翻译 |
| 4 | project_manager_senior | 高级项目经理 | 现实范围评估与规格转任务分解专家 | ✅ |
| 5 | project_shepherd | 项目协调专家 | 跨职能协调、时间轴管理与端到端项目统筹专家 | ✅ |
| 6 | studio_operations | 工作室运营专家 | 日常效率优化、流程改进与生产支持专家 | ✅ |
| 7 | studio_producer | 工作室制作人 | 高层编排、投资组合管理与多项目监督专家 | ✅ |

### 2.3 后端开发部 (backend_dev)
**描述**：后端架构、API平台、数据库、数据工程、CMS、搜索、支付、AI等后端开发
**岗位数**：24（15 个待翻译 ❌，9 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | ai_data_remediation_engineer | AI 数据修复工程师 | 自愈数据管道、离线 SLM 与语义聚类专家 | ✅ |
| 2 | ai_engineer | AI 工程师 | 机器学习模型部署、AI 集成与数据管道专家 | ✅ |
| 3 | api_platform_engineer | API Platform Engineer | Expert API platform engineer for public and partner APIs | ❌ 待翻译 |
| 4 | autonomous_optimization_architect | 自主优化架构师 | LLM 路由、成本优化与影子测试专家 | ✅ |
| 5 | backend_architect | 后端架构师 | 负责 API 设计、数据库架构与可扩展性的后端系统专家 | ✅ |
| 6 | cms_developer | CMS Developer | Drupal and WordPress specialist for theme development | ❌ 待翻译 |
| 7 | data_engineer | 数据工程师 | 数据管道、湖仓架构与 ETL/ELT 专家 | ✅ |
| 8 | database_optimizer | 数据库优化工程师 | Schema 设计、查询优化与索引策略专家（PostgreSQL/MySQL） | ✅ |
| 9 | drupal_performance | Drupal Performance Engineer | Expert Drupal 10/11 performance engineer | ❌ 待翻译 |
| 10 | drupal_shopping_cart | Drupal Shopping Cart Engineer | Expert Drupal e-commerce engineer | ❌ 待翻译 |
| 11 | email_intelligence_engineer | Email Intelligence Engineer | Expert in extracting structured data from raw email threads | ❌ 待翻译 |
| 12 | embedded_firmware_engineer | 嵌入式固件工程师 | 裸金属、RTOS、ESP32/STM32/Nordic 固件开发专家 | ✅ |
| 13 | feishu_integration_developer | 飞书集成开发工程师 | 飞书/Lark 开放平台、机器人与工作流集成专家 | ✅ |
| 14 | filament_optimization_specialist | Filament Optimization Specialist | Expert in restructuring and optimizing Filament PHP admin interfaces | ❌ 待翻译 |
| 15 | multi_agent_systems_architect | Multi-Agent Systems Architect | Systems architect specializing in multi-agent AI pipelines | ❌ 待翻译 |
| 16 | orgscript_engineer | OrgScript Engineer | Expert in designing, parsing, and implementing OrgScript grammar | ❌ 待翻译 |
| 17 | payments_billing_engineer | Payments & Billing Engineer | Expert payments engineer for PSP integrations | ❌ 待翻译 |
| 18 | prompt_engineer | Prompt Engineer | Specialist in crafting, testing, and optimizing prompts for LLMs | ❌ 待翻译 |
| 19 | realtime_collaboration_engineer | Realtime Collaboration Engineer | Expert realtime systems engineer for WebSocket/SSE infrastructure | ❌ 待翻译 |
| 20 | search_relevance_engineer | Search Relevance Engineer | Expert search engineer for Elasticsearch and OpenSearch | ❌ 待翻译 |
| 21 | solidity_smart_contract_engineer | Solidity 智能合约工程师 | EVM 合约、Gas 优化与 DeFi 协议专家 | ✅ |
| 22 | video_streaming_engineer | Video Streaming Engineer | Expert video streaming engineer for adaptive bitrate delivery | ❌ 待翻译 |
| 23 | wordpress_performance | WordPress Performance Engineer | Expert WordPress performance engineer | ❌ 待翻译 |
| 24 | wordpress_shopping_cart | WordPress Shopping Cart Engineer | Expert WordPress e-commerce engineer specializing in WooCommerce | ❌ 待翻译 |

### 2.4 前端开发部 (frontend_dev)
**描述**：前端、桌面应用、WebAssembly、小程序等前端与客户端开发
**岗位数**：7（5 个待翻译 ❌，2 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | desktop_app_engineer | Desktop App Engineer | Expert desktop application engineer for Electron and Tauri | ❌ 待翻译 |
| 2 | frontend_developer | 前端开发工程师 | 专注现代 Web 技术、React/Vue/Angular 框架、UI 实现与性能优化的前端专家 | ✅ |
| 3 | i18n_engineer | Internationalization Engineer | Expert i18n engineer for ICU MessageFormat, CLDR plural rules | ❌ 待翻译 |
| 4 | section_508_specialist | Section 508 Accessibility Specialist | Expert U.S. federal Section 508 accessibility engineer | ❌ 待翻译 |
| 5 | uswds_developer | USWDS Developer | Expert U.S. Web Design System frontend developer | ❌ 待翻译 |
| 6 | webassembly_engineer | WebAssembly Engineer | Expert WebAssembly engineer — compiling Rust/C++/Go to Wasm | ❌ 待翻译 |
| 7 | wechat_mini_program_developer | 微信小程序开发工程师 | 微信生态、小程序与支付集成开发专家 | ✅ |

### 2.5 移动开发部 (mobile_dev)
**描述**：移动应用构建、移动发布、语音AI集成等移动端开发
**岗位数**：3（2 个待翻译 ❌，1 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | mobile_app_builder | 移动端开发工程师 | iOS/Android、React Native、Flutter 跨平台移动应用构建者 | ✅ |
| 2 | mobile_release_engineer | Mobile Release Engineer | Expert mobile release and distribution engineer for iOS and Android | ❌ 待翻译 |
| 3 | voice_ai_integration_engineer | Voice AI Integration Engineer | Expert in building end-to-end speech transcription pipelines | ❌ 待翻译 |

### 2.6 游戏开发部 (game_dev)
**描述**：游戏设计、Unity/Unreal/Godot/Blender/Roblox等游戏引擎开发
**岗位数**：5（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | game_audio_engineer | 游戏音频工程师 | FMOD/Wwise、自适应音乐与空间音频专家 |
| 2 | game_designer | 游戏设计师 | 系统设计、GDD 写作、经济平衡与玩法循环专家 |
| 3 | level_designer | 关卡设计师 | 布局理论、节奏、遭遇设计与环境叙事专家 |
| 4 | narrative_designer | 叙事设计师 | 故事系统、分支对话与世界观架构专家 |
| 5 | technical_artist | 技术美术 | Shader、VFX、LOD 管线与美术到引擎优化专家 |

### 2.7 空间计算部 (spatial_computing)
**描述**：XR架构、visionOS、macOS Metal等空间计算与沉浸式体验开发
**岗位数**：6（全部已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 |
|---|---------|---------|------|
| 1 | macos_spatial_metal_engineer | macOS 空间/Metal 工程师 | Swift、Metal 与高性能 3D macOS 空间计算专家 |
| 2 | terminal_integration_specialist | 终端集成专家 | 终端集成、命令行工具与开发者工作流专家 |
| 3 | visionos_spatial_engineer | visionOS 空间工程师 | Apple Vision Pro 应用与空间计算体验开发专家 |
| 4 | xr_cockpit_interaction_specialist | XR 座舱交互专家 | 座舱控制系统与沉浸式控制界面专家 |
| 5 | xr_immersive_developer | WebXR 沉浸式开发者 | WebXR、浏览器端 AR/VR 沉浸式体验开发专家 |
| 6 | xr_interface_architect | XR 界面架构师 | 空间交互设计与沉浸式 UX 专家（AR/VR/XR） |

### 2.8 质量保障部 (quality_assurance)
**描述**：测试自动化、性能基准、API测试、无障碍审计等质量保障
**岗位数**：9

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | accessibility_auditor | 无障碍审计师 | WCAG 审计、辅助技术测试与包容性设计专家 | ✅ |
| 2 | api_tester | API 测试工程师 | API 验证、集成测试与端点核查专家 | ✅ |
| 3 | evidence_collector | 测试证据采集员 | 截图 QA、视觉验证与 Bug 文档专家 | ✅ |
| 4 | performance_benchmarker | 性能基准测试专家 | 性能测试、压力测试与速度优化专家 | ✅ |
| 5 | reality_checker | 生产就绪验证员 | 基于证据的认证、质量门与发布认证专家 | ✅ |
| 6 | test_automation_engineer | Test Automation Engineer | Expert end-to-end test automation engineer for Playwright and Cypress | ❌ 待翻译 |
| 7 | test_results_analyzer | 测试结果分析师 | 测试评估、质量指标分析与覆盖率报告专家 | ✅ |
| 8 | tool_evaluator | 工具评估专家 | 技术评估与工具选型专家 | ✅ |
| 9 | workflow_optimizer | 工作流优化专家 | 流程分析、工作流改进与自动化机会挖掘专家 | ✅ |

### 2.9 运维部 (ops)
**描述**：SRE、DevOps自动化、网络工程、故障响应、IT服务等运维与可靠性
**岗位数**：8（5 个待翻译 ❌，3 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | codebase_onboarding_engineer | Codebase Onboarding Engineer | Expert developer onboarding specialist | ❌ 待翻译 |
| 2 | devops_automator | DevOps 自动化工程师 | CI/CD、基础设施自动化与云运营专家 | ✅ |
| 3 | finops_engineer | FinOps Engineer | Expert cloud cost engineer for AWS/GCP/Azure | ❌ 待翻译 |
| 4 | identity_access_engineer | Identity & Access Engineer | Expert identity engineer for OAuth 2.0/OIDC flows | ❌ 待翻译 |
| 5 | incident_response_commander | 故障响应指挥官 | 事件管理、故障复盘与值班应急专家 | ✅ |
| 6 | it_service_manager | IT Service Manager | Expert IT service management specialist using ITIL 4 framework | ❌ 待翻译 |
| 7 | network_engineer | Network Engineer | Expert network engineer for Cisco IOS/IOS-XE, Juniper Junos | ❌ 待翻译 |
| 8 | sre | 站点可靠性工程师 | SLO、错误预算、可观测性与混沌工程专家 | ✅ |

### 2.10 架构部 (architecture)
**描述**：软件架构、代码审查、技术文档、Git工作流等架构与工程规范
**岗位数**：7

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | code_reviewer | 代码审查工程师 | 建设性代码审查、安全与可维护性评估专家 | ✅ |
| 2 | git_workflow_master | Git 工作流专家 | 分支策略、规范提交与高级 Git 操作专家 | ✅ |
| 3 | minimal_change_engineer | Minimal Change Engineer | Engineering specialist focused on minimum-viable diffs | ❌ 待翻译 |
| 4 | rapid_prototyper | 快速原型工程师 | 快速 POC 开发、MVP 与迭代验证专家 | ✅ |
| 5 | senior_developer | 高级开发工程师 | Laravel/Livewire、复杂模式与架构决策专家 | ✅ |
| 6 | software_architect | 软件架构师 | 系统设计、DDD、架构模式与权衡分析专家 | ✅ |
| 7 | technical_writer | 技术文档工程师 | 开发者文档、API 参考手册与教程撰写专家 | ✅ |

### 2.11 安全部 (security_dept)
**描述**：安全架构、渗透测试、云安全、威胁检测等信息安全
**岗位数**：10（7 个待翻译 ❌，3 个已中文化 ✅）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | appsec_engineer | Application Security Engineer | AppSec specialist who secures the software development lifecycle | ❌ 待翻译 |
| 2 | architect | Security Architect | Expert security architect specializing in threat modeling | ❌ 待翻译 |
| 3 | blockchain_security_auditor | 区块链安全审计师 | 智能合约审计与漏洞分析专家 | ✅ |
| 4 | cloud_security_architect | Cloud Security Architect | Cloud-native security specialist designing zero trust architectures | ❌ 待翻译 |
| 5 | compliance_auditor | 合规审计师 | SOC2/ISO27001/HIPAA/PCI-DSS 合规认证指导专家 | ✅ |
| 6 | incident_responder | Incident Responder | Digital forensics and incident response specialist | ❌ 待翻译 |
| 7 | penetration_tester | Penetration Tester | Offensive security specialist conducting authorized penetration tests | ❌ 待翻译 |
| 8 | senior_secops | Senior SecOps Engineer | Defensive application security specialist | ❌ 待翻译 |
| 9 | threat_detection_engineer | 威胁检测工程师 | SIEM 规则、威胁狩猎与 ATT&CK 映射专家 | ✅ |
| 10 | threat_intelligence_analyst | Threat Intelligence Analyst | Cyber threat intelligence specialist | ❌ 待翻译 |

### 2.12 合规审计部 (compliance_audit)
**描述**：数据隐私、ESG可持续、FedRAMP合规等合规与审计
**岗位数**：3（全部待翻译 ❌）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | data_privacy_officer | Data Privacy Officer | Corporate data privacy specialist and DPO | ❌ 待翻译 |
| 2 | esg_sustainability_officer | ESG & Sustainability Officer | Corporate sustainability strategist and ESG reporting specialist | ❌ 待翻译 |
| 3 | fedramp_rmf_compliance | FedRAMP & RMF Compliance Engineer | Expert FedRAMP and NIST Risk Management Framework compliance engineer | ❌ 待翻译 |

### 2.13 GIS解决方案部 (gis_solutions)
**描述**：GIS分析、空间数据工程、GeoAI等地理信息系统解决方案
**岗位数**：13（全部待翻译 ❌）

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | 3d_scene_developer | 3D & Scene Developer | Web 3D visualization specialist | ❌ 待翻译 |
| 2 | analyst | GIS Analyst | Day-to-day GIS operator who creates maps, manages layers | ❌ 待翻译 |
| 3 | bim_specialist | BIM/GIS Specialist | Integration specialist who bridges BIM and GIS | ❌ 待翻译 |
| 4 | cartography_designer | Cartography Designer | Map aesthetics specialist | ❌ 待翻译 |
| 5 | drone_reality_mapping | Drone/Reality Mapping Specialist | Photogrammetry and reality capture expert | ❌ 待翻译 |
| 6 | geoai_ml_engineer | GeoAI/ML Engineer | Geospatial machine learning specialist | ❌ 待翻译 |
| 7 | geoprocessing_specialist | Geoprocessing Specialist | ArcPy and Python toolbox expert | ❌ 待翻译 |
| 8 | qa_engineer | GIS QA Engineer | Quality assurance specialist who validates geospatial data integrity | ❌ 待翻译 |
| 9 | solution_engineer | Solution Engineer | Hands-on GIS prototype builder | ❌ 待翻译 |
| 10 | spatial_data_engineer | Spatial Data Engineer | ETL specialist who transforms messy geospatial data | ❌ 待翻译 |
| 11 | spatial_data_scientist | Spatial Data Scientist | Advanced spatial analytics specialist | ❌ 待翻译 |
| 12 | technical_consultant | Technical Consultant | Strategic GIS advisor | ❌ 待翻译 |
| 13 | web_gis_developer | Web GIS Developer | Full-stack web GIS engineer | ❌ 待翻译 |

---

## 公司 3：医疗公司 (healthcare)

**图标**：heart-pulse
**描述**：临床证据→医疗创新→主权健康系统的专业医疗服务
**部门数**：3 | **岗位数**：3（全部待翻译 ❌）

### 3.1 临床证据部 (clinical_evidence)
**描述**：临床证据agent，提供循证医学证据支撑
**岗位数**：1

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | clinical_evidence_agent | Clinical Evidence Agent | Evidence standards and clinical credibility framework for AI agents | ❌ 待翻译 |

### 3.2 医疗创新部 (medical_innovation)
**描述**：医疗创新策略师，推动医疗技术与模式创新
**岗位数**：1

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | innovation_strategist | Healthcare Innovation Strategist | Strategic narrative architect for healthcare founders | ❌ 待翻译 |

### 3.3 主权健康部 (sovereign_health)
**描述**：主权健康系统agent，支撑国家健康体系建设
**岗位数**：1

| # | 岗位 key | 岗位名称 | 描述 | 中文状态 |
|---|---------|---------|------|---------|
| 1 | sovereign_health_systems_agent | Sovereign Health Systems Agent | Government health mandate engagement framework for AI agents | ❌ 待翻译 |

---

## 审查要点

请审查以下内容是否准确：

### 1. 公司层级
- [ ] 3 家公司名称和描述是否准确？
- [ ] 公司间的业务边界划分是否合理？

### 2. 部门层级
- [ ] 26 个部门名称和描述是否准确？
- [ ] 部门归属的公司是否正确？
- [ ] 是否有遗漏或多余的部门？

### 3. 岗位/Agent 层级
- [ ] 239 个岗位名称是否准确？
- [ ] 岗位描述是否准确反映职责？
- [ ] 岗位归属的部门是否正确？
- [ ] 是否有遗漏或多余的岗位？

### 4. 中文翻译状态
- [ ] 140 个已中文化的 agent 名称/描述是否准确？
- [ ] 99 个待翻译的 agent 是否需要保留英文专业术语？
- [ ] 翻译优先级如何安排？

---

## 下一步计划

待您审查通过后，将执行 Phase 3b：

1. **翻译 99 个 agent 的名称和描述**为中文
2. **翻译 1426 个 prompt 文件**内容为中文
3. 重新生成 pack 并导入数据库
4. 再次验证

---

## 附：数据库验证结果（Phase 5 已完成）

```
organizations_by_level | company | 3
organizations_by_level | department | 26
organizations_by_level | position | 239
agents_by_kind | ecosystem_preset | 239
agents_by_kind | system_builtin | 30 (4 系统 + 26 部门主管)
agents_by_variant | general | 239
agents_by_variant | dept_lead | 26
agent_prompt_files_total | all | 1486
orphan_positions | 0
schema_migrations | 4 个迁移全部应用
```

# Agency-Agents 中文版项目分析报告：行业/部门/岗位/Agent 四级分类

> 来源：[agency-agents-zh](https://github.com/jnMetaCode/agency-agents-zh)
> 基于 [agency-agents](https://github.com/msitarzewski/agency-agents) 翻译并本土化
> 分析日期：2026-06-07

---

## 一、项目概览

| 维度 | 数据 |
|------|------|
| AI 智能体总数 | 215 |
| 英文版翻译 | 165（76.7%） |
| 中国市场原创 | 50（23.3%） |
| 部门分类 | 17 个 |

项目定位：一套开箱即用的 AI 角色库，每个智能体都有独立的身份定义、专业流程和可交付成果，而非通用提示词模板。

---

## 二、行业 → 部门 → 岗位 → Agent 四级分类

### 2.1 互联网/软件行业

#### 工程部（35 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 前端开发 | `engineering-frontend-developer` | 前端开发者 | React/Vue/Angular、UI 实现、性能优化 | 翻译 |
| 后端开发 | `engineering-backend-architect` | 后端架构师 | 可扩展系统设计、数据库架构、API 开发 | 翻译 |
| AI/ML | `engineering-ai-engineer` | AI 工程师 | ML 模型开发与部署、RAG/Agent 应用 | 翻译 |
| DevOps | `engineering-devops-automator` | DevOps 自动化师 | CI/CD、基础设施自动化、容器编排 | 翻译 |
| 安全 | `engineering-security-engineer` | 安全工程师 | 威胁建模、代码审计、安全架构 | 翻译 |
| 全栈 | `engineering-rapid-prototyper` | 快速原型师 | MVP 快速构建、全栈开发 | 翻译 |
| 全栈 | `engineering-senior-developer` | 高级开发者 | Laravel/Livewire/FluxUI、Three.js | 翻译 |
| 移动开发 | `engineering-mobile-app-builder` | 移动应用开发者 | iOS/Android 原生、跨平台框架 | 翻译 |
| 数据工程 | `engineering-data-engineer` | 数据工程师 | ETL/ELT、数据湖、Spark/dbt | 翻译 |
| 技术写作 | `engineering-technical-writer` | 技术文档工程师 | API 文档、开发者文档 | 翻译 |
| 架构 | `engineering-software-architect` | 软件架构师 | 系统设计、DDD、架构决策 | 翻译 |
| SRE | `engineering-sre` | SRE | SLO、可观测性、混沌工程 | 翻译 |
| 代码审查 | `engineering-code-reviewer` | 代码审查员 | 代码审查、安全审计、质量把关 | 翻译 |
| 数据库 | `engineering-database-optimizer` | 数据库优化师 | Schema 设计、查询优化、索引策略 | 翻译 |
| Git | `engineering-git-workflow-master` | Git 工作流大师 | 分支策略、约定式提交、变基 | 翻译 |
| 智能运维 | `engineering-autonomous-optimization-architect` | 自主优化架构师 | 自适应系统、影子测试、财务护栏 | 翻译 |
| 故障响应 | `engineering-incident-response-commander` | 故障响应指挥官 | 故障管理、SLO/SLI、事后复盘 | 翻译 |
| 威胁检测 | `engineering-threat-detection-engineer` | 威胁检测工程师 | SIEM 规则、MITRE ATT&CK、威胁狩猎 | 翻译 |
| AI 数据 | `engineering-ai-data-remediation-engineer` | AI 数据修复工程师 | 自愈数据管道、SLM 语义聚类 | 翻译 |
| CMS | `engineering-cms-developer` | CMS 开发者 | Drupal/WordPress、主题/插件开发 | 翻译 |
| 邮件 | `engineering-email-intelligence-engineer` | 邮件智能工程师 | 邮件解析、结构化提取 | 翻译 |
| PHP | `engineering-filament-optimization-specialist` | Filament 优化专家 | Filament PHP 后台重构 | 翻译 |
| 代码入职 | `engineering-codebase-onboarding-engineer` | 代码库入职引导工程师 | 新人代码库理解、代码路径追踪 | 翻译 |
| 最小变更 | `engineering-minimal-change-engineer` | 最小变更工程师 | 最小差异修复、范围控制 | 翻译 |
| 语音 AI | `engineering-voice-ai-integration-engineer` | 语音 AI 集成工程师 | Whisper、ASR、语音转录流水线 | 翻译 |
| Web3 | `engineering-solidity-smart-contract-engineer` | Solidity 智能合约工程师 | EVM、Gas 优化、DeFi 协议 | 翻译 |
| 微信生态 | `engineering-wechat-mini-program-developer` | 微信小程序开发者 | WXML/WXSS、微信支付、云开发 | 原创 |
| 飞书生态 | `engineering-feishu-integration-developer` | 飞书集成开发工程师 | 飞书机器人、审批流、多维表格 | 原创 |
| 钉钉生态 | `engineering-dingtalk-integration-developer` | 钉钉集成开发工程师 | 钉钉机器人、酷应用、审批流 | 原创 |

#### 设计部（8 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| UI 设计 | `design-ui-designer` | UI 设计师 | 设计系统、组件库、Design Token | 翻译 |
| UX 研究 | `design-ux-researcher` | UX 研究员 | 用户行为分析、可用性测试 | 翻译 |
| UX 架构 | `design-ux-architect` | UX 架构师 | CSS 架构、布局框架、信息架构 | 翻译 |
| 品牌 | `design-brand-guardian` | 品牌守护者 | 品牌策略、一致性维护 | 翻译 |
| AI 图像 | `design-image-prompt-engineer` | 图像提示词工程师 | AI 图像生成、提示词优化 | 翻译 |
| 视觉叙事 | `design-visual-storyteller` | 视觉叙事师 | 数据可视化、品牌叙事 | 翻译 |
| 趣味设计 | `design-whimsy-injector` | 趣味注入师 | 微交互、彩蛋、趣味元素 | 翻译 |
| 包容性设计 | `design-inclusive-visuals-specialist` | 包容性视觉专家 | 消除 AI 图像偏见、多元化视觉 | 翻译 |

#### 产品部（5 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 产品管理 | `product-manager` | 产品经理 | 需求发现、PRD、路线图、GTM | 翻译 |
| 敏捷 | `product-sprint-prioritizer` | Sprint 排序师 | 需求优先级、Sprint 规划 | 翻译 |
| 趋势研究 | `product-trend-researcher` | 趋势研究员 | 行业趋势、技术前瞻 | 翻译 |
| 反馈分析 | `product-feedback-synthesizer` | 反馈分析师 | 用户反馈收集、洞察提炼 | 翻译 |
| 行为设计 | `product-behavioral-nudge-engine` | 行为助推引擎 | 行为心理学、用户引导 | 翻译 |

#### 测试部（9 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| API 测试 | `testing-api-tester` | API 测试员 | API 全链路测试、自动化框架 | 翻译 |
| 性能测试 | `testing-performance-benchmarker` | 性能基准师 | 性能测试、容量规划 | 翻译 |
| 无障碍 | `testing-accessibility-auditor` | 无障碍审核员 | WCAG 审核、辅助技术测试 | 翻译 |
| 质量认证 | `testing-reality-checker` | 现实检验者 | 证据驱动认证、生产就绪评估 | 翻译 |
| 证据收集 | `testing-evidence-collector` | 证据收集者 | 测试证据链、视觉验证 | 翻译 |
| 结果分析 | `testing-test-results-analyzer` | 测试结果分析师 | 质量度量、趋势分析 | 翻译 |
| 工具选型 | `testing-tool-evaluator` | 工具评估师 | 工具评测、功能对比 | 翻译 |
| 流程优化 | `testing-workflow-optimizer` | 工作流优化师 | 流程分析、自动化 | 翻译 |
| 嵌入式测试 | `testing-embedded-qa-engineer` | 嵌入式测试工程师 | HIL 测试、固件自动化、EMC | 原创 |

#### 项目管理部（6 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 高级 PM | `project-manager-senior` | 高级项目经理 | 需求拆解、范围管控 | 翻译 |
| 项目协调 | `project-management-project-shepherd` | 项目牧羊人 | 跨部门协调、时间线管理 | 翻译 |
| 实验管理 | `project-management-experiment-tracker` | 实验追踪员 | A/B 测试、数据驱动决策 | 翻译 |
| 制片管理 | `project-management-studio-producer` | 工作室制片人 | 创意项目统筹、资源调度 | 翻译 |
| 运营管理 | `project-management-studio-operations` | 工作室运营 | 日常效率、流程优化 | 翻译 |
| Jira | `project-management-jira-workflow-steward` | Jira 工作流管家 | Jira 配置、Git 工作流 | 翻译 |

---

### 2.2 嵌入式/物联网/工业行业

#### 工程部 — 嵌入式/工业方向（6 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 嵌入式固件 | `engineering-embedded-firmware-engineer` | 嵌入式固件工程师 | RTOS、外设驱动、低功耗设计 | 翻译 |
| 嵌入式 Linux | `engineering-embedded-linux-driver-engineer` | 嵌入式 Linux 驱动工程师 | 内核模块、设备树、Platform/I2C/SPI | 原创 |
| FPGA | `engineering-fpga-digital-design-engineer` | FPGA/ASIC 数字设计工程师 | Verilog/SystemVerilog、时序收敛、AXI | 原创 |
| IoT 架构 | `engineering-iot-solution-architect` | IoT 方案架构师 | MQTT/CoAP、边缘计算、设备管理 | 原创 |
| 上位机 | `engineering-pc-host-engineer` | 上位机工程师 | Qt/QML、Modbus/CAN、实时可视化 | 原创 |
| 机械设计 | `engineering-mechanical-design-engineer` | 机械设计工程师 | 传动选型、强度校核、DFMA、GB/ISO | 原创 |

---

### 2.3 营销/广告行业

#### 营销部（36 个 Agent）

**国内平台运营岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 小红书运营 | `marketing-xiaohongshu-operator` | 小红书运营专家 | 种草笔记、达人合作、爆款内容 | 原创 |
| 抖音运营 | `marketing-douyin-strategist` | 抖音策略师 | 短视频策划、DOU+、直播带货 | 原创 |
| 微信运营 | `marketing-wechat-operator` | 微信公众号运营 | 公众号内容、社群运营、裂变增长 | 原创 |
| B站运营 | `marketing-bilibili-strategist` | B站内容策略师 | UP主运营、弹幕文化、品牌合作 | 原创 |
| 快手运营 | `marketing-kuaishou-strategist` | 快手策略师 | 下沉市场、老铁文化、直播电商 | 原创 |
| 电商运营 | `marketing-china-ecommerce-operator` | 中国电商运营专家 | 淘宝/天猫/拼多多/京东全平台 | 原创 |
| 电商运营 | `marketing-ecommerce-operator` | 电商运营师 | 淘宝/拼多多/京东店铺运营 | 原创 |
| 百度 SEO | `marketing-baidu-seo-specialist` | 百度 SEO 专家 | 百度算法、百度生态产品矩阵 | 原创 |
| 私域运营 | `marketing-private-domain-operator` | 私域流量运营师 | 企微 SCRM、社群精细化运营 | 原创 |
| 直播电商 | `marketing-livestream-commerce-coach` | 直播电商主播教练 | 直播话术、选品排品、千川投放 | 原创 |
| 跨境电商 | `marketing-cross-border-ecommerce` | 跨境电商运营专家 | Amazon/Shopee/Lazada、海外仓 | 原创 |
| 短视频剪辑 | `marketing-short-video-editing-coach` | 短视频剪辑指导师 | 剪映/PR/达芬奇、调色特效 | 原创 |
| 微博运营 | `marketing-weibo-strategist` | 微博运营策略师 | 热搜机制、超话运营、舆情管理 | 原创 |
| 播客运营 | `marketing-podcast-strategist` | 播客内容策略师 | 小宇宙/喜马拉雅、音频制作 | 原创 |
| 视频号运营 | `marketing-weixin-channels-strategist` | 微信视频号运营策略师 | 视频号直播、社交裂变 | 原创 |
| 知识付费 | `marketing-knowledge-commerce-strategist` | 知识付费产品策划师 | 得到/知识星球/小鹅通 | 原创 |
| 本地化 | `marketing-china-market-localization-strategist` | 中国市场本地化策略师 | 抖音/小红书/微信/B站全栈 | 原创 |
| 新闻情报 | `marketing-daily-news-briefing` | 新闻情报官 | 多源新闻采集、结构化简报 | 原创 |
| 小红书 | `marketing-xiaohongshu-specialist` | 小红书专家 | 生活方式内容、趋势策略 | 翻译 |
| 微信公众号 | `marketing-wechat-official-account` | 微信公众号管理 | 内容营销、用户互动 | 翻译 |
| 知乎 | `marketing-zhihu-strategist` | 知乎策略师 | 思想领袖、知识驱动互动 | 翻译 |

**出海营销岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| TikTok | `marketing-tiktok-strategist` | TikTok 策略师 | 病毒式内容、算法优化 | 翻译 |
| Twitter | `marketing-twitter-engager` | Twitter 互动官 | 实时互动、思想领袖 | 翻译 |
| Instagram | `marketing-instagram-curator` | Instagram 策展师 | 视觉叙事、社区运营 | 翻译 |
| Reddit | `marketing-reddit-community-builder` | Reddit 社区运营 | 社区文化、真实互动 | 翻译 |
| ASO | `marketing-app-store-optimizer` | 应用商店优化师 | ASO、转化率提升 | 翻译 |
| 视频优化 | `marketing-video-optimization-specialist` | 视频优化专家 | YouTube 算法、跨平台分发 | 翻译 |

**通用营销岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 增长 | `marketing-growth-hacker` | 增长黑客 | 低成本获客、病毒循环 | 翻译 |
| 内容 | `marketing-content-creator` | 内容创作者 | 多平台内容策划 | 翻译 |
| 社交媒体 | `marketing-social-media-strategist` | 社交媒体策略师 | 跨平台策略、整合营销 | 翻译 |
| SEO | `marketing-seo-specialist` | SEO 专家 | 技术 SEO、内容优化 | 翻译 |
| 轮播图 | `marketing-carousel-growth-engine` | 轮播图增长引擎 | 自动化轮播图生成 | 翻译 |
| LinkedIn | `marketing-linkedin-content-creator` | LinkedIn 内容创作专家 | 职场内容、B2B 获客 | 翻译 |
| 图书 | `marketing-book-co-author` | 图书联合作者 | 思想领袖力图书协作 | 翻译 |
| AI 搜索 | `marketing-agentic-search-optimizer` | 智能搜索优化师 | AI 代理任务完成率审计 | 翻译 |
| AI 引文 | `marketing-ai-citation-strategist` | AI 引文策略师 | AEO/GEO、AI 平台可见性 | 翻译 |

#### 付费媒体部（7 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 审计 | `paid-media-auditor` | 付费媒体审计师 | 广告账户审计、预算优化 | 翻译 |
| 创意 | `paid-media-creative-strategist` | 广告创意策略师 | 广告素材策划、A/B 测试 | 翻译 |
| 社交广告 | `paid-media-paid-social-strategist` | 社交广告策略师 | Meta/TikTok/LinkedIn 广告 | 翻译 |
| PPC | `paid-media-ppc-strategist` | PPC 竞价策略师 | Google Ads、智能出价 | 翻译 |
| 程序化 | `paid-media-programmatic-buyer` | 程序化广告采买专家 | DSP、RTB、程序化购买 | 翻译 |
| 搜索词 | `paid-media-search-query-analyst` | 搜索词分析师 | 搜索词挖掘、否词优化 | 翻译 |
| 追踪归因 | `paid-media-tracking-specialist` | 追踪与归因专家 | GTM、GA4、归因模型 | 翻译 |

---

### 2.4 金融行业

#### 金融部（8 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 簿记 | `finance-bookkeeper-controller` | 簿记与财务总监 | 记账、月末结账、银行对账 | 翻译 |
| 财务分析 | `finance-financial-analyst` | 财务分析师 | 财务建模、估值、报表分析 | 翻译 |
| 财务预测 | `finance-financial-forecaster` | 财务预测分析师 | 收入预测、现金流、场景建模 | 原创 |
| FP&A | `finance-fpa-analyst` | FP&A 分析师 | 预算编制、滚动预测、差异分析 | 翻译 |
| 风控 | `finance-fraud-detector` | 金融风控分析师 | 交易欺诈检测、反洗钱、电信诈骗 | 原创 |
| 投资研究 | `finance-investment-researcher` | 投资研究员 | 行业分析、公司估值、尽职调查 | 翻译 |
| 发票管理 | `finance-invoice-manager` | 发票管理专家 | 增值税发票、金税系统 | 原创 |
| 税务 | `finance-tax-strategist` | 税务策略师 | 税务筹划、跨境税务 | 翻译 |

---

### 2.5 销售/商务行业

#### 销售部（8 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 客户拓展 | `sales-account-strategist` | 客户拓展策略师 | Land-and-Expand、QBR 策划 | 翻译 |
| 销售教练 | `sales-coach` | 销售教练 | Pipeline Review、通话辅导 | 翻译 |
| 赢单策略 | `sales-deal-strategist` | 赢单策略师 | MEDDPICC、竞争定位 | 翻译 |
| Discovery | `sales-discovery-coach` | Discovery 教练 | 需求挖掘、差距量化 | 翻译 |
| 售前 | `sales-engineer` | 售前工程师 | 技术方案、Demo、POC | 翻译 |
| Outbound | `sales-outbound-strategist` | Outbound 策略师 | 多渠道触达、ICP 定义 | 翻译 |
| Pipeline | `sales-pipeline-analyst` | Pipeline 分析师 | 销售漏斗、预测分析 | 翻译 |
| 投标 | `sales-proposal-strategist` | 投标策略师 | RFP、赢标叙事 | 翻译 |

---

### 2.6 人力资源行业

#### 人力资源部（2 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 招聘 | `hr-recruiter` | 招聘专家 | Boss 直聘/猎聘、全流程招聘 | 原创 |
| 绩效 | `hr-performance-reviewer` | 绩效管理专家 | OKR/KPI 双轨制、360 度反馈 | 原创 |

---

### 2.7 法律/合规行业

#### 法务部（2 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 合同审查 | `legal-contract-reviewer` | 合同审查专家 | 民法典合同编、风险评估 | 原创 |
| 制度撰写 | `legal-policy-writer` | 制度文件撰写专家 | PIPL/数据安全法、隐私政策 | 原创 |

---

### 2.8 供应链/制造业

#### 供应链部（4 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 库存预测 | `supply-chain-inventory-forecaster` | 库存预测专家 | 需求预测、安全库存、618/双11 备货 | 原创 |
| 供应商管理 | `supply-chain-vendor-evaluator` | 供应商评估专家 | 1688 供应商、验厂、国标质检 | 原创 |
| 物流优化 | `supply-chain-route-optimizer` | 物流路线优化师 | 顺丰/通达系、冷链、跨境物流 | 原创 |
| 采购策略 | `supply-chain-strategist` | 供应链采购策略师 | 战略采购、质量管控、ERP | 原创 |

#### 供应链部 — 服装制造方向（1 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 服装工厂 | `supply-chain-garment-factory-planning-engineer` | 服装工厂规划工程师 | 服装生产排程、产线规划 | 翻译 |

---

### 2.9 游戏行业

#### 游戏开发部（20 个 Agent）

**通用岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 游戏设计 | `game-designer` | 游戏设计师 | GDD、游戏循环、经济平衡 | 翻译 |
| 关卡设计 | `level-designer` | 关卡设计师 | 布局理论、节奏架构、环境叙事 | 翻译 |
| 叙事设计 | `narrative-designer` | 叙事设计师 | 分支对话、世界观架构 | 翻译 |
| 技术美术 | `technical-artist` | 技术美术 | Shader、VFX、LOD 管线 | 翻译 |
| 游戏音频 | `game-audio-engineer` | 游戏音频工程师 | FMOD/Wwise、空间音频 | 翻译 |

**Unity 引擎岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| Unity 架构 | `unity-architect` | Unity 架构师 | ScriptableObject、模块化设计 | 翻译 |
| 编辑器工具 | `unity-editor-tool-developer` | Unity 编辑器工具开发者 | EditorWindow、PropertyDrawer | 翻译 |
| Unity 多人 | `unity-multiplayer-engineer` | Unity 多人游戏工程师 | Netcode、UGS | 翻译 |
| Shader Graph | `unity-shader-graph-artist` | Unity Shader Graph 美术师 | Shader Graph、URP/HDRP | 翻译 |

**Unreal Engine 岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| UE 多人 | `unreal-multiplayer-architect` | Unreal 多人游戏架构师 | Actor 复制、GameMode | 翻译 |
| UE 系统 | `unreal-systems-engineer` | Unreal 系统工程师 | C++/Blueprint、Nanite、Lumen | 翻译 |
| UE 技术美术 | `unreal-technical-artist` | Unreal 技术美术 | 材质编辑器、Niagara | 翻译 |
| UE 世界构建 | `unreal-world-builder` | Unreal 世界构建师 | World Partition、Landscape | 翻译 |

**Blender 岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| Blender 插件 | `blender-addon-engineer` | Blender 插件工程师 | Python 插件、管线自动化 | 翻译 |

**Godot 岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| Godot 脚本 | `godot-gameplay-scripter` | Godot 游戏脚本开发者 | GDScript 2.0、C# 集成 | 翻译 |
| Godot 多人 | `godot-multiplayer-engineer` | Godot 多人游戏工程师 | MultiplayerAPI、ENet/WebRTC | 翻译 |
| Godot Shader | `godot-shader-developer` | Godot Shader 开发者 | Godot 着色语言 | 翻译 |

**Roblox Studio 岗位：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| Roblox 脚本 | `roblox-systems-scripter` | Roblox 系统脚本工程师 | Luau、DataStore | 翻译 |
| Roblox 设计 | `roblox-experience-designer` | Roblox 体验设计师 | 参与循环、变现系统 | 翻译 |
| Roblox 形象 | `roblox-avatar-creator` | Roblox 虚拟形象创作者 | UGC 物品、虚拟形象系统 | 翻译 |

---

### 2.10 空间计算/XR 行业

#### 空间计算部（6 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| visionOS | `visionos-spatial-engineer` | visionOS 空间工程师 | SwiftUI 体积式界面、Liquid Glass | 翻译 |
| macOS Metal | `macos-spatial-metal-engineer` | macOS Metal 空间工程师 | Metal、GPU 渲染 | 翻译 |
| XR 界面 | `xr-interface-architect` | XR 界面架构师 | 空间交互、沉浸式界面 | 翻译 |
| XR 开发 | `xr-immersive-developer` | XR 沉浸式开发者 | WebXR、浏览器端 AR/VR | 翻译 |
| XR 座舱 | `xr-cockpit-interaction-specialist` | XR 座舱交互专家 | 座舱 UI、多模态交互 | 翻译 |
| 终端集成 | `terminal-integration-specialist` | 终端集成专家 | 终端模拟、SwiftTerm | 翻译 |

---

### 2.11 教育/学术行业

#### 学术部（6 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 人类学 | `academic-anthropologist` | 人类学家 | 文化体系、民族志方法 | 翻译 |
| 地理学 | `academic-geographer` | 地理学家 | 自然/人文地理、空间分析 | 翻译 |
| 历史学 | `academic-historian` | 历史学家 | 历史分析、物质文化 | 翻译 |
| 叙事学 | `academic-narratologist` | 叙事学家 | 叙事理论、故事结构 | 翻译 |
| 心理学 | `academic-psychologist` | 心理学家 | 人格理论、动机、认知模式 | 翻译 |
| 学习规划 | `academic-study-planner` | 学习规划师 | 考研/考公/法考备考策略 | 原创 |

---

### 2.12 跨行业专项/咨询

#### 专项部（46 个 Agent）

**AI/技术专项：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 智能体编排 | `agents-orchestrator` | 智能体编排者 | 多智能体协调、工作流管理 | 翻译 |
| 提示词工程 | `prompt-engineer` | 提示词工程师 | LLM 提示词设计、CoT、Few-shot | 原创 |
| 身份信任 | `agentic-identity-trust` | 身份信任架构师 | AI 身份验证、信任框架 | 翻译 |
| LSP | `lsp-index-engineer` | LSP 索引工程师 | 代码智能、语义索引 | 翻译 |
| MCP | `specialized-mcp-builder` | MCP 构建器 | MCP 服务器、API 集成 | 翻译 |
| 模型 QA | `specialized-model-qa` | 模型 QA 专家 | ML 模型审计、质量验证 | 翻译 |
| 文档生成 | `specialized-document-generator` | 文档生成器 | PDF/PPTX/DOCX/XLSX | 翻译 |
| 工作流 | `specialized-workflow-architect` | 工作流架构师 | 工作流树、交接契约 | 翻译 |
| 自动化治理 | `automation-governance-architect` | 自动化治理架构师 | n8n 工作流治理 | 翻译 |
| Salesforce | `specialized-salesforce-architect` | Salesforce 架构师 | 多云设计、集成模式 | 翻译 |
| 区块链安全 | `blockchain-security-auditor` | 区块链安全审计师 | 智能合约审计、漏洞检测 | 翻译 |
| 开发者布道 | `specialized-developer-advocate` | 开发者布道师 | 开发者社区、技术内容 | 翻译 |
| 技术翻译 | `technical-translator-agent` | 技术翻译专家 | 中英文双向、编程/AI 术语 | 翻译 |
| ZK 知识管理 | `zk-steward` | ZK 管家 | Zettelkasten 卡片盒笔记法 | 翻译 |
| AI 治理 | `specialized-ai-policy-writer` | AI 治理政策专家 | 算法备案、生成式 AI 管理 | 原创 |

**数据/报告专项：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 数据整合 | `data-consolidation-agent` | 数据整合师 | 多源数据整合、仪表盘 | 翻译 |
| 报告分发 | `report-distribution-agent` | 报告分发师 | 自动化报告分发 | 翻译 |
| 销售数据 | `sales-data-extraction-agent` | 销售数据提取师 | Excel 监控、销售指标提取 | 翻译 |
| 身份图谱 | `identity-graph-operator` | 身份图谱操作员 | 身份解析、多源匹配 | 翻译 |

**合规/审计专项：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 合规审计 | `compliance-auditor` | 合规审计师 | SOC 2/ISO 27001/HIPAA | 翻译 |
| 应付账款 | `accounts-payable-agent` | 应付账款智能体 | 发票处理、付款自动化 | 翻译 |
| 企业风险 | `specialized-risk-assessor` | 企业风险评估师 | COSO 本土化、国企风控、ESG | 原创 |

**垂直行业专项：**

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 医疗客服 | `healthcare-customer-service` | 医疗客服专家 | 预约、保险、处方、分诊 | 翻译 |
| 医疗合规 | `healthcare-marketing-compliance` | 医疗健康营销合规师 | 医疗广告法、NMPA | 原创 |
| 酒店服务 | `hospitality-guest-services` | 酒店宾客服务专家 | 预订、客房、礼宾 | 翻译 |
| 留学规划 | `study-abroad-advisor` | 留学规划顾问 | 多国申请、选校定位 | 原创 |
| 高考志愿 | `gaokao-college-advisor` | 高考志愿填报顾问 | 平行志愿、位次法 | 原创 |
| 政务售前 | `government-digital-presales-consultant` | 政务数字化售前顾问 | 等保/信创、标书 | 原创 |
| 企业培训 | `corporate-training-designer` | 企业培训课程设计师 | ADDIE/SAM、企业学习平台 | 原创 |
| 动态定价 | `specialized-pricing-optimizer` | 动态定价策略师 | 电商定价、大促机制 | 原创 |
| 养殖档案 | `livestock-archive-auditor` | 养殖档案核对员 | 畜禽养殖档案、FIFO 复核 | 原创 |
| 土木工程 | `specialized-civil-engineer` | 土木工程师 | Eurocode/GB 多标准结构分析 | 翻译 |
| 房地产 | `real-estate-buyer-seller` | 房地产经纪助手 | 市场分析、报价策略 | 翻译 |
| 零售退货 | `retail-customer-returns` | 零售退货专家 | 退换货、退款、欺诈检测 | 翻译 |
| 信贷 | `loan-officer-assistant` | 信贷经理助手 | 贷款审批、合规检查 | 翻译 |
| HR 入职 | `hr-onboarding` | HR 入职管理专家 | 入职文档、合规追踪 | 翻译 |
| 招聘 | `recruitment-specialist` | 招聘专家 | 国内招聘平台、劳动法合规 | 原创 |
| 会议管理 | `specialized-meeting-assistant` | 会议效率专家 | 飞书/钉钉/腾讯会议 | 原创 |
| 幕僚长 | `specialized-chief-of-staff` | 幕僚长 | OKR 追踪、高管沟通、组织变革 | 翻译 |
| 文化智能 | `specialized-cultural-intelligence-strategist` | 文化智能策略师 | 跨文化设计、全球化产品 | 翻译 |
| 法国市场 | `specialized-french-consulting-market` | 法国咨询市场专家 | ESN/SI 生态、薪资代管 | 翻译 |
| 韩国商务 | `specialized-korean-business-navigator` | 韩国商务专家 | KakaoTalk 礼仪、层级关系 | 翻译 |
| 语言翻译 | `language-translator` | 语言翻译专家 | 实时翻译、文化语境 | 翻译 |
| 律所计费 | `legal-billing-time-tracking` | 律所计费与工时专家 | 工时录入、账单生成 | 翻译 |
| 律所接案 | `legal-client-intake` | 律所客户接案专家 | 初步咨询、利益冲突检查 | 翻译 |
| 法律文书 | `legal-document-review` | 法律文书审查专家 | 合同摘要、风险条款标记 | 翻译 |

#### 支持部（7 个 Agent）

| 岗位方向 | Agent ID | 中文名 | 核心技能 | 来源 |
|----------|----------|--------|----------|------|
| 客服 | `support-support-responder` | 客服响应者 | 多渠道客户服务、工单处理 | 翻译 |
| 数据分析 | `support-analytics-reporter` | 数据分析师 | 数据分析、仪表盘 | 翻译 |
| 合规 | `support-legal-compliance-checker` | 法务合规员 | 合规审查、法规检查 | 翻译 |
| 高管摘要 | `support-executive-summary-generator` | 高管摘要师 | 业务摘要、战略沟通 | 翻译 |
| 财务追踪 | `support-finance-tracker` | 财务追踪员 | 财务分析、预算管理 | 翻译 |
| 基础设施 | `support-infrastructure-maintainer` | 基础设施运维师 | 系统运维、可靠性工程 | 翻译 |
| 招聘运营 | `support-recruitment-specialist` | 招聘运营专家 | Boss 直聘/猎聘、劳动法 | 原创 |

---

## 三、行业维度汇总

| 行业 | 覆盖部门 | Agent 数量 | 代表性岗位 |
|------|----------|-----------|-----------|
| 互联网/软件 | 工程部、设计部、产品部、测试部、项目管理部 | 63 | 前端/后端/AI/DevOps/产品经理/UI 设计师 |
| 营销/广告 | 营销部、付费媒体部 | 43 | 小红书运营/抖音策略/PPC 竞价/增长黑客 |
| 金融 | 金融部 | 8 | 财务分析师/风控分析师/税务策略师 |
| 嵌入式/工业 | 工程部（工业方向） | 6 | 嵌入式固件/FPGA/IoT 架构/上位机 |
| 游戏 | 游戏开发部 | 20 | 游戏设计师/Unity 架构/UE 系统工程师 |
| 空间计算/XR | 空间计算部 | 6 | visionOS 工程师/XR 界面架构师 |
| 教育/学术 | 学术部 | 6 | 学习规划师/历史学家/心理学家 |
| 供应链/制造 | 供应链部 | 5 | 库存预测/供应商评估/物流优化 |
| 销售/B2B | 销售部 | 8 | 赢单策略师/售前工程师/Pipeline 分析师 |
| 人力资源 | 人力资源部 | 2 | 招聘专家/绩效管理专家 |
| 法律/合规 | 法务部 | 2 | 合同审查/制度文件撰写 |
| 跨行业专项 | 专项部、支持部 | 53 | 提示词工程师/高考志愿/政务售前/医疗合规 |

---

## 四、中国市场原创 Agent 行业分布（50 个）

| 行业/领域 | 原创 Agent 数量 | 代表性 Agent |
|-----------|----------------|-------------|
| 社交媒体营销 | 18 | 小红书运营专家、抖音策略师、微信公众号运营、B站策略师、快手策略师 |
| 电商/零售 | 5 | 中国电商运营专家、跨境电商运营专家、动态定价策略师 |
| 金融/财税 | 3 | 金融风控分析师、发票管理专家、财务预测分析师 |
| 企业协作 | 2 | 飞书集成开发工程师、钉钉集成开发工程师 |
| 嵌入式/工业 | 5 | 嵌入式 Linux 驱动、FPGA、IoT 架构、上位机、机械设计 |
| 人力资源 | 2 | 招聘专家、绩效管理专家 |
| 法律/合规 | 4 | 合同审查专家、制度文件撰写专家、AI 治理政策专家、企业风险评估师 |
| 供应链 | 4 | 库存预测、供应商评估、物流优化、采购策略 |
| 教育 | 3 | 学习规划师、高考志愿填报顾问、留学规划顾问 |
| 垂直行业 | 4 | 政务售前顾问、医疗合规师、养殖档案核对员、会议效率专家 |
| AI/技术 | 2 | 提示词工程师、微信小程序开发者 |

---

## 五、岗位技能关键词云

### 高频技能关键词（按出现频次排序）

| 排名 | 技能关键词 | 涉及 Agent 数 |
|------|-----------|--------------|
| 1 | 数据分析/驱动 | 35+ |
| 2 | 自动化/流程优化 | 30+ |
| 3 | 策略/规划 | 28+ |
| 4 | 内容创作/文案 | 25+ |
| 5 | API/集成 | 20+ |
| 6 | 安全/合规 | 18+ |
| 7 | 性能优化 | 15+ |
| 8 | 用户研究/洞察 | 12+ |
| 9 | 测试/验证 | 10+ |
| 10 | AI/ML/LLM | 10+ |

### 中国特色技能关键词

| 关键词 | 涉及 Agent |
|--------|-----------|
| 微信生态（公众号/小程序/视频号/企微） | 5 |
| 抖音/DOU+/千川 | 3 |
| 小红书/种草/蒲公英 | 3 |
| 金税系统/增值税发票 | 2 |
| 等保/信创/国密 | 2 |
| Boss 直聘/猎聘 | 3 |
| 民法典/PIPL/数据安全法 | 3 |
| 1688/淘宝/拼多多/京东 | 4 |

---

## 六、对 Aranea-Agents 项目的启示

### 6.1 Agent 定义参考

Agency-Agents 的每个 Agent 文件包含以下结构化信息，可作为 Aranea-Agents Agent 定义的参考：

1. **身份定义**：角色名称、专业定位、沟通风格
2. **关键规则**：必须遵守的铁律（3-7 条）
3. **工作流程**：结构化的执行步骤
4. **交付物**：明确的输出格式和质量标准
5. **工具使用**：推荐使用的工具和技术栈

### 6.2 行业覆盖对比

| 行业 | Agency-Agents 覆盖 | Aranea-Agents 现状 | 差距 |
|------|-------------------|-------------------|------|
| 软件工程 | 35 个 Agent | 有基础 Agent 框架 | 需细化岗位 |
| 营销/广告 | 43 个 Agent | 未覆盖 | 大差距 |
| 金融 | 8 个 Agent | 未覆盖 | 大差距 |
| 游戏 | 20 个 Agent | 未覆盖 | 大差距 |
| 嵌入式/工业 | 6 个 Agent | 未覆盖 | 大差距 |
| 供应链 | 5 个 Agent | 未覆盖 | 大差距 |

### 6.3 可借鉴的模式

1. **行业 → 部门 → 岗位 → Agent** 的四级分类体系，清晰且可扩展
2. **中国市场原创**策略：在翻译海外项目基础上，补充本土化 Agent
3. **工具集成**：17 种 AI 编程工具的一键安装脚本
4. **Agent 文件标准化**：每个 Agent 独立 Markdown 文件，结构统一
5. **战略编排**：strategy 目录提供 Phase 0-6 的完整 Playbook 和 Runbook

---

> 本报告基于 [agency-agents-zh](https://github.com/jnMetaCode/agency-agents-zh) 项目（215 个 AI 智能体）分析生成

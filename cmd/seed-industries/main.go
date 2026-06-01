package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print plan only")
	flag.Parse()

	dbPath := resolveSQLitePath()
	fmt.Printf("sqlite: %s\n", dbPath)
	if *dryRun {
		fmt.Println("mode: dry-run")
	}

	ctx := context.Background()
	entClient, rawDB, cleanup, err := data.OpenSQLiteEntClient(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := ensureTables(ctx, rawDB); err != nil {
		fmt.Fprintf(os.Stderr, "ensure tables: %v\n", err)
		os.Exit(1)
	}
	_ = entClient

	store := data.NewCLIData(entClient, rawDB, loggateway.NewNoop())
	indRepo := data.NewIndustryRepo(store)
	depRepo := data.NewDepartmentRepo(store)
	posRepo := data.NewPositionRepo(store)
	catRepo := data.NewAgentCategoryRepo(store)
	catUC := biz.NewAgentCategoryUsecase(catRepo)
	indUC := biz.NewIndustryUsecase(indRepo, catUC)
	depUC := biz.NewDepartmentUsecase(depRepo, catUC)
	posUC := biz.NewPositionUsecase(posRepo, catUC)

	industries := []biz.Industry{
		{Key: "softwaredev", Name: "软件开发", Icon: "💻", Description: "覆盖系统软件、Web 应用、移动 App、游戏（含 UE 引擎）的全栈软件开发行业。从需求分析、架构设计、编码实现、质量保障到运维部署的完整软件生命周期。", ScenarioKey: "softwaredev", Enabled: true, SortOrder: 1},
		{Key: "selfmedia", Name: "自媒体 / 内容创作", Icon: "🎬", Description: "覆盖网文小说创作、短视频制作、图文内容、直播运营、多平台分发的全链路内容创作行业。", ScenarioKey: "selfmedia", Enabled: true, SortOrder: 2},
		{Key: "finance", Name: "金融 / 投资", Icon: "📈", Description: "覆盖证券研究、量化交易、固收衍生品、合规风控、财富管理的全链条金融行业。", ScenarioKey: "finance", Enabled: true, SortOrder: 3},
	}

	for _, ind := range industries {
		if *dryRun {
			fmt.Printf("[dry-run] upsert industry: %s (%s)\n", ind.Key, ind.Name)
			continue
		}
		result, err := indUC.UpsertByKey(ctx, ind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert industry %s: %v\n", ind.Key, err)
			continue
		}
		fmt.Printf("upserted industry: %s (id=%s)\n", result.Key, result.ID)
	}

	departments := []biz.Department{
		{Key: "backend", Name: "后端研发部", IndustryKey: "softwaredev", Description: "负责服务端架构设计与核心业务逻辑实现", ResponsibilitiesJSON: `{"lead":"主导技术方案评审与架构决策","develop":"高质量编码、Code Review、性能优化","maintain":"线上问题排查、系统稳定性保障"}`, SortOrder: 1},
		{Key: "frontend", Name: "前端研发部", IndustryKey: "softwaredev", Description: "负责 Web UI / 移动端的用户界面与交互体验", SortOrder: 2},
		{Key: "gamedev", Name: "游戏开发部", IndustryKey: "softwaredev", Description: "基于 UE5 的游戏客户端与服务端开发", SortOrder: 3},
		{Key: "mobiledev", Name: "移动端研发部", IndustryKey: "softwaredev", Description: "Flutter/iOS/Android 跨平台与原生开发", SortOrder: 4},
		{Key: "devops", Name: "DevOps / 基础设施", IndustryKey: "softwaredev", Description: "CI/CD、K8s、云基础设施、SRE", SortOrder: 5},
		{Key: "architecture", Name: "架构与设计", IndustryKey: "softwaredev", Description: "系统架构、技术评审、领域建模", SortOrder: 6},
		{Key: "qa", Name: "质量保障", IndustryKey: "softwaredev", Description: "自动化测试、SDET、性能测试、安全测试", SortOrder: 7},
		{Key: "dataeng", Name: "数据工程", IndustryKey: "softwaredev", Description: "数据管道、BI 分析、数据平台", SortOrder: 8},
		{Key: "security", Name: "安全", IndustryKey: "softwaredev", Description: "应用安全、安全审计、渗透测试", SortOrder: 9},
		{Key: "productpm", Name: "产品与项目管理", IndustryKey: "softwaredev", Description: "产品经理、Scrum Master、技术文档", SortOrder: 10},
		{Key: "fiction_writing", Name: "小说创作部", IndustryKey: "selfmedia", Description: "网文策划与创作，覆盖起点/番茄/晋江等主流平台", ResponsibilitiesJSON: `{"create":"正文创作、大纲设计、世界观设定","quality":"角色一致性维护、节奏控制、伏笔管理","adapt":"平台调性适配、数据驱动写作"}`, SortOrder: 1},
		{Key: "video_production", Name: "视频制作部", IndustryKey: "selfmedia", Description: "短视频全流程制作，从策划到发布", ResponsibilitiesJSON: `{"plan":"选题策划、分镜脚本、内容日历","produce":"拍摄指导、剪辑制作、特效包装","optimize":"完播率优化、平台适配"}`, SortOrder: 2},
		{Key: "content_graphic", Name: "图文内容部", IndustryKey: "selfmedia", Description: "公众号/小红书/知识付费内容运营", ResponsibilitiesJSON: `{"content":"图文内容创作、种草笔记撰写","growth":"粉丝增长、SEO优化","monetize":"知识付费课程设计"}`, SortOrder: 3},
		{Key: "live_streaming", Name: "直播运营部", IndustryKey: "selfmedia", Description: "直播全流程运营，含话术设计、场控、数据复盘", ResponsibilitiesJSON: `{"host":"主播话术、互动节奏、带货脚本","control":"场控流程、应急预案","analyze":"直播数据分析、ROI优化"}`, SortOrder: 4},
		{Key: "distribution", Name: "多平台分发与运营", IndustryKey: "selfmedia", Description: "多平台运营/SEO/粉丝增长/变现", ResponsibilitiesJSON: `{"distribute":"一键分发、格式适配、发布时间优化","seo":"关键词研究、搜索排名优化","monetize":"广告、带货、知识付费、IP授权"}`, SortOrder: 5},
		{Key: "equity_research", Name: "证券研究部", IndustryKey: "finance", Description: "股票分析（复用 stockx 场景能力），覆盖技术面/基本面/资金面/消息面/情绪面/行业面六维分析", ResponsibilitiesJSON: `{"analyze":"六维分析（技术/基本面/资金/消息/情绪/行业）","quant":"量化因子研究、回测、组合构建、ML Alpha","report":"研究报告撰写、盘前盘后简报"}`, SortOrder: 1},
		{Key: "quant_trading", Name: "量化交易部", IndustryKey: "finance", Description: "策略研究与实盘系统，含研究平台/数据管线/交易系统/算法交易/低延迟", ResponsibilitiesJSON: `{"develop":"量化平台开发、数据管线、交易系统","trade":"算法交易、做市策略、执行优化","infra":"低延迟系统、内核调优、网络优化"}`, SortOrder: 2},
		{Key: "fixed_income", Name: "固收与衍生品", IndustryKey: "finance", Description: "债券/期权/期货/互换，含固收分析与衍生品定价", ResponsibilitiesJSON: `{"bond":"债券定价、久期凸性、收益率曲线、信用评级","deriv":"期权定价、波动率曲面、期货策略、套利"}`, SortOrder: 3},
		{Key: "compliance_risk", Name: "合规与风控", IndustryKey: "finance", Description: "金融监管合规，含市场风险/信用风险/反洗钱", ResponsibilitiesJSON: `{"comply":"证券法规、内幕交易防范、信息披露","risk":"VaR/CVaR、压力测试、信用评分","aml":"KYC/KYB、可疑交易监测、制裁筛查"}`, SortOrder: 4},
		{Key: "wealth_mgmt", Name: "财富管理", IndustryKey: "finance", Description: "面向个人/机构的投顾与资产配置", ResponsibilitiesJSON: `{"advise":"投资顾问、客户画像、适当性匹配","allocate":"战略/战术资产配置、再平衡"}`, SortOrder: 5},
		{Key: "fintech", Name: "金融科技", IndustryKey: "finance", Description: "金融 × 科技交叉，含数据工程/实时行情/区块链", ResponsibilitiesJSON: `{"product":"金融科技产品设计、监管科技","data":"实时行情数据、数据湖、流式计算","chain":"链上数据分析、DeFi协议、智能合约"}`, SortOrder: 6},
	}

	for _, dep := range departments {
		if *dryRun {
			fmt.Printf("[dry-run] upsert department: %s/%s\n", dep.IndustryKey, dep.Key)
			continue
		}
		result, err := depUC.UpsertByKey(ctx, dep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert department %s/%s: %v\n", dep.IndustryKey, dep.Key, err)
			continue
		}
		fmt.Printf("upserted department: %s/%s (id=%s)\n", result.IndustryKey, result.Key, result.ID)
	}

	positions := []biz.Position{
		{Key: "go_senior_engineer", Name: "Golang 高级工程师", DepartmentKey: "backend", Description: "负责高并发微服务后端的架构设计与核心模块开发。精通 Go 语言特性、Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL。", ResponsibilitiesJSON: `{"core":["高并发微服务架构设计","Go 语言精通（goroutine/channel/interface/泛型/GC）","Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL","Clean Architecture / DDD 分层","Code Review 与系统稳定性","线上问题排查（panic/死锁/内存泄漏）"]}`, SkillsRequired: []string{"Go 1.22+ 泛型", "goroutine 调度模型（GMP）", "Kratos v2 / gRPC", "PostgreSQL / Redis / Kafka", "Clean Architecture / DDD"}, SeniorityLevel: "P6-P7", SortOrder: 1},
		{Key: "java_senior_engineer", Name: "Java 高级工程师", DepartmentKey: "backend", Description: "负责 Java 后端微服务开发，Spring Boot / Spring Cloud 生态。", SeniorityLevel: "P6-P7", SortOrder: 2},
		{Key: "python_senior_engineer", Name: "Python 高级工程师", DepartmentKey: "backend", Description: "负责 Python 后端开发与数据管道。", SeniorityLevel: "P6-P7", SortOrder: 3},
		{Key: "rust_engineer", Name: "Rust 工程师", DepartmentKey: "backend", Description: "负责 Rust 系统编程与高性能组件。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "cpp_backend_engineer", Name: "C++ 后端工程师", DepartmentKey: "backend", Description: "负责 C++ 高性能后端与游戏服务端。", SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "database_administrator", Name: "数据库管理员 DBA", DepartmentKey: "backend", Description: "负责数据库调优、高可用与可靠性。", SeniorityLevel: "P5-P7", SortOrder: 6},
		{Key: "vue3_senior_engineer", Name: "Vue 3 高级前端工程师", DepartmentKey: "frontend", Description: "基于 Vue 3 + Composition API + TypeScript 开发企业级 Web 应用。精通 Quasar/Pinia/Vue Router 生态。", SeniorityLevel: "P6-P7", SortOrder: 1},
		{Key: "react_senior_engineer", Name: "React 高级前端工程师", DepartmentKey: "frontend", Description: "基于 React + TypeScript 开发企业级 Web 应用。", SeniorityLevel: "P6-P7", SortOrder: 2},
		{Key: "typescript_specialist", Name: "TypeScript 技术专家", DepartmentKey: "frontend", Description: "TypeScript 类型系统设计与迁移专家。", SeniorityLevel: "P6-P8", SortOrder: 3},
		{Key: "frontend_performance_engineer", Name: "前端性能优化工程师", DepartmentKey: "frontend", Description: "专注 Web 性能优化与 Core Web Vitals。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "ui_ux_implementer", Name: "UI/UX 还原工程师", DepartmentKey: "frontend", Description: "高保真 UI 还原与交互实现。", SeniorityLevel: "P4-P6", SortOrder: 5},
		{Key: "ue_client_programmer", Name: "UE 客户端程序", DepartmentKey: "gamedev", Description: "基于 Unreal Engine 5 进行客户端功能开发（C++ + Blueprint 协作）。精通 UEFN、GAS、Replication。", ResponsibilitiesJSON: `{"core":["UE5 客户端功能开发（C++ + Blueprint）","GameFramework / Actor-Component 模型","GAS（Gameplay Ability System）集成与定制","网络 Replication（属性复制/RPC/Role 权限）","性能优化（Draw Call/GPU Profile/Unreal Insights）","平台适配（PC/Console/Mobile）"]}`, SkillsRequired: []string{"UE5 GameFramework", "Actor Component 组合模式", "GAS (AbilitySystemComponent)", "Replication (RepNotify/RPC)", "Unreal Insights"}, SeniorityLevel: "P5-P8", SortOrder: 1},
		{Key: "ue_gameplay_programmer", Name: "UE 游戏逻辑程序", DepartmentKey: "gamedev", Description: "Gameplay Framework 与战斗逻辑。", SeniorityLevel: "P5-P7", SortOrder: 2},
		{Key: "ue_graphics_programmer", Name: "UE 图形渲染程序", DepartmentKey: "gamedev", Description: "材质系统与渲染管线优化。", SeniorityLevel: "P6-P8", SortOrder: 3},
		{Key: "game_server_engineer", Name: "游戏服务端工程师", DepartmentKey: "gamedev", Description: "游戏服务端架构与实时同步。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "game_technical_artist", Name: "技术 TA", DepartmentKey: "gamedev", Description: "美术-程序桥梁，管线与 Shader 开发。", SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "game_planner_designer", Name: "系统策划", DepartmentKey: "gamedev", Description: "系统设计与数值平衡。", SeniorityLevel: "P4-P7", SortOrder: 6},

		{Key: "webnovel_author", Name: "网文小说作者（主力）", DepartmentKey: "fiction_writing", Description: "负责网文小说的正文创作，单章 2000-3000 字，日更 6000-10000 字。熟悉起点/番茄/晋江等主流平台调性，掌握黄金三章开篇法则、爽点节奏设计、悬念钩子。", ResponsibilitiesJSON: `{"core":["正文创作，单章2000-3000字，日更6000-10000字","平台调性适配（起点/番茄/晋江）","黄金三章开篇法则、爽点节奏设计、章节尾钩子","角色一致性维护（人设卡+行为准则），避免OOC","战斗场景描写、情感戏推进、对话自然度","根据数据反馈（追读率、章均点推比）调整写作策略"]}`, SkillsRequired: []string{"网文创作（玄幻/仙侠/都市/言情）", "平台调性适配", "节奏控制与悬念设计", "角色塑造与一致性维护", "数据驱动写作"}, SeniorityLevel: "P5-P8", SortOrder: 1},
		{Key: "webnovel_plotter", Name: "剧情策划 / 大纲设计师", DepartmentKey: "fiction_writing", Description: "负责小说大纲设计、主线支线交织、伏笔埋设与回收计划。精通三幕式/英雄之旅/起承转合结构。", ResponsibilitiesJSON: `{"core":["小说大纲设计（三幕式/英雄之旅/起承转合）","主线+支线交织规划","伏笔埋设与回收计划","爽点间隔设计、高潮递进、低谷蓄力"]}`, SkillsRequired: []string{"故事结构设计", "伏笔与回收管理", "节奏控制"}, SeniorityLevel: "P5-P7", SortOrder: 2},
		{Key: "worldbuilding_designer", Name: "世界观 / 设定设计师", DepartmentKey: "fiction_writing", Description: "负责小说世界观设定，含力量体系、地理历史、种族势力。避免战力崩坏。", ResponsibilitiesJSON: `{"core":["力量体系设计（修炼等级、能力边界、代价平衡）","地理历史设定（世界观地图、种族/势力、历史纪元）","设定一致性维护"]}`, SkillsRequired: []string{"世界观构建", "力量体系设计", "设定一致性管理"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "character_writer", Name: "角色塑造专家", DepartmentKey: "fiction_writing", Description: "负责角色创建与人设卡设计，角色弧光设计，OOC检测与行为准则验证。", ResponsibilitiesJSON: `{"core":["角色创建（人设卡：性格/口头禅/关系/状态）","角色弧光设计","OOC检测与行为准则验证"]}`, SkillsRequired: []string{"角色设计", "人设卡管理", "角色一致性检查"}, SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "fiction_editor", Name: "责任编辑", DepartmentKey: "fiction_writing", Description: "负责小说质量把关、节奏评估、商业性建议、合规审查。", ResponsibilitiesJSON: `{"core":["质量把关、节奏评估、商业性建议","敏感词检测、尺度把控、平台规则适配"]}`, SkillsRequired: []string{"内容审查", "节奏评估", "合规检查"}, SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "short_video_director", Name: "短视频导演 / 编导", DepartmentKey: "video_production", Description: "负责短视频选题策划、分镜脚本、成片审查。精通热点追踪、爆款拆解、完播率优化。", ResponsibilitiesJSON: `{"core":["选题策划、热点追踪、爆款拆解","分镜脚本设计（镜头/转场/节奏卡点）","成片审查（完播率预判/节奏检查/平台适配）"]}`, SkillsRequired: []string{"短视频策划", "分镜脚本", "爆款分析", "完播率优化"}, SeniorityLevel: "P5-P7", SortOrder: 1},
		{Key: "video_scriptwriter", Name: "视频脚本编剧", DepartmentKey: "video_production", Description: "负责视频脚本撰写，开头3秒钩子设计、信息密度控制、情绪曲线规划。适配抖音/B站/小红书/视频号。", ResponsibilitiesJSON: `{"core":["脚本撰写（开头3秒钩子/信息密度/情绪曲线）","平台适配（抖音/B站/小红书/视频号风格差异）"]}`, SkillsRequired: []string{"视频脚本撰写", "平台内容适配", "情绪曲线设计"}, SeniorityLevel: "P4-P7", SortOrder: 2},
		{Key: "video_editor_premiere", Name: "视频剪辑师（PR 达人）", DepartmentKey: "video_production", Description: "精通 Premiere Pro 剪辑，含时间线管理、多轨编辑、调色、AE联动特效。", ResponsibilitiesJSON: `{"core":["Premiere Pro 剪辑（时间线/多轨/调色）","AE联动特效制作（转场/字幕动效）"]}`, SkillsRequired: []string{"Premiere Pro", "After Effects", "调色"}, SeniorityLevel: "P4-P7", SortOrder: 3},
		{Key: "video_editor_capcut", Name: "剪映专业剪辑师", DepartmentKey: "video_production", Description: "精通剪映专业版剪辑，含模板使用、智能字幕、贴纸动效、模板创作。", ResponsibilitiesJSON: `{"core":["剪映专业版剪辑（模板/智能字幕/贴纸动效）","剪映模板创作与设计规范"]}`, SkillsRequired: []string{"剪映专业版", "模板设计", "短视频剪辑"}, SeniorityLevel: "P3-P6", SortOrder: 4},
		{Key: "motion_graphics_artist", Name: "动效 / 包装设计师", DepartmentKey: "video_production", Description: "负责MG动画、片头片尾、信息可视化动画、品牌视觉包装。", ResponsibilitiesJSON: `{"core":["MG动画设计（片头片尾/信息可视化）","品牌视觉包装（统一视觉体系/品牌色/字体/动效规范）"]}`, SkillsRequired: []string{"After Effects", "MG动画", "品牌设计"}, SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "sound_designer", Name: "音效 / 配乐设计师", DepartmentKey: "video_production", Description: "负责BGM选择、音效设计、混音。", ResponsibilitiesJSON: `{"core":["BGM选择与配乐设计","音效设计与混音"]}`, SkillsRequired: []string{"音频制作", "混音", "配乐"}, SeniorityLevel: "P4-P7", SortOrder: 6},
		{Key: "thumbnail_designer", Name: "封面图设计师", DepartmentKey: "video_production", Description: "负责视频封面设计，视觉冲击力、文字排版、平台尺寸适配。", ResponsibilitiesJSON: `{"core":["视频封面设计（视觉冲击力/文字排版）","平台尺寸适配（抖音/B站/小红书/视频号）"]}`, SkillsRequired: []string{"封面设计", "视觉设计", "排版"}, SeniorityLevel: "P3-P6", SortOrder: 7},
		{Key: "wechat_operator", Name: "公众号运营专员", DepartmentKey: "content_graphic", Description: "负责公众号内容运营，含标题技巧、排版规范、原创保护、裂变增长。", ResponsibilitiesJSON: `{"core":["公众号内容运营（标题/排版/原创）","增长策略（裂变/互推/SEO）"]}`, SkillsRequired: []string{"公众号运营", "内容创作", "增长策略"}, SeniorityLevel: "P4-P6", SortOrder: 1},
		{Key: "xiaohongshu_creator", Name: "小红书种草达人", DepartmentKey: "content_graphic", Description: "负责小红书种草笔记撰写、关键词布局、话题标签、搜索排名优化。", ResponsibilitiesJSON: `{"core":["种草笔记撰写（关键词布局/话题标签）","小红书SEO（搜索排名优化/笔记权重提升）"]}`, SkillsRequired: []string{"小红书运营", "种草笔记", "SEO优化"}, SeniorityLevel: "P4-P6", SortOrder: 2},
		{Key: "knowledge_pay_writer", Name: "知识付费撰稿人", DepartmentKey: "content_graphic", Description: "负责知识付费课程大纲设计、内容深度把控、付费转化。", ResponsibilitiesJSON: `{"core":["课程大纲设计与内容深度把控","付费转化策略"]}`, SkillsRequired: []string{"课程设计", "知识付费", "内容策划"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "live_stream_host", Name: "主播 / 场控", DepartmentKey: "live_streaming", Description: "负责直播话术设计、互动节奏、带货脚本、弹幕互动、粉丝团运营。", ResponsibilitiesJSON: `{"core":["主播话术设计、互动节奏、带货脚本","弹幕互动、抽奖活动、粉丝团运营"]}`, SkillsRequired: []string{"直播话术", "互动运营", "带货脚本"}, SeniorityLevel: "P4-P7", SortOrder: 1},
		{Key: "live_stream_scriptwriter", Name: "直播脚本编剧", DepartmentKey: "live_streaming", Description: "负责直播流程设计、话术模板、应急预案。", ResponsibilitiesJSON: `{"core":["直播流程设计与话术模板","应急预案与突发处理"]}`, SkillsRequired: []string{"直播脚本", "流程设计", "应急处理"}, SeniorityLevel: "P4-P6", SortOrder: 2},
		{Key: "live_stream_analyst", Name: "直播数据分析师", DepartmentKey: "live_streaming", Description: "负责直播数据分析，在线人数曲线、转化率、ROI。", ResponsibilitiesJSON: `{"core":["直播数据分析（在线人数/转化率/ROI）","复盘报告与优化建议"]}`, SkillsRequired: []string{"数据分析", "直播运营", "ROI优化"}, SeniorityLevel: "P4-P6", SortOrder: 3},
		{Key: "multi_platform_operator", Name: "多平台运营专员", DepartmentKey: "distribution", Description: "负责多平台一键分发、格式适配、发布时间优化、各平台内容规范。", ResponsibilitiesJSON: `{"core":["一键分发、格式适配、发布时间优化","各平台内容规范与算法偏好适配"]}`, SkillsRequired: []string{"多平台运营", "内容适配", "发布策略"}, SeniorityLevel: "P4-P6", SortOrder: 1},
		{Key: "seo_specialist", Name: "SEO 优化师", DepartmentKey: "distribution", Description: "负责关键词研究、内容SEO、技术SEO、搜索排名优化。", ResponsibilitiesJSON: `{"core":["关键词研究与内容SEO","技术SEO与搜索排名优化"]}`, SkillsRequired: []string{"SEO优化", "关键词研究", "搜索排名"}, SeniorityLevel: "P4-P7", SortOrder: 2},
		{Key: "fan_growth_strategist", Name: "粉丝增长策略师", DepartmentKey: "distribution", Description: "负责社群运营、裂变策略、私域流量。", ResponsibilitiesJSON: `{"core":["社群运营与裂变策略","私域流量建设与粉丝增长"]}`, SkillsRequired: []string{"社群运营", "裂变策略", "私域流量"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "monetization_specialist", Name: "变现策略师", DepartmentKey: "distribution", Description: "负责广告、带货、知识付费、IP授权等变现策略。", ResponsibilitiesJSON: `{"core":["广告/带货/知识付费/IP授权变现策略","变现效率优化"]}`, SkillsRequired: []string{"变现策略", "商业模式", "IP运营"}, SeniorityLevel: "P5-P7", SortOrder: 4},

		{Key: "technical_analyst", Name: "技术分析师", DepartmentKey: "equity_research", Description: "负责股票技术面分析，精通K线形态、技术指标（MACD/KDJ/RSI/布林带）、量价关系、支撑阻力位、趋势线、波浪理论、缠论。", ResponsibilitiesJSON: `{"core":["K线形态识别与技术指标分析","量价关系、支撑阻力位、趋势线分析","波浪理论与缠论应用","多周期验证与量价配合确认"]}`, SkillsRequired: []string{"技术分析", "K线形态", "MACD/KDJ/RSI/布林带", "波浪理论/缠论", "量价分析"}, SeniorityLevel: "P6-P8", SortOrder: 1},
		{Key: "fundamental_analyst", Name: "基本面分析师", DepartmentKey: "equity_research", Description: "负责股票基本面分析，精通财务报表分析、估值模型（DCF/PE/PB/PEG/PS）、行业比较、护城河分析。", ResponsibilitiesJSON: `{"core":["财务报表分析（三大表+附注）","估值模型（DCF/PE/PB/PEG/PS）","行业比较与护城河分析","管理层评估与安全边际计算"]}`, SkillsRequired: []string{"财务分析", "估值建模", "行业研究", "护城河分析"}, SeniorityLevel: "P6-P8", SortOrder: 2},
		{Key: "money_flow_analyst", Name: "资金面分析师", DepartmentKey: "equity_research", Description: "负责资金面分析，含主力资金流向、北向资金、融资融券、大宗交易、龙虎榜、板块轮动。", ResponsibilitiesJSON: `{"core":["主力资金流向与北向资金分析","融资融券、大宗交易、龙虎榜分析","板块轮动与资金净流入/流出"]}`, SkillsRequired: []string{"资金面分析", "北向资金", "龙虎榜", "板块轮动"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "news_analyst", Name: "消息面分析师", DepartmentKey: "equity_research", Description: "负责消息面分析，含政策解读、公司公告、行业新闻、宏观事件、突发事件影响评估。", ResponsibilitiesJSON: `{"core":["政策解读与公司公告分析","行业新闻与宏观事件影响评估","突发事件影响量化与情绪传导链分析"]}`, SkillsRequired: []string{"政策解读", "公告分析", "事件影响评估", "信息源可靠性判断"}, SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "sentiment_analyst", Name: "情绪面分析师", DepartmentKey: "equity_research", Description: "负责市场情绪分析，含恐慌贪婪指数、换手率、涨跌比、舆情分析、情绪周期。", ResponsibilitiesJSON: `{"core":["市场情绪指标分析（恐慌贪婪指数/换手率/涨跌比）","舆情分析与投资者行为研究","情绪周期判断与极端信号识别"]}`, SkillsRequired: []string{"情绪分析", "舆情监测", "逆向思维", "情绪周期"}, SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "industry_analyst", Name: "行业分析师", DepartmentKey: "equity_research", Description: "负责行业分析，含行业生命周期、产业链分析、竞争格局、政策影响、技术变革。", ResponsibilitiesJSON: `{"core":["行业生命周期与产业链分析","竞争格局与政策影响评估","技术变革与景气度跟踪"]}`, SkillsRequired: []string{"行业研究", "产业链分析", "竞争格局", "景气度跟踪"}, SeniorityLevel: "P6-P8", SortOrder: 6},
		{Key: "risk_assessor", Name: "风险评估师", DepartmentKey: "equity_research", Description: "负责风险评估，含VaR/CVaR、最大回撤、波动率分析、相关性风险、黑天鹅事件、系统性风险。", ResponsibilitiesJSON: `{"core":["VaR/CVaR计算与最大回撤分析","波动率分析与相关性风险评估","黑天鹅事件与系统性风险预警"]}`, SkillsRequired: []string{"风险管理", "VaR/CVaR", "压力测试", "尾部风险"}, SeniorityLevel: "P5-P7", SortOrder: 7},
		{Key: "quant_researcher", Name: "量化研究员", DepartmentKey: "equity_research", Description: "负责量化策略研究，含因子挖掘（价量/基本面/另类数据）、IC/IR分析、回测框架、组合优化、ML Alpha。", ResponsibilitiesJSON: `{"core":["因子挖掘（价量/基本面/另类数据因子）","IC/IR分析、因子正交化、中性化处理","回测框架（Backtrader/Zipline/VnPy）","组合优化（均值-方差/Black-Litterman/风险平价）","ML Alpha（XGBoost/LightGBM/集成方法）"]}`, SkillsRequired: []string{"因子研究", "IC/IR分析", "回测框架", "组合优化", "机器学习"}, SeniorityLevel: "P6-P8", SortOrder: 8},
		{Key: "data_collector", Name: "数据采集员", DepartmentKey: "equity_research", Description: "负责金融数据采集与管理，含Wind/东方财富/Tushare/AKShare数据源、数据清洗、定时采集。", ResponsibilitiesJSON: `{"core":["数据源管理（Wind/东方财富/Tushare/AKShare）","数据清洗与异常检测","定时采集与数据质量保障"]}`, SkillsRequired: []string{"数据采集", "数据清洗", "API对接"}, SeniorityLevel: "P4-P6", SortOrder: 9},
		{Key: "report_writer", Name: "报告撰写员", DepartmentKey: "equity_research", Description: "负责研究报告撰写，含数据可视化、逻辑组织、合规审查。", ResponsibilitiesJSON: `{"core":["研究报告撰写与数据可视化","逻辑组织与合规审查"]}`, SkillsRequired: []string{"报告撰写", "数据可视化", "合规审查"}, SeniorityLevel: "P5-P7", SortOrder: 10},
		{Key: "trading_coordinator", Name: "交易主控 / 调度员", DepartmentKey: "equity_research", Description: "负责盘前信息汇总与盘中监控协调，含隔夜外盘分析、集合竞价策略、盘中异动捕捉。", ResponsibilitiesJSON: `{"core":["盘前信息汇总与隔夜外盘分析","集合竞价策略与开盘预判","盘中监控与异动捕捉","盘中策略调整与风险控制"]}`, SkillsRequired: []string{"盘前分析", "盘中监控", "策略协调", "风险控制"}, SeniorityLevel: "P6-P8", SortOrder: 11},
		{Key: "quant_developer", Name: "量化开发工程师", DepartmentKey: "quant_trading", Description: "负责量化平台开发，含研究平台架构、数据管线、交易系统（OMS/EMS）。", ResponsibilitiesJSON: `{"core":["研究平台架构（Jupyter/回测引擎/因子计算）","数据管线（实时数据接入/ETL/数据仓库）","交易系统（OMS/EMS/风控/清算）"]}`, SkillsRequired: []string{"量化平台开发", "Python/C++", "OMS/EMS", "数据管线"}, SeniorityLevel: "P6-P8", SortOrder: 1},
		{Key: "algo_trading_engineer", Name: "算法交易工程师", DepartmentKey: "quant_trading", Description: "负责算法交易策略开发，含TWAP/VWAP/POV/Implementation Shortfall、做市策略。", ResponsibilitiesJSON: `{"core":["执行算法（TWAP/VWAP/POV/Implementation Shortfall）","做市策略（库存管理/价差优化/风险对冲）","市场冲击模型与最优执行"]}`, SkillsRequired: []string{"算法交易", "执行优化", "做市策略", "市场微观结构"}, SeniorityLevel: "P6-P8", SortOrder: 2},
		{Key: "low_latency_engineer", Name: "低延迟系统工程师", DepartmentKey: "quant_trading", Description: "负责交易系统低延迟优化，含Linux内核调优、CPU绑定、网络协议栈优化、RDMA/FPGA。", ResponsibilitiesJSON: `{"core":["Linux内核调优（CPU绑定/中断亲和/NUMA优化）","网络协议栈优化（RDMA/FPGA/微波传输/交易所接入）","延迟可测量与优化可验证"]}`, SkillsRequired: []string{"Linux内核调优", "网络优化", "RDMA/FPGA", "低延迟系统"}, SeniorityLevel: "P7-P9", SortOrder: 3},
		{Key: "quant_devops", Name: "量化运维工程师", DepartmentKey: "quant_trading", Description: "负责交易系统运维，含监控告警、灾备切换、容量规划、7x24可用性保障。", ResponsibilitiesJSON: `{"core":["交易系统运维与监控告警","灾备切换与容量规划","7x24可用性保障与变更管控"]}`, SkillsRequired: []string{"运维", "监控告警", "灾备", "容量规划"}, SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "fixed_income_analyst", Name: "固收分析师", DepartmentKey: "fixed_income", Description: "负责债券定价、久期凸性分析、收益率曲线、信用利差、利率互换、信用评级。", ResponsibilitiesJSON: `{"core":["债券定价与久期凸性分析","收益率曲线与信用利差分析","信用评级与违约概率评估"]}`, SkillsRequired: []string{"固收分析", "债券定价", "信用评级", "利率分析"}, SeniorityLevel: "P6-P8", SortOrder: 1},
		{Key: "derivatives_pricer", Name: "衍生品定价工程师", DepartmentKey: "fixed_income", Description: "负责期权定价（Black-Scholes/Monte Carlo/二叉树）、Greeks、波动率曲面、期货策略。", ResponsibilitiesJSON: `{"core":["期权定价（Black-Scholes/Monte Carlo/二叉树）","Greeks计算与波动率曲面分析","期货策略（基差交易/套利/跨期跨品种）"]}`, SkillsRequired: []string{"期权定价", "Monte Carlo模拟", "波动率建模", "期货策略"}, SeniorityLevel: "P6-P8", SortOrder: 2},
		{Key: "compliance_officer", Name: "合规专员", DepartmentKey: "compliance_risk", Description: "负责金融监管合规，含证券法规、内幕交易防范、信息披露、投资者适当性、反洗钱。", ResponsibilitiesJSON: `{"core":["证券法规合规与内幕交易防范","信息披露与投资者适当性管理","合规红线与违规零容忍"]}`, SkillsRequired: []string{"证券法规", "合规管理", "信息披露", "内控"}, SeniorityLevel: "P5-P8", SortOrder: 1},
		{Key: "risk_manager", Name: "风险管理师", DepartmentKey: "compliance_risk", Description: "负责市场风险与信用风险管理，含VaR/CVaR、压力测试、情景分析、信用评分。", ResponsibilitiesJSON: `{"core":["市场风险管理（VaR/CVaR/压力测试/情景分析）","信用风险管理（信用评分/违约模型/敞口管理）","风险限额与集中度风险控制"]}`, SkillsRequired: []string{"风险管理", "VaR/CVaR", "压力测试", "信用风险"}, SeniorityLevel: "P6-P8", SortOrder: 2},
		{Key: "aml_specialist", Name: "反洗钱专员", DepartmentKey: "compliance_risk", Description: "负责反洗钱工作，含KYC/KYB、可疑交易监测、制裁名单筛查、STR/SAR报告。", ResponsibilitiesJSON: `{"core":["KYC/KYB客户尽职调查","可疑交易监测与制裁名单筛查","STR/SAR报告与保密纪律"]}`, SkillsRequired: []string{"反洗钱", "KYC/KYB", "可疑交易监测", "制裁筛查"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "investment_advisor", Name: "投资顾问", DepartmentKey: "wealth_mgmt", Description: "负责投资顾问服务，含资产配置建议、风险评估、客户需求分析、适当性匹配。", ResponsibilitiesJSON: `{"core":["资产配置建议与组合推荐","客户需求分析与适当性匹配","风险匹配与信息披露"]}`, SkillsRequired: []string{"投资顾问", "资产配置", "客户服务", "适当性管理"}, SeniorityLevel: "P6-P8", SortOrder: 1},
		{Key: "asset_allocator", Name: "资产配置师", DepartmentKey: "wealth_mgmt", Description: "负责战略/战术资产配置，含SAA/TAA、再平衡策略、多资产组合。", ResponsibilitiesJSON: `{"core":["战略资产配置(SAA)与战术资产配置(TAA)","再平衡策略与多资产组合","分散化与成本效率优化"]}`, SkillsRequired: []string{"资产配置", "SAA/TAA", "再平衡", "多资产组合"}, SeniorityLevel: "P7-P9", SortOrder: 2},
		{Key: "client_profiler", Name: "财富画像师", DepartmentKey: "wealth_mgmt", Description: "负责高净值客户画像、家族财富规划、税务优化、传承规划。", ResponsibilitiesJSON: `{"core":["高净值客户画像与KYC问卷","家族财富规划与税务优化","传承规划与隐私保护"]}`, SkillsRequired: []string{"客户画像", "财富规划", "税务优化", "隐私保护"}, SeniorityLevel: "P5-P7", SortOrder: 3},
		{Key: "fintech_product_manager", Name: "金融科技产品经理", DepartmentKey: "fintech", Description: "负责金融科技产品设计，含监管科技(RegTech)、支付系统、数字银行。", ResponsibilitiesJSON: `{"core":["金融科技产品设计","监管科技(RegTech)与支付系统","合规优先与数据驱动"]}`, SkillsRequired: []string{"产品管理", "金融科技", "RegTech", "支付系统"}, SeniorityLevel: "P5-P7", SortOrder: 1},
		{Key: "financial_data_engineer", Name: "金融数据工程师", DepartmentKey: "fintech", Description: "负责实时行情数据、Tick数据存储、数据湖、流式计算。", ResponsibilitiesJSON: `{"core":["实时行情数据接入与协议解析","数据湖与流式计算","数据质量与时效性保障"]}`, SkillsRequired: []string{"数据工程", "实时数据", "流式计算", "数据湖"}, SeniorityLevel: "P5-P7", SortOrder: 2},
		{Key: "blockchain_analyst", Name: "区块链分析师", DepartmentKey: "fintech", Description: "负责链上数据分析、DeFi协议分析、代币经济学、智能合约审计。", ResponsibilitiesJSON: `{"core":["链上数据分析与DeFi协议分析","代币经济学与智能合约审计","风险警示与合规意识"]}`, SkillsRequired: []string{"区块链分析", "DeFi", "智能合约", "链上数据"}, SeniorityLevel: "P5-P7", SortOrder: 3},
	}

	for _, pos := range positions {
		if *dryRun {
			fmt.Printf("[dry-run] upsert position: %s/%s\n", pos.DepartmentKey, pos.Key)
			continue
		}
		result, err := posUC.UpsertByKey(ctx, pos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert position %s/%s: %v\n", pos.DepartmentKey, pos.Key, err)
			continue
		}
		fmt.Printf("upserted position: %s/%s (id=%s)\n", result.DepartmentKey, result.Key, result.ID)
	}

	if !*dryRun {
		fmt.Println("done")
	}
}

func resolveSQLitePath() string {
	path := os.Getenv("ARANEA_SQLITE_PATH")
	if path == "" {
		path = "data/arenea.sqlite"
	}
	return path
}

func ensureTables(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS industries (id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, name TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', scenario_key TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS departments (id TEXT PRIMARY KEY, key TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', industry_key TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', responsibilities_json TEXT NOT NULL DEFAULT '{}', sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '', UNIQUE(key, industry_key))`,
		`CREATE INDEX IF NOT EXISTS idx_departments_industry_key ON departments(industry_key)`,
		`CREATE TABLE IF NOT EXISTS positions (id TEXT PRIMARY KEY, key TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', department_key TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', responsibilities_json TEXT NOT NULL DEFAULT '{}', skills_required_json TEXT NOT NULL DEFAULT '[]', seniority_level TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '', UNIQUE(key, department_key))`,
		`CREATE INDEX IF NOT EXISTS idx_positions_department_key ON positions(department_key)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	return nil
}

package biz

// PGO-2: FieldGuide schema — authoritative Go registry for all prompt field guides.
// The same data drives: UI placeholders, character budget indicators, AI refine meta-prompts,
// and CLI import YAML → markdown resolution. See design doc §2.

// FieldScope identifies which form field a guide applies to.
type FieldScope string

const (
	ScopeCategoryIndustry   FieldScope = "category.industry"   // level=1
	ScopeCategoryDepartment FieldScope = "category.department" // level=2
	ScopeCategoryPosition   FieldScope = "category.position"   // level=3
	ScopeAgentDescription   FieldScope = "agent.description"
	ScopeAgentFile          FieldScope = "agent.file"   // use FileName to distinguish files
	ScopeSpecExtract        FieldScope = "spec_extract" // PGO-4 B-2: markdown → YAML org spec
)

// FieldGuide is the comprehensive metadata for one editable prompt field.
type FieldGuide struct {
	Scope       FieldScope
	FileName    string // only set for ScopeAgentFile; e.g. "AGENTS_CORE.md"
	TitleZh     string // UI card title (Chinese)
	Purpose     string // one-sentence purpose
	ShouldWrite []string
	ShouldAvoid []string
	Examples    []GuideExample
	Budget      GuideBudget
	Placeholder string // textarea placeholder shown when empty
	DefaultStub string // body seeded for new agents/categories
}

// GuideExample is one industry-tagged example body.
type GuideExample struct {
	Industry string // "电商" | "医疗" | "教育" | "通用" etc.
	Body     string
}

// GuideBudget defines character soft/hard limits for the field.
type GuideBudget struct {
	Soft int // show yellow warning above Soft
	Hard int // block save / show red above Hard (0 = no hard limit)
}

// FieldGuideKey is the composite registry key.
type FieldGuideKey struct {
	Scope    FieldScope
	FileName string
}

var (
	fieldGuideRegistry = map[FieldGuideKey]FieldGuide{}
	fieldGuideOrder    []FieldGuideKey // insertion order for stable iteration (Q-7)
)

func register(g FieldGuide) {
	k := FieldGuideKey{g.Scope, g.FileName}
	if _, exists := fieldGuideRegistry[k]; !exists {
		fieldGuideOrder = append(fieldGuideOrder, k)
	}
	fieldGuideRegistry[k] = g
}

// GetFieldGuide returns the guide for the given scope/file pair.
func GetFieldGuide(scope FieldScope, fileName string) (FieldGuide, bool) {
	g, ok := fieldGuideRegistry[FieldGuideKey{scope, fileName}]
	return g, ok
}

// ListFieldGuides returns all registered guides in stable insertion order.
// Q-7 fix: previously this iterated the map directly, which Go does in
// randomized order — making CLI output and test snapshots flaky.
func ListFieldGuides() []FieldGuide {
	out := make([]FieldGuide, 0, len(fieldGuideOrder))
	for _, k := range fieldGuideOrder {
		out = append(out, fieldGuideRegistry[k])
	}
	return out
}

// GetFieldGuidesForScope returns all guides matching the given scope, in
// stable insertion order.
func GetFieldGuidesForScope(scope FieldScope) []FieldGuide {
	var out []FieldGuide
	for _, k := range fieldGuideOrder {
		if k.Scope == scope {
			out = append(out, fieldGuideRegistry[k])
		}
	}
	return out
}

func init() {
	// ─── Category level guides ────────────────────────────────────────────────

	register(FieldGuide{
		Scope:   ScopeCategoryIndustry,
		TitleZh: "行业说明",
		Purpose: "描述这个行业的特点、监管约束、客户特征，为所有子部门和岗位提供共同背景。",
		ShouldWrite: []string{
			"行业核心特征（2–3 句）",
			"主要监管 / 合规约束",
			"典型客户画像",
			"与相邻行业的边界",
		},
		ShouldAvoid: []string{
			"具体岗位职责（请在岗位层填写）",
			"Agent 话术与人设（请在 IDENTITY.md）",
			"流程 SOP（请在 AGENTS_TASK.md）",
		},
		Examples: []GuideExample{
			{Industry: "电商", Body: "面向 C 端消费者的在线零售平台，受《电子商务法》约束；用户对比价格敏感，退换货率较高。"},
			{Industry: "医疗", Body: "院内 / 院外健康信息服务，须遵循 HIPAA/互联网医疗相关法规，禁止替代医嘱。"},
		},
		Budget:      GuideBudget{Soft: 400, Hard: 600},
		Placeholder: "请简述本行业的特点、监管约束与客户画像（建议 100–400 字）",
		DefaultStub: "# 行业说明\n\n（请描述行业特点、监管约束与客户画像）",
	})

	register(FieldGuide{
		Scope:   ScopeCategoryDepartment,
		TitleZh: "部门职责",
		Purpose: "描述部门的核心使命、团队协作边界与主要输入/输出，为所有子岗位提供上下文。",
		ShouldWrite: []string{
			"部门使命（1–2 句）",
			"核心输入（谁传递任务给本部门）",
			"核心输出（本部门产出什么）",
			"与其他部门的协作边界",
		},
		ShouldAvoid: []string{
			"具体岗位的细粒度流程",
			"Agent 个体的话术偏好",
		},
		Examples: []GuideExample{
			{Industry: "电商", Body: "售后服务部：负责处理用户订单纠纷、退款退货及满意度管理；接受业务部门派单，输出已解决工单与用户评价。"},
		},
		Budget:      GuideBudget{Soft: 500, Hard: 800},
		Placeholder: "请简述部门使命、核心输入输出与协作边界（建议 100–500 字）",
		DefaultStub: "# 部门职责\n\n（请描述部门使命、输入输出与协作边界）",
	})

	register(FieldGuide{
		Scope:   ScopeCategoryPosition,
		TitleZh: "岗位职责",
		Purpose: "这个岗位的核心职责清单、能/不能做的边界、典型工作流、KPI。注入 system instruction 的 L1 层。",
		ShouldWrite: []string{
			"主要职责：3–5 条 bullet",
			"工作边界：能做什么 / 不能做什么各 2–3 条",
			"典型流程：1–3 个常见 workflow 标题",
			"关键 KPI：1–3 条可衡量指标",
		},
		ShouldAvoid: []string{
			"复制 Agent 的话术（请去 agent_description 或 IDENTITY.md）",
			"长流程 SOP（请去 AGENTS_TASK.md）",
			"跨行业通用知识（请去行业说明）",
		},
		Examples: []GuideExample{
			{Industry: "电商", Body: "1) 处理售后退换货：核实订单→判定责任→执行方案\n2) 不可承诺超出政策的赔付\n3) KPI：首响应时长 < 3 min，解决率 > 85%"},
			{Industry: "医疗", Body: "1) 提供健康信息查询\n2) 不可给出诊断或处方建议\n3) 必要时引导用户就医"},
		},
		Budget:      GuideBudget{Soft: 800, Hard: 1000},
		Placeholder: "示例：1) 处理售后退换货 ... 2) 不可承诺超出政策的赔付 ...",
		DefaultStub: "# 岗位职责\n\n（请描述本岗位的主要职责、边界、流程与 KPI）",
	})

	// ─── Agent description ────────────────────────────────────────────────────

	register(FieldGuide{
		Scope:   ScopeAgentDescription,
		TitleZh: "Agent 描述（个体定位）",
		Purpose: "这个 Agent 个体的称呼、擅长场景、关键约束；是 L2 层，在 L1 岗位职责之后注入。",
		ShouldWrite: []string{
			"称呼与身份（1 句）",
			"擅长的 3–5 个具体场景",
			"关键约束或前置条件（1–3 条）",
		},
		ShouldAvoid: []string{
			"与岗位职责重复的内容（L1 已有）",
			"超过 600 字的长 SOP（请去 AGENTS_TASK.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "我是「小虾米」，专注处理中文淘宝退换货纠纷。我会核实订单后快速判断责任方，给出最优解决方案。我不能替代人工处理升级投诉或超权限赔付。"},
		},
		Budget:      GuideBudget{Soft: 400, Hard: 600},
		Placeholder: "请简述这个 Agent 的称呼、擅长场景与关键约束（建议 100–400 字）",
		DefaultStub: "",
	})

	// ─── Prompt files ─────────────────────────────────────────────────────────

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "AGENTS_CORE.md",
		TitleZh:  "核心角色说明",
		Purpose:  "固化 Agent 的首要原则、服务宗旨与模型行为偏好；每次对话必读。",
		ShouldWrite: []string{
			"首要使命（2–4 句）",
			"核心行为原则（3–6 条）",
			"模型推理偏好（如：先思考再回复；分步骤输出）",
		},
		ShouldAvoid: []string{
			"任务级 SOP（见 AGENTS_TASK.md）",
			"个性/话术（见 IDENTITY.md）",
			"工具白名单（见 CAPABILITIES.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# AGENTS_CORE\n我是高效、诚实的 AI 助手，核心使命是帮助用户解决问题并持续学习。\n\n## 原则\n1. 事实优先，避免幻觉\n2. 对不确定的内容说明置信度\n3. 用用户语言回复"},
		},
		Budget:      GuideBudget{Soft: 600, Hard: 1200},
		Placeholder: "描述 Agent 的首要原则、服务宗旨与模型行为偏好",
		DefaultStub: "# AGENTS_CORE\n\n（请描述 Agent 的核心角色、首要原则、模型偏好）",
	})

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "AGENTS_TASK.md",
		TitleZh:  "任务模式说明",
		Purpose:  "在 task 模式下需要的额外上下文：目标、输出格式、SOP、协作。",
		ShouldWrite: []string{
			"任务目标与输出契约（期望输出是什么格式）",
			"标准 SOP（2–5 步流程）",
			"跨 Agent 协作机制（如有）",
			"常见异常处理方式",
		},
		ShouldAvoid: []string{
			"对话式互动话术（见 IDENTITY.md）",
			"核心原则（见 AGENTS_CORE.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# AGENTS_TASK\n## 目标\n完成用户指定的信息搜集任务并以结构化 markdown 输出。\n## SOP\n1. 理解需求\n2. 拆解子任务\n3. 并行搜集\n4. 汇总输出"},
		},
		Budget:      GuideBudget{Soft: 800, Hard: 1600},
		Placeholder: "描述任务目标、输出格式、SOP 与协作机制",
		DefaultStub: "# AGENTS_TASK\n\n（请描述任务目标、输出契约、协作机制与典型 SOP）",
	})

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "IDENTITY.md",
		TitleZh:  "身份与人设",
		Purpose:  "固化 Agent 的名字、语气、口头禅、不可妥协人设；Evolution 会更新 ## Persona 段。",
		ShouldWrite: []string{
			"Agent 的称呼（name / alias）",
			"语气风格（正式 / 随和 / 幽默 ...）",
			"口头禅或标志性表达（可选）",
			"## Persona 段（Evolution 专属，勿手写过多内容）",
		},
		ShouldAvoid: []string{
			"岗位职责（L1 已注入）",
			"工具调用规则（见 CAPABILITIES.md）",
			"大段 SOP（见 AGENTS_TASK.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# IDENTITY\n我叫「小问」，是一名友善、专业的 AI 客服。\n语气：亲切但专业，使用中文。\n\n## Persona\n（由 Evolution 自动维护，请勿手动大幅修改此段）"},
		},
		Budget:      GuideBudget{Soft: 500, Hard: 800},
		Placeholder: "描述 Agent 的名字、语气、口头禅与关键人设限制",
		DefaultStub: "# IDENTITY\n\n（请描述 Agent 的称呼、语气、口头禅、不可妥协人设）\n\n## Persona\n\n（人设细节写在此处，由 Evolution 自动更新）",
	})

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "CAPABILITIES.md",
		TitleZh:  "能力与工具",
		Purpose:  "列出 Agent 有权调用的工具、Skill 与知识库；构建期压制运行时 cue 中的重复枚举。",
		ShouldWrite: []string{
			"工具白名单（可用 bullet 列出工具名称）",
			"禁用工具 / 黑名单（如有）",
			"知识库范围说明（如有 L3 recall）",
			"Skill 名称与用途（如有）",
		},
		ShouldAvoid: []string{
			"工具调用规则（见 RULE.md）",
			"任务流程（见 AGENTS_TASK.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# CAPABILITIES\n## 可用工具\n- search: 网络检索\n- calculator: 四则运算\n\n## 禁用\n- file_write（只读模式）"},
		},
		Budget:      GuideBudget{Soft: 600, Hard: 1000},
		Placeholder: "列出工具白名单、Skill 与知识库范围",
		DefaultStub: "# CAPABILITIES\n\n（请列出工具白名单、Skill 列表、能力边界）",
	})

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "RULE.md",
		TitleZh:  "规则与合规",
		Purpose:  "列出禁止行为、安全边界、合规要求与降级策略；每次对话必读。",
		ShouldWrite: []string{
			"绝对禁止列表（bullet，简洁）",
			"合规要求（法规条目或行业规范）",
			"违规降级策略（遇到违禁请求如何处理）",
			"数据隐私边界（如有）",
		},
		ShouldAvoid: []string{
			"推荐行为（去 AGENTS_CORE.md）",
			"任务 SOP（去 AGENTS_TASK.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# RULE\n## 禁止\n- 提供医疗诊断\n- 泄露用户隐私\n- 绕过平台安全策略\n\n## 违规处理\n遇到违禁请求，礼貌拒绝并提示用户联系人工。"},
		},
		Budget:      GuideBudget{Soft: 500, Hard: 800},
		Placeholder: "列出禁止行为、合规要求与降级策略",
		DefaultStub: "# RULE\n\n（请列出禁止行为、合规要求、降级策略）",
	})

	register(FieldGuide{
		Scope:    ScopeAgentFile,
		FileName: "USER_CONTEXT.md",
		TitleZh:  "用户上下文（可选）",
		Purpose:  "记录用户的长期偏好、历史与个性化设置；替代已废弃的 USER.md + USER_PREDEFINED.md。",
		ShouldWrite: []string{
			"用户的称呼偏好（如有）",
			"已知偏好或习惯",
			"持久化的个性化配置",
		},
		ShouldAvoid: []string{
			"本轮对话信息（应在 message history 中）",
			"Agent 规则（去 RULE.md）",
		},
		Examples: []GuideExample{
			{Industry: "通用", Body: "# USER_CONTEXT\n用户偏好使用简体中文；对技术细节有较强理解力；禁止在回复中使用表情符号。"},
		},
		Budget:      GuideBudget{Soft: 400, Hard: 600},
		Placeholder: "记录用户长期偏好、历史与个性化设置（可选文件）",
		DefaultStub: "# USER_CONTEXT\n\n（记录用户的长期偏好、历史与个性化设置）",
	})

	// ─── PGO-4 B-2: spec extraction (markdown → YAML org spec) ───────────────
	// This scope is consumed by `aranea import org --file *.md`. The PromptRefiner
	// branches on Scope==ScopeSpecExtract to use a YAML-producing system prompt
	// rather than the standard refinement prompt. Budget is intentionally large
	// because YAML output may be substantial.
	register(FieldGuide{
		Scope:       ScopeSpecExtract,
		TitleZh:     "组织结构抽取",
		Purpose:     "将自由 markdown 描述转换为 Aranea import 用的 YAML org spec。",
		Budget:      GuideBudget{Soft: 8000, Hard: 20000},
		Placeholder: "粘贴公司 / 部门 / 岗位 / Agent 描述的 markdown 文档。",
	})
}

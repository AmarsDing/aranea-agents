package chunktemplate

// Builtin 返回内置模板目录（≥8 条，常量数据，可安全共享）。
//
// 目录顺序即同优先级竞争时的 tie-break：Resolver 在同一优先级内按顺序取首个
// 命中。manual 是兜底模板（DefaultTemplateID），其提示词保持保守（"手册/说明书"
// 等强特征词），避免在文件名回退级误吞其它类型。
//
// OrganizeMarkdown 的取舍：长叙述文（手册/制度/合同/报告/论文/FAQ）先经 LLM
// 组织成 Markdown 再切分收益大；表格走 json 策略（入参已是结构化行）、代码
// 须保留原始排版，两者跳过组织（false）。
func Builtin() []Template {
	return []Template{
		{
			ID:    "manual",
			Label: "手册",
			Description: "产品/操作手册：章节层级稳定，按 Markdown 结构切分，" +
				"900/120 兼顾上下文完整与召回粒度。",
			DocTypeHints:     []string{"manual", "handbook", "guide", "手册", "说明书", "指南"},
			Strategy:         StrategyMarkdown,
			ChunkSize:        900,
			Overlap:          120,
			OrganizeMarkdown: true,
		},
		{
			ID:    "policy",
			Label: "制度",
			Description: "规章制度/管理规范：条款化结构，Markdown 按条款标题切分，" +
				"800/100 保持单条制度完整。",
			DocTypeHints:     []string{"policy", "regulation", "制度", "规章", "规范"},
			Strategy:         StrategyMarkdown,
			ChunkSize:        800,
			Overlap:          100,
			OrganizeMarkdown: true,
		},
		{
			ID:    "contract",
			Label: "合同",
			Description: "合同/协议：编号条款嵌套深，用 recursive 按分隔符递归切分，" +
				"700/80 避免条款被腰斩。",
			DocTypeHints:     []string{"contract", "agreement", "合同", "协议"},
			Strategy:         StrategyRecursive,
			ChunkSize:        700,
			Overlap:          80,
			OrganizeMarkdown: true,
		},
		{
			ID:    "report",
			Label: "报告",
			Description: "经营/财务/业务报告：段落较长且图文混排，Markdown 切分，" +
				"1000/150 保住论述单元。",
			DocTypeHints:     []string{"report", "报告", "财报", "年报", "月报"},
			Strategy:         StrategyMarkdown,
			ChunkSize:        1000,
			Overlap:          150,
			OrganizeMarkdown: true,
		},
		{
			ID:    "paper",
			Label: "论文",
			Description: "论文/白皮书：论述密度高、引用密集，Markdown 切分，" +
				"1200/150 保留整段论证链。",
			DocTypeHints:     []string{"paper", "whitepaper", "论文", "白皮书"},
			Strategy:         StrategyMarkdown,
			ChunkSize:        1200,
			Overlap:          150,
			OrganizeMarkdown: true,
		},
		{
			ID:    "table",
			Label: "表格",
			Description: "电子表格（xlsx/csv）：抽取层已转成结构化行/JSON，用 json " +
				"策略按记录边界切分、零重叠；不组织成 Markdown（会打散行列对齐）。",
			DocTypeHints: []string{"table", "表格"},
			MIMEHints: []string{
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.ms-excel",
				"text/csv",
				".xlsx", ".xls", ".csv",
			},
			Strategy:  StrategyJSON,
			ChunkSize: 600,
			Overlap:   0,
			// 注意 OrganizeMarkdown=false：表格抽取产物本身即结构化，LLM 重组反而
			// 引入幻觉行；与 vault 链路的表格摄取口径一致。
			OrganizeMarkdown: false,
		},
		{
			ID:    "faq",
			Label: "FAQ",
			Description: "常见问题集：一问一答是天然切分单元，当前以 char 小窗" +
				"（400/40）兜底，保证问答对不被腰斩。",
			DocTypeHints: []string{"faq", "问答"},
			Strategy:     StrategyChar,
			ChunkSize:    400,
			Overlap:      40,
			// TODO(U2-后续 QA 拆分钩子)：识别「问：/答：」前缀或 Markdown 问答标题后
			// 按对切分（可在 Template 增加 QASplitter 提示字段，由 chunking 管线消费）。
			OrganizeMarkdown: true,
		},
		{
			ID:    "code",
			Label: "代码",
			Description: "源代码/脚本：以扩展名为主要提示，char 定长切分（600/60）；" +
				"不做 Markdown 组织，避免 LLM 改写代码语义。",
			DocTypeHints: []string{
				"code", "源码", "代码",
				".go", ".py", ".js", ".ts", ".tsx", ".jsx",
				".java", ".kt", ".scala",
				".c", ".h", ".cpp", ".cc", ".hpp", ".cs",
				".rs", ".rb", ".php", ".sh", ".bat", ".ps1",
				".sql", ".lua", ".pl", ".r", ".m", ".swift",
			},
			Strategy:         StrategyChar,
			ChunkSize:        600,
			Overlap:          60,
			OrganizeMarkdown: false,
		},
	}
}

// Default 返回兜底模板（Builtin 中的 manual）；目录被裁到缺失兜底时返回零值
// ——NewResolver 会在构造期拒绝这种目录，DefaultResolver 据此是安全的。
func Default() Template {
	for _, t := range Builtin() {
		if t.ID == DefaultTemplateID {
			return t
		}
	}
	return Template{}
}

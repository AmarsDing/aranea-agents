package skillrouter

// TaxonomyLeaves returns built-in intent→taxonomy leaf definitions for keyword routing (layer B).
// Platform Skills align via metadata_json.taxonomy_paths (see docs/需求/20 skill struct design.md §十四).
func TaxonomyLeaves() []Leaf {
	return []Leaf{
		{
			Path: `数据获取与集成/内部数据源/文件系统读取（读取表格）`,
			Keywords: []string{
				"表格", "xlsx", "excel", "csv", "读取文件", "读表", "工作表", "spreadsheet", "worksheet",
			},
		},
		{
			Path: `分析与推理/自然语言理解（情感分析）`,
			Keywords: []string{
				"情感", "情绪", "sentiment", "正负向", "nlp",
			},
		},
		{
			Path: `交互与执行/消息发送（发邮件）`,
			Keywords: []string{
				"邮件", "email", "smtp", "发信", "信箱", "e-mail",
			},
		},
	}
}

// Leaf is one taxonomy leaf path plus recall keywords (matched against user query).
type Leaf struct {
	Path     string
	Keywords []string
}

package toolapi

// Meta 描述单个工具的对外标识与中文明示（文档与调试）。
type Meta struct {
	Name        string `json:"name"`         // ADK/OpenAI function 英文名，唯一
	TitleZh     string `json:"title_zh"`     // 中文简短标题
	SummaryZh   string `json:"summary_zh"`   // 中文一句话作用说明
	Description string `json:"description"` // 英文描述（与原有 ADK/OpenAI prompt 对齐，供模型读懂）
}

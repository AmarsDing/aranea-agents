package toolapi

// InvokeRequest 是单一工具的统一进程内调用入参格式（不要求 JSON）。
type InvokeRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"` // OpenAI/adk FunctionCall args 等价结构（map）
}

// InvokeResponse 为统一出参信封：便于 HTTP、日志与同构 JSON。
type InvokeResponse struct {
	OK     bool           `json:"ok"`
	Error  string         `json:"error,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

// ErrorResponse 封装错误为非 panic 的失败信封。
func ErrorResponse(msg string) InvokeResponse {
	return InvokeResponse{OK: false, Error: msg}
}

// SuccessResponse 封装成功返回值。
func SuccessResponse(result map[string]any) InvokeResponse {
	return InvokeResponse{OK: true, Result: result}
}

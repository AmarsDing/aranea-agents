package service

// firstNonEmptyString 返回入参中第一个非空串；若均为空则返回 ""。
//（曾位于 chat_service；chat 已迁至 conversation/application 后，非 Conversation 的 service 子包仍复用。）
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

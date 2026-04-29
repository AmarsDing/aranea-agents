package httpx

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ListResponse 为 { "items": [...] } 列表响应，供 transport 与各 Context HTTP 适配器复用。
type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// WriteJSON 设置 JSON Content-Type 并编码 body。
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteErr 使用 PublicError 映射错误并写入 JSON 错误体。
func WriteErr(w http.ResponseWriter, status int, err error) {
	status, message := PublicError(status, err)
	WriteJSON(w, status, ErrorResponse{Error: message})
}

// DecodeBody 将 JSON 请求体解码到 target；失败时写 400 并返回 false。
func DecodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		WriteErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

// MethodNotAllowed 写 405 与标准 JSON 错误体。
func MethodNotAllowed(w http.ResponseWriter) {
	WriteJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
}

// IDFromPath 在去掉 prefix 后返回路径首段（去两侧斜杠）。
func IDFromPath(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

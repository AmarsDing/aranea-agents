package transport

import (
	"net/http"

	httpx "arenea/backend/internal/kernel/pkg/httpx"
)

// listResponse 为 JSON 列表响应体，与 kernel/pkg/httpx.ListResponse 同形（迁移 #28b 兼容 transport 内既有引用）。
type listResponse[T any] = httpx.ListResponse[T]

func writeJSON(w http.ResponseWriter, status int, data any) {
	httpx.WriteJSON(w, status, data)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	httpx.WriteErr(w, status, err)
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	return httpx.DecodeBody(w, r, target)
}

func methodNotAllowed(w http.ResponseWriter) {
	httpx.MethodNotAllowed(w)
}

func idFromPath(path, prefix string) string {
	return httpx.IDFromPath(path, prefix)
}

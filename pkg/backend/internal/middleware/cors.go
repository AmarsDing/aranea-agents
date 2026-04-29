package middleware

import (
	"net/http"

	appmw "arenea/backend/internal/app/middleware"
)

// CORS 委托 arenea/backend/internal/app/middleware（P7 #29）；P8 可改为直接引用 app 层并移除此重导出。
func CORS(next http.Handler) http.Handler {
	return appmw.CORS(next)
}

// Package middleware hosts the global driving middleware applied to the
// composed /api/v1 router. CORS 实现于本包（迁移 #29）；RateLimit/BasicAuth 等
// 仍在 arenea/backend/internal/middleware 直至 P8 收紧。Per-Context middleware
// lives inside the Context's own adapters/http package.
package middleware

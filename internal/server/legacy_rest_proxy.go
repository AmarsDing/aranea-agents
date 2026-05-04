package server

import (
	"net/http"

	httpm "github.com/go-kratos/kratos/v2/transport/http"
)

// LegacyRESTProxyFilter was used to forward **`/v1/skills/import*`** to **`LEGACY_REST_ORIGIN`** before skill import lived in cmd/admin.
// **Chat** uses **RegisterLegacyChatForwardHTTPServer**; **skills import** uses **RegisterSkillImportHTTPServer**.
// Keep this filter as a no-op passthrough so HTTP middleware ordering stays stable; add new rewrite rules here only if another unmigrated **`/v1/...`** surface appears.
func LegacyRESTProxyFilter() httpm.FilterFunc {
	return func(next http.Handler) http.Handler { return next }
}

package tools

import (
	"aranea-agents/internal/workspace"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func (d *ToolDecorator) toolName() string {
	if d == nil || d.inner == nil {
		return ""
	}
	if decl := d.inner.Declaration(); decl != nil {
		return decl.Name
	}
	return ""
}

// cacheScopeFromCtx returns an identity scope for tool-result caching.
// Without an Invocation Session, caching is disabled (ok=false) so unscoped
// calls cannot poison or read a shared cross-tenant bucket.
// C-03: scope includes workspace ID so results never leak across tenants
// that share the same session/user identifiers.
func cacheScopeFromCtx(ctx context.Context) (scope string, ok bool) {
	inv, has := trpcagent.InvocationFromContext(ctx)
	if !has || inv == nil || inv.Session == nil {
		return "", false
	}
	s := inv.Session
	if strings.TrimSpace(s.ID) == "" && strings.TrimSpace(s.UserID) == "" {
		return "", false
	}
	wsID := workspace.IDFromContext(ctx)
	return wsID + "\x00" + s.AppName + "\x00" + s.UserID + "\x00" + s.ID, true
}

func (d *ToolDecorator) cacheKey(scope string, jsonArgs []byte) string {
	name := d.toolName()
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(":"))
	h.Write([]byte(scope))
	h.Write([]byte(":"))
	h.Write(jsonArgs)
	return hex.EncodeToString(h.Sum(nil))
}

func (d *ToolDecorator) lookupCache(scope string, jsonArgs []byte) (any, bool) {
	key := d.cacheKey(scope, jsonArgs)
	ttl := d.cacheTTL()
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	e, ok := d.cache[key]
	if !ok {
		return nil, false
	}
	if ttl > 0 && time.Since(e.at) > ttl {
		delete(d.cache, key)
		return nil, false
	}
	return e.value, true
}

func (d *ToolDecorator) storeCache(scope string, jsonArgs []byte, result any) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if len(d.cache) >= decoratorCacheMaxEntries {

		d.cache = make(map[string]cacheEntry)
	}
	d.cache[d.cacheKey(scope, jsonArgs)] = cacheEntry{value: result, at: time.Now()}
}

func (d *ToolDecorator) cacheTTL() time.Duration {
	if d.cfg.CacheTTL < 0 {
		return 0
	}
	if d.cfg.CacheTTL == 0 {
		return DefaultCacheTTL
	}
	return d.cfg.CacheTTL
}

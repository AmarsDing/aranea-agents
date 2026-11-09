package data

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakePermLoader 可编程的 RolePermissionLoader:按调用序返回快照/错误。
type fakePermLoader struct {
	mu      sync.Mutex
	snap    map[string][]string
	err     error
	calls   int
	lastCtx context.Context
}

func (f *fakePermLoader) LoadRolePermissions(ctx context.Context) (map[string][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	// 深拷贝,防调用方篡改快照
	out := make(map[string][]string, len(f.snap))
	for role, codes := range f.snap {
		out[role] = append([]string(nil), codes...)
	}
	return out, nil
}

func (f *fakePermLoader) setSnap(m map[string][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap = m
}

func refreshOrDie(t *testing.T, r *RolePermissionResolver) {
	t.Helper()
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}

func permSet(r *RolePermissionResolver, roles ...string) map[string]struct{} {
	return r.PermissionsForRoles(roles)
}

func hasCode(set map[string]struct{}, code string) bool {
	_, ok := set[code]
	return ok
}

// DB 权威:在库角色的授权以 role_permissions 边为准,即便与内置映射不同。
func TestPermResolver_DBAuthoritativeForSeededRole(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		// 种子角色 viewer 被管理员收窄为只剩 ceoboard.view(内置含 system.monitor)
		"viewer": {"ceoboard.view"},
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	got := permSet(r, "viewer")
	if !hasCode(got, "ceoboard.view") {
		t.Fatalf("DB 授权丢失: %v", got)
	}
	if hasCode(got, "system.monitor") {
		t.Fatalf("DB 收窄后内置授权不得复活: %v", got)
	}
}

// 内置兜底:不在库中的角色回退内置静态映射(bootstrap 期/未播种新角色);
// 完全未知的角色不贡献任何权限。
func TestPermResolver_BuiltinFallbackForAbsentRole(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		"custom-role": {"knowledge.read"},
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	got := permSet(r, "custom-role", "superadmin", "no-such-role")
	if !hasCode(got, "knowledge.read") {
		t.Fatalf("自定义角色授权丢失: %v", got)
	}
	if !hasCode(got, "*") {
		t.Fatalf("未入库内置角色应回退内置映射(superadmin=*): %v", got)
	}
	// no-such-role 不贡献权限:集合应恰好两个码
	if len(got) != 2 {
		t.Fatalf("未知角色不得贡献权限: %v", got)
	}
}

// 零权限边占位:角色在库但无边 → 空授权,且不得回退内置(防"清空权限被复活")。
func TestPermResolver_ZeroEdgeRoleStaysEmpty(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		"viewer": nil, // 管理员剥光 viewer 的全部权限
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	if got := permSet(r, "viewer"); len(got) != 0 {
		t.Fatalf("零权限边角色应保持空授权(不回退内置): %v", got)
	}
}

// bootstrap:首刷前缓存为空,全部角色走内置兜底(种子未跑时不锁死)。
func TestPermResolver_EmptyCacheFallsBackToBuiltin(t *testing.T) {
	r := NewRolePermissionResolver(&fakePermLoader{}, nil) // 未 Refresh
	if got := permSet(r, "oncall"); !hasCode(got, "ceoboard.ops") {
		t.Fatalf("空缓存期应走内置兜底: %v", got)
	}
}

// 失败保旧:刷新失败保留上一份缓存并返回错误;恢复后下一刷生效。
func TestPermResolver_RefreshFailureKeepsLastGood(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		"custom": {"a.b"},
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	loader.mu.Lock()
	loader.err = errors.New("db down")
	loader.mu.Unlock()
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("加载失败应返回错误")
	}
	if got := permSet(r, "custom"); !hasCode(got, "a.b") {
		t.Fatalf("刷新失败须保留 last-good 缓存: %v", got)
	}

	loader.mu.Lock()
	loader.err = nil
	loader.snap = map[string][]string{"custom": {"c.d"}}
	loader.mu.Unlock()
	refreshOrDie(t, r)
	got := permSet(r, "custom")
	if !hasCode(got, "c.d") || hasCode(got, "a.b") {
		t.Fatalf("恢复后应整体换缓存: %v", got)
	}
}

// 多角色取并集;同一码去重。
func TestPermResolver_UnionAcrossRoles(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		"r1": {"knowledge.read", "audit.read"},
		"r2": {"audit.read", "usage.read"},
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	got := permSet(r, "r1", "r2")
	if len(got) != 3 {
		t.Fatalf("并集去重失败: %v", got)
	}
	for _, code := range []string{"knowledge.read", "audit.read", "usage.read"} {
		if !hasCode(got, code) {
			t.Fatalf("缺少 %s: %v", code, got)
		}
	}
}

// 缓存与快照隔离:调用方改加载器快照不影响已换入的缓存。
func TestPermResolver_CacheIsolatedFromLoaderSnapshot(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{
		"r1": {"a.b"},
	}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	loader.setSnap(map[string][]string{"r1": {"x.y"}}) // 不 Refresh
	if got := permSet(r, "r1"); !hasCode(got, "a.b") || hasCode(got, "x.y") {
		t.Fatalf("未刷新的快照变化不得影响缓存: %v", got)
	}
}

// 并发烟测(本机无 -race):读写并发不死锁、不 panic。
func TestPermResolver_ConcurrentReadRefresh(t *testing.T) {
	loader := &fakePermLoader{snap: map[string][]string{"r1": {"a.b"}}}
	r := NewRolePermissionResolver(loader, nil)
	refreshOrDie(t, r)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = permSet(r, "r1", "viewer")
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			_ = r.Refresh(context.Background())
		}
	}()
	wg.Wait()
}

package main

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

// hangingPermLoader 模拟 DB 挂起：阻塞至 ctx 取消并返回其错误。
type hangingPermLoader struct{}

func (hangingPermLoader) LoadRolePermissions(ctx context.Context) (map[string][]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type staticPermLoader struct{ snap map[string][]string }

func (s staticPermLoader) LoadRolePermissions(context.Context) (map[string][]string, error) {
	return s.snap, nil
}

// 评审 2026-11-09 D-LOW：permResolver 启动首刷必须带超时——DB 挂起时在预算内
// 降级返回（仅 Warn，不阻断启动），超时后空缓存 = 纯内置兜底（fail-usable），
// 与周期刷新失败路径语义一致。
func TestInitialPermRefreshTimesOutAndDegrades(t *testing.T) {
	r := data.NewRolePermissionResolver(hangingPermLoader{}, nil)
	start := time.Now()
	initialPermRefresh(r, loggateway.NewNoop(), 50*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("initialPermRefresh 未在超时预算内返回: %s", elapsed)
	}
	// 超时降级后内置角色兜底可用（superadmin=*），不得全员锁死。
	if got := r.PermissionsForRoles([]string{"superadmin"}); !hasCode(got, "*") {
		t.Fatalf("超时降级后应走内置兜底: %v", got)
	}
}

func TestInitialPermRefreshSuccessPopulatesCache(t *testing.T) {
	r := data.NewRolePermissionResolver(staticPermLoader{snap: map[string][]string{
		"viewer": {"ceoboard.view"},
	}}, nil)
	initialPermRefresh(r, loggateway.NewNoop(), time.Second)
	if got := r.PermissionsForRoles([]string{"viewer"}); !hasCode(got, "ceoboard.view") {
		t.Fatalf("首刷成功后 DB 授权应生效: %v", got)
	}
}

func hasCode(set map[string]struct{}, code string) bool {
	_, ok := set[code]
	return ok
}

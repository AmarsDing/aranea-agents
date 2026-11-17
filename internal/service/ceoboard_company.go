// CEO 经营驾驶舱业务快照接口。
//
// 设计说明：
//   - 该路由为只读 JSON BFF，不修改既有 ceoboard.proto 契约，避免影响 12 个
//     已登记 RPC 与权限注册表；后续若要契约化再提升为 proto rpc。
//   - 鉴权与 GetEventByUIDHTTP 保持一致：有主体必须持 ceoboard.view；无主体
//     仅 dev/test 放行（生产无凭据请求已在认证过滤器被拒）。
//   - 聚合层带 10s 缓存，多端 15s 轮询不会放大数据库压力。
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"aranea-agents/internal/biz/ceoboard/bizview"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"
)

const companySnapshotTimeout = 6 * time.Second

// bizSnapshotBuilder 惰性构造聚合器，保证进程内只有一个缓存实例。
func (s *CeoboardService) bizSnapshotBuilder() *bizview.Builder {
	s.bizOnce.Do(func() {
		s.bizView = bizview.NewBuilder(s.rt.Store.Client())
	})
	return s.bizView
}

// CompanyViewHTTP 是 GET /v1/ceoboard/company 的处理函数。
func (s *CeoboardService) CompanyViewHTTP(w http.ResponseWriter, r *http.Request) {
	if a, ok := auth.FromContext(r.Context()); ok && a != nil {
		if !a.HasPermission("ceoboard.view") {
			writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
			return
		}
	} else if !auth.DevOrTestEnv() {
		writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), companySnapshotTimeout)
	defer cancel()

	snap, err := s.bizSnapshotBuilder().Build(ctx)
	if err != nil {
		s.lg.Error("ceoboard company snapshot failed", loggateway.Err(err), loggateway.Domain(ceoboardDomain))
		writeCeoboardHTTPError(w, http.StatusServiceUnavailable, "business data temporarily unavailable")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snap)
}

// Package memoryhttp 提供记忆 L0~L4 的 HTTP 适配器，由 transport 注入与 Catalog evolution 同型的响应辅助函数。
package memoryhttp

import (
	"net/http"

	"arenea/backend/internal/service"
)

// MemoryHTTP 持有所需 Memory 服务访问器与审计 / JSON 编解码，供 Register 与会话内分发方法使用。
type MemoryHTTP struct {
	memoryL0         func() *service.MemoryL0Service
	memoryL1         func() *service.MemoryL1Service
	memoryL2         func() *service.MemoryL2Service
	memoryL3         func() *service.MemoryL3Service
	memoryL4         func() *service.MemoryL4Service
	audit            *service.AuditService
	writeJSON        func(http.ResponseWriter, int, any)
	writeErr         func(http.ResponseWriter, int, error)
	decodeBody       func(http.ResponseWriter, *http.Request, any) bool
	methodNotAllowed func(http.ResponseWriter)
	parsePositiveInt func(string, int) int
}

// NewMemoryHTTP 与 transport 层 wiring；各 l* 未注入时对应链路上返回 503。
func NewMemoryHTTP(
	l0 func() *service.MemoryL0Service,
	l1 func() *service.MemoryL1Service,
	l2 func() *service.MemoryL2Service,
	l3 func() *service.MemoryL3Service,
	l4 func() *service.MemoryL4Service,
	audit *service.AuditService,
	writeJSON func(http.ResponseWriter, int, any),
	writeErr func(http.ResponseWriter, int, error),
	decodeBody func(http.ResponseWriter, *http.Request, any) bool,
	methodNotAllowed func(http.ResponseWriter),
	parsePositiveInt func(string, int) int,
) *MemoryHTTP {
	return &MemoryHTTP{
		memoryL0: l0, memoryL1: l1, memoryL2: l2, memoryL3: l3, memoryL4: l4,
		audit:            audit,
		writeJSON:        writeJSON,
		writeErr:         writeErr,
		decodeBody:       decodeBody,
		methodNotAllowed: methodNotAllowed,
		parsePositiveInt: parsePositiveInt,
	}
}

// Register 挂载 /api/v1/l0/...、/api/v1/memory/l{1,2,3,4}/... 等路由。会话下 L0/L1/L2 由 HandleL* 从 sessions 入口调用。
func (m *MemoryHTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/l0/preview", m.HandleL0Preview)
	mux.HandleFunc("/api/v1/l0/snapshots/", m.HandleL0SnapshotByID)
	m.registerMemoryL1Routes(mux)
	m.registerMemoryL2AdminRoutes(mux)
	m.registerMemoryL3Routes(mux)
	m.registerMemoryL4Routes(mux)
}

func (m *MemoryHTTP) l0() *service.MemoryL0Service {
	if m.memoryL0 == nil {
		return nil
	}
	return m.memoryL0()
}

func (m *MemoryHTTP) l1() *service.MemoryL1Service {
	if m.memoryL1 == nil {
		return nil
	}
	return m.memoryL1()
}

func (m *MemoryHTTP) l2() *service.MemoryL2Service {
	if m.memoryL2 == nil {
		return nil
	}
	return m.memoryL2()
}

func (m *MemoryHTTP) l3() *service.MemoryL3Service {
	if m.memoryL3 == nil {
		return nil
	}
	return m.memoryL3()
}

func (m *MemoryHTTP) l4() *service.MemoryL4Service {
	if m.memoryL4 == nil {
		return nil
	}
	return m.memoryL4()
}

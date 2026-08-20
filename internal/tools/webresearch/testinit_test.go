package webresearch_test

import (
	"os"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	// 本包测试全部打本地 httptest mock server；经 outboundguard 官方
	// escape hatch 放行 loopback，避免 SSRF dial 拦截误伤测试。
	os.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", "127.0.0.1,::1")
	os.Exit(m.Run())
}

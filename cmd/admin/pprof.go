package main

import (
	"net/http"
	_ "net/http/pprof" // 将 /debug/pprof/* 处理器注册到 http.DefaultServeMux
	"strings"

	"github.com/go-kratos/kratos/v2/log"
)

// startPprofServer 在独立端口启动 pprof 诊断服务（G1 观测缺口补齐）。
// addr 为空（环境变量未设置）时不启动——默认关闭；评测环境由 compose 注入
// ARANEA_PPROF_ADDR=":8813"。该端口不映射宿主机，仅 araneanet 内网可达，
// 经 `docker exec aranea-admin wget -O- http://127.0.0.1:8813/debug/pprof/...` 抓取。
func startPprofServer(addr string, helper *log.Helper) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}
	go func() {
		helper.Infof("pprof server listening on %s (container-internal only)", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			helper.Warnf("pprof server exited: %v", err)
		}
	}()
}

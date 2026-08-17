package main

import (
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// 空地址（含纯空白）必须不启动监听：默认关闭是安全红线。
func TestStartPprofServerDisabledOnEmptyAddr(t *testing.T) {
	helper := log.NewHelper(log.DefaultLogger)
	for _, addr := range []string{"", "   ", "\t\n"} {
		done := make(chan struct{})
		go func() {
			startPprofServer(addr, helper)
			close(done)
		}()
		<-done // 立即返回即视为未启动
	}
}

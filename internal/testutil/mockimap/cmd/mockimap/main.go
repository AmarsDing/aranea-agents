// mockimap 独立启动器（W1d 冒烟用；单测请直接进程内 mockimap.Listen）。
//
//	go run ./internal/testutil/mockimap/cmd/mockimap
//
// 环境变量：MOCK_IMAP_ADDR（默认 127.0.0.1:11430）、MOCK_IMAP_CTL_ADDR
// （HTTP 控制口，默认 127.0.0.1:11431；置空关闭）。账号任意非空即可登录。
package main

import (
	"fmt"
	"os"

	"aranea-agents/internal/testutil/mockimap"
)

func main() {
	imapAddr := os.Getenv("MOCK_IMAP_ADDR")
	if imapAddr == "" {
		imapAddr = "127.0.0.1:11430"
	}
	ctlAddr := os.Getenv("MOCK_IMAP_CTL_ADDR")
	if ctlAddr == "" {
		ctlAddr = "127.0.0.1:11431"
	}
	srv, err := mockimap.Listen(imapAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mockimap listen:", err)
		os.Exit(1)
	}
	fmt.Printf("mock IMAP listening: %s（账号任意非空，如 test@local / test）\n", srv.Addr())
	if ctlAddr != "" {
		ctl, err := srv.EnableControl(ctlAddr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mockimap control listen:", err)
			os.Exit(1)
		}
		fmt.Printf("mock IMAP control: http://%s（POST /inject 注入新邮件触发 IDLE；GET /count）\n", ctl)
	}
	select {}
}

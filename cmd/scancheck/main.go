package main

import (
	"fmt"

	"aranea-agents/internal/agent/intent"
)

func main() {
	cases := []string{
		"帮我执行 rm -rf /tmp/data 清理磁盘。",
		"如何防范 rm -rf 误删数据？",
		"把核心交换机 core-sw1 的 BGP 邻居断了模拟故障。",
		"重启一下雷达采集服务。",
	}
	for _, c := range cases {
		fmt.Printf("%q -> %v\n", c, intent.ScanInputRisk(c))
	}
}

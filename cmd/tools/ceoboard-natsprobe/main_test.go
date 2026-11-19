// 评审 2026-11-09 D6:任一目标探测失败 → 非零退出;摘要输出形态不变。
package main

import (
	"bytes"
	"strings"
	"testing"
)

// 不可达地址(本机 1 号端口无监听,连接即拒,测试不依赖外网与超时等待)。
const unreachable = "nats://127.0.0.1:1"

func TestRunAllTargetsFailReturnsNonZero(t *testing.T) {
	var out bytes.Buffer
	code := run([][2]string{{"BAD1", unreachable}, {"BAD2", unreachable}}, &out)
	if code == 0 {
		t.Fatal("全部目标探测失败时必须非零退出")
	}
	if !strings.Contains(out.String(), "连接失败") {
		t.Fatalf("失败明细必须照常打印(摘要模式不变):\n%s", out.String())
	}
	if !strings.Contains(out.String(), "BAD1") || !strings.Contains(out.String(), "BAD2") {
		t.Fatalf("每个失败目标都应留痕:\n%s", out.String())
	}
}

func TestProbeUnreachableReturnsError(t *testing.T) {
	var out bytes.Buffer
	if err := probe("BAD", unreachable, &out); err == nil {
		t.Fatal("不可达目标 probe 必须返回错误")
	}
}

func TestRunEmptyTargetListSucceeds(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 0 {
		t.Fatalf("无目标视为无事可做,应 0: %d", code)
	}
}

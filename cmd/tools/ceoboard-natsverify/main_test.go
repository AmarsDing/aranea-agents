// 评审 2026-11-09 D5/D6/D8 测试:
//   - D8 死信路径自检:与生产 natsjs.go persistEventResult 相同调用序列在
//     storetest(sqlite)上全绿;
//   - D6 退出码:NATS 不可达的启用段 → run 返回 1,双侧跳过 → 0;
//   - D5 注入形态在 main 中(flag/环境),此处不重复测装配。
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/biz/ceoboard/store/storetest"
)

// 本机 1 号端口无监听,连接即拒,测试不依赖外网与超时等待。
const unreachable = "nats://127.0.0.1:1"

func openVerifyStore(t *testing.T) *store.Store {
	t.Helper()
	client, _ := storetest.Open(t)
	return store.New(client, nil)
}

// D8:malformed 消息 → nats-decode 死信写入/读回/pending 计数/标重放全链通过,
// 且重复运行以全新 uid 互不冲突。
func TestVerifyDeadLetterPath(t *testing.T) {
	st := openVerifyStore(t)
	ctx := context.Background()
	if err := verifyDeadLetterPath(ctx, st, "hcf"); err != nil {
		t.Fatalf("死信路径自检必须全绿: %v", err)
	}
	if err := verifyDeadLetterPath(ctx, st, "novatrader"); err != nil {
		t.Fatalf("第二平台死信路径自检必须全绿: %v", err)
	}
}

// D6:NATS 不可达 → 启用段全部候选失败 → run 返回 1,且段输出含失败明细。
func TestRunUnreachableNATSSectionFails(t *testing.T) {
	st := openVerifyStore(t)
	var out bytes.Buffer
	code := run(context.Background(), st, unreachable, "", 1, &out)
	if code != 1 {
		t.Fatalf("NT 段全灭应 exit 1,实际 %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "所有候选 subject 均失败") {
		t.Fatalf("段失败明细必须打印:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "HCF:跳过") {
		t.Fatalf("未启用的 HCF 段应明示跳过:\n%s", out.String())
	}
}

// D6 反向:双侧皆跳过 → 无事失败 → 0。
func TestRunBothSectionsSkippedSucceeds(t *testing.T) {
	st := openVerifyStore(t)
	var out bytes.Buffer
	if code := run(context.Background(), st, "", "", 1, &out); code != 0 {
		t.Fatalf("双侧跳过应 exit 0,实际 %d\n%s", code, out.String())
	}
}

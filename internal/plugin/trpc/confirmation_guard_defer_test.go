package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TestConfirmationGuardPlugin_DefersToProductGate 钉死 79-runtime-governance
// 三轮审查 T3 根修：vendored 框架执行序为 runner 插件先于 agent 链回调
// （上游官方文档明载的设计语义），ToolConfirmHandled 标记在插件执行时必不
// 存在——confirmation_guard 启用时会整体遮蔽链上产品门禁（param gate deny
// 失审计 / ask 无确认卡 / allow 失效）。根治：进程级「产品门禁已装配」标记
// 置位时插件完全让位；未置位（CLI 无 DB 形态）保留遗产硬拦（fail-closed）。
// 让位不损失拦截语义：产品门禁 decide() 插件分支用同一
// MatchConfirmationGuard + 同一 DB 配置行升级为交互确认。
//
// 进程级标记，本包测试不得 t.Parallel()。
func TestConfirmationGuardPlugin_DefersToProductGate(t *testing.T) {
	newPlugin := func() *ConfirmationGuardPlugin {
		return &ConfirmationGuardPlugin{
			base: newBasePlugin("confirmation_guard", nil, nil, loggateway.NewNoop()),
			cfg:  ConfirmationGuardConfig{ConfirmTools: []string{"exec_command"}, DefaultAction: "reject"},
		}
	}
	matchingArgs := &trpctool.BeforeToolArgs{ToolName: "exec_command", Arguments: []byte(`{"command":"ls"}`)}

	t.Run("装配标记置位→让位（命中工具也不硬拦）", func(t *testing.T) {
		SetProductConfirmGateActive(true)
		t.Cleanup(func() { SetProductConfirmGateActive(false) })
		res, err := newPlugin().beforeTool(context.Background(), matchingArgs)
		if err != nil {
			t.Fatalf("beforeTool err = %v", err)
		}
		if res == nil || res.CustomResult != nil {
			t.Fatalf("产品门禁装配后插件必须让位（no CustomResult），got %+v", res)
		}
	})

	t.Run("未置位→遗产硬拦（CLI 形态 fail-closed）", func(t *testing.T) {
		SetProductConfirmGateActive(false)
		res, err := newPlugin().beforeTool(context.Background(), matchingArgs)
		if err != nil {
			t.Fatalf("beforeTool err = %v", err)
		}
		if res == nil || res.CustomResult == nil {
			t.Fatal("CLI 形态（产品门禁未装配）插件必须保留硬拦")
		}
	})

	t.Run("未置位+未命中工具→放行", func(t *testing.T) {
		SetProductConfirmGateActive(false)
		res, err := newPlugin().beforeTool(context.Background(), &trpctool.BeforeToolArgs{
			ToolName: "read_file", Arguments: []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("beforeTool err = %v", err)
		}
		if res == nil || res.CustomResult != nil {
			t.Fatalf("未命中 ConfirmTools 不得拦截，got %+v", res)
		}
	})

	t.Run("handled 标记优先于装配标记（链先序防御）", func(t *testing.T) {
		SetProductConfirmGateActive(false) // 即使未置位，handled 也让位
		ctx := WithToolConfirmHandled(context.Background())
		res, err := newPlugin().beforeTool(ctx, matchingArgs)
		if err != nil {
			t.Fatalf("beforeTool err = %v", err)
		}
		if res == nil || res.CustomResult != nil {
			t.Fatalf("ToolConfirmHandled 已标记时插件必须 no-op，got %+v", res)
		}
	})
}

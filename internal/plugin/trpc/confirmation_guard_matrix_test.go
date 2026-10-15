package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TestConfirmationGuard_ModeMatrix tables server vs CLI × catalog hit × handled.
// Complements TestConfirmationGuardPlugin_DefersToProductGate (W1-6).
// Process-global flag — do not t.Parallel().
func TestConfirmationGuard_ModeMatrix(t *testing.T) {
	prev := ProductConfirmGateActive()
	t.Cleanup(func() { SetProductConfirmGateActive(prev) })

	newPlugin := func() *ConfirmationGuardPlugin {
		return &ConfirmationGuardPlugin{
			base: newBasePlugin("confirmation_guard", nil, nil, loggateway.NewNoop()),
			cfg:  ConfirmationGuardConfig{ConfirmTools: []string{"exec_command"}, DefaultAction: "reject"},
		}
	}
	hit := &trpctool.BeforeToolArgs{ToolName: "exec_command", Arguments: []byte(`{"command":"ls"}`)}
	miss := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: []byte(`{}`)}

	type want struct {
		hardBlock bool
	}
	cases := []struct {
		name    string
		server  bool
		handled bool
		args    *trpctool.BeforeToolArgs
		want    want
	}{
		{name: "server_hit_defers", server: true, handled: false, args: hit, want: want{hardBlock: false}},
		{name: "server_miss_defers", server: true, handled: false, args: miss, want: want{hardBlock: false}},
		{name: "cli_hit_hard_block", server: false, handled: false, args: hit, want: want{hardBlock: true}},
		{name: "cli_miss_pass", server: false, handled: false, args: miss, want: want{hardBlock: false}},
		{name: "cli_hit_handled_defers", server: false, handled: true, args: hit, want: want{hardBlock: false}},
		{name: "server_hit_handled_defers", server: true, handled: true, args: hit, want: want{hardBlock: false}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetProductConfirmGateActive(tc.server)
			ctx := context.Background()
			if tc.handled {
				ctx = WithToolConfirmHandled(ctx)
			}
			res, err := newPlugin().beforeTool(ctx, tc.args)
			if err != nil {
				t.Fatalf("beforeTool err = %v", err)
			}
			blocked := res != nil && res.CustomResult != nil
			if blocked != tc.want.hardBlock {
				t.Fatalf("hardBlock=%v want %v (res=%+v)", blocked, tc.want.hardBlock, res)
			}
		})
	}
}

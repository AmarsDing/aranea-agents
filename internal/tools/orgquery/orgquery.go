// Package orgquery provides read-only organization inspect tools for
// company_lead and dept_lead (P0 org visibility). These are governance
// query tools — they do not form teams or inherit Spirit reserved keys.
package orgquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Inspector is the biz-layer port backing org inspect.
type Inspector interface {
	InspectGovernance(ctx context.Context, caller biz.Agent, focusKey string) (biz.OrgInspectView, error)
}

// RegisterAll creates the org inspect tools bound to the caller's identity.
func RegisterAll(insp Inspector, caller biz.Agent, lg loggateway.Logger) []trpctool.Tool {
	if insp == nil || !biz.IsOrgGovernanceAgent(caller) {
		return nil
	}
	return []trpctool.Tool{
		&orgInspectTool{insp: insp, caller: caller, lg: lg},
	}
}

type orgInspectTool struct {
	insp   Inspector
	caller biz.Agent
	lg     loggateway.Logger
}

type orgInspectInput struct {
	FocusKey string `json:"focus_key"`
}

func (t *orgInspectTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "org_inspect",
		Description: "只读查看编制表：公司/部门/岗位名称与层级。" +
			"用于回答「有哪些部门、某部门挂了什么岗」。不能组队、不能派发任务。" +
			"跨部门执行请建议用户到精灵会话走 plan_and_execute，或指出对应部门主管会话。",
		InputSchema: &trpctool.Schema{
			Type: "object",
			Properties: map[string]*trpctool.Schema{
				"focus_key": {Type: "string", Description: "可选。部门或岗位 key，缩小到该子树。空则返回调用方可见范围。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"scope_key", "entries"},
			Properties: map[string]*trpctool.Schema{
				"scope_key":  {Type: "string", Description: "当前快照根节点 key。"},
				"scope_name": {Type: "string", Description: "当前快照根节点名称。"},
				"entries":    {Type: "array", Description: "节点列表（level/key/name/parent_key/lead_hint）。"},
			},
		},
	}
}

func (t *orgInspectTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in orgInspectInput
	if len(jsonArgs) > 0 {
		if err := json.Unmarshal(jsonArgs, &in); err != nil {
			return nil, fmt.Errorf("参数解析失败: %w", err)
		}
	}
	view, err := t.insp.InspectGovernance(ctx, t.caller, strings.TrimSpace(in.FocusKey))
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("org_inspect 失败",
				loggateway.StepID("tool.org_inspect"),
				loggateway.Str("caller", t.caller.AgentKey),
				loggateway.Err(err))
		}
		return nil, err
	}
	return view, nil
}

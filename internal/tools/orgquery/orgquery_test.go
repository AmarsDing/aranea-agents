package orgquery

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type fakeInspector struct {
	view biz.OrgInspectView
	err  error
	got  biz.Agent
	key  string
}

func (f *fakeInspector) InspectGovernance(_ context.Context, caller biz.Agent, focusKey string) (biz.OrgInspectView, error) {
	f.got = caller
	f.key = focusKey
	return f.view, f.err
}

func TestRegisterAll_OnlyGovernance(t *testing.T) {
	insp := &fakeInspector{}
	if got := RegisterAll(insp, biz.Agent{AgentKey: "worker"}, loggateway.NewNoop()); got != nil {
		t.Fatalf("specialist must not get org_inspect, got %d tools", len(got))
	}
	lead := biz.Agent{AgentKey: "__dept_lead_mkt__", AgentVariant: "dept_lead"}
	tools := RegisterAll(insp, lead, loggateway.NewNoop())
	if len(tools) != 1 {
		t.Fatalf("dept_lead tools = %d, want 1", len(tools))
	}
	if tools[0].Declaration().Name != "org_inspect" {
		t.Fatalf("name = %q", tools[0].Declaration().Name)
	}
}

func TestOrgInspectCall(t *testing.T) {
	insp := &fakeInspector{view: biz.OrgInspectView{
		ScopeKey: "mkt", ScopeName: "市场部",
		Entries: []biz.OrgInspectEntry{{Key: "mkt", Name: "市场部", Level: "department"}},
	}}
	lead := biz.Agent{ID: "lead-1", AgentKey: "__dept_lead_mkt__", AgentVariant: "dept_lead"}
	tool := RegisterAll(insp, lead, loggateway.NewNoop())[0]
	raw, err := json.Marshal(map[string]string{"focus_key": "copy"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.(interface {
		Call(context.Context, []byte) (any, error)
	}).Call(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if insp.key != "copy" || insp.got.ID != "lead-1" {
		t.Fatalf("inspector got key=%q caller=%s", insp.key, insp.got.ID)
	}
	view, ok := out.(biz.OrgInspectView)
	if !ok || view.ScopeKey != "mkt" {
		t.Fatalf("output = %#v", out)
	}
}

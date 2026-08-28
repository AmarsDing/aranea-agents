package biz

import (
	"context"
	"encoding/json"
	"testing"

	"fmt"

	"aranea-agents/pkg/apierror"
)

func TestIsCompanyLeadAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{"variant", Agent{AgentVariant: AgentVariantCompanyLead}, true},
		{"key", Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme__"}, true},
		{"prefix only", Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme"}, false},
		{"dept lead is not company", Agent{AgentVariant: "dept_lead"}, false},
		{"worker", Agent{AgentKey: "be"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsCompanyLeadAgent(c.agent); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestCompanyLeadNotAssignable(t *testing.T) {
	t.Parallel()
	a := Agent{
		AgentKey:     CompanyLeadAgentKeyPrefix + "acme__",
		AgentVariant: AgentVariantCompanyLead,
		Status:       "active",
	}
	if IsCatalogAgentAssignable(a) {
		t.Fatal("company_lead must not be catalog-assignable")
	}
	cap := AgentCapability{AgentKey: a.AgentKey, AgentVariant: a.AgentVariant}
	if cap.IsHeuristicAssignable() {
		t.Fatal("company_lead must not be heuristically assignable")
	}
}

func TestCompanyLeadOrgKeys(t *testing.T) {
	t.Parallel()
	if CompanyOfficeDeptKey("acme") != "acme_office" || CompanyLeadPositionKey("acme") != "acme_gm" {
		t.Fatal("keys")
	}
}

func TestEnsureCompanyLeadPositionCreatesOfficeAndGM(t *testing.T) {
	t.Parallel()
	repo := newFakeCompanyLeadSlotRepo()
	company := OrganizationNode{ID: "co-1", Key: "acme", Name: "Acme", Level: "company"}
	pos, err := ensureCompanyLeadPositionOn(context.Background(), repo, company)
	if err != nil {
		t.Fatal(err)
	}
	if pos.Level != "position" || pos.Name != CompanyLeadPositionName || pos.Key != "acme_gm" {
		t.Fatalf("pos=%+v", pos)
	}
	office, err := repo.GetOrgNodeByKey(context.Background(), "acme_office")
	if err != nil || office.Level != "department" || office.ParentID != "co-1" {
		t.Fatalf("office=%+v err=%v", office, err)
	}
	if pos.ParentID != office.ID {
		t.Fatalf("position parent %s want %s", pos.ParentID, office.ID)
	}
}

func TestFilterRecipeAgentKeysDropsLeads(t *testing.T) {
	t.Parallel()
	got := FilterRecipeAgentKeys([]string{"be", CompanyLeadAgentKeyPrefix + "acme__", DeptLeadAgentKeyPrefix + "eng__"})
	if len(got) != 1 || got[0] != "be" {
		t.Fatalf("got %v", got)
	}
}

func TestCompanyLeadMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	n := OrganizationNode{MetadataJSON: `{"playbooks":[]}`, CompanyLeadAgentID: "lead-1"}
	ApplyCompanyLeadToMetadata(&n)
	n.CompanyLeadAgentID = ""
	HydrateCompanyLeadFromMetadata(&n)
	if n.CompanyLeadAgentID != "lead-1" {
		t.Fatalf("id=%q", n.CompanyLeadAgentID)
	}
}

// Q10（session-eval-20260827，C+A 组合）：GM 种子 allow 字面量的不变式——
// 恰好命名 subagents 四件、JSON 合法、且四件全部在 registryOptInOnlyKeys
// （种子 enabled=false 时 allow 授予唯一生效通道；出表即被 applyRegistryAdminDenials
// 全员硬 deny，授权静默无效——2026-08-24 twin_config_* 漏登记事故的同款防线）。
// 存量库由 docker/q10-gm-subagent-allow.sql 同步，字面量必须与脚本一致。
func TestCompanyLeadSubagentAllowJSON(t *testing.T) {
	t.Parallel()
	var keys []string
	if err := json.Unmarshal([]byte(companyLeadSubagentAllowJSON), &keys); err != nil {
		t.Fatalf("allow JSON malformed: %v", err)
	}
	want := []string{"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel"}
	if len(keys) != len(want) {
		t.Fatalf("allow = %v, want exactly %v", keys, want)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("allow[%d] = %s, want %s（与 docker/q10-gm-subagent-allow.sql 保持一致）", i, keys[i], k)
		}
		if !registryOptInOnlyKeys[k] {
			t.Fatalf("%s 不在 registryOptInOnlyKeys——allow 授予将被硬 deny 静默无效", k)
		}
	}
	// R17 边界：GM allow 不得含 Spirit 保留件。
	for _, k := range keys {
		for _, reserved := range SpiritReservedToolKeys() {
			if k == reserved {
				t.Fatalf("GM allow 不得命名 Spirit 保留件 %s", k)
			}
		}
	}
}

type fakeCompanyLeadSlotRepo struct {
	byKey map[string]OrganizationNode
	n     int
}

func newFakeCompanyLeadSlotRepo() *fakeCompanyLeadSlotRepo {
	return &fakeCompanyLeadSlotRepo{byKey: map[string]OrganizationNode{}}
}

func (f *fakeCompanyLeadSlotRepo) GetOrgNodeByKey(_ context.Context, key string) (OrganizationNode, error) {
	n, ok := f.byKey[key]
	if !ok {
		return OrganizationNode{}, apierror.NotFound(apierror.DomainOrg, "not found")
	}
	return n, nil
}

func (f *fakeCompanyLeadSlotRepo) CreateOrgNode(_ context.Context, c OrganizationNode) (OrganizationNode, error) {
	f.n++
	if c.ID == "" {
		c.ID = fmt.Sprintf("n%d", f.n)
	}
	f.byKey[c.Key] = c
	return c, nil
}

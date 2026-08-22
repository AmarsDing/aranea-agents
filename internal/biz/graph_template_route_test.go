package biz

import (
	"strings"
	"testing"
)

func TestResolveGraphTemplateRouteBuiltinRemapsMode(t *testing.T) {
	t.Parallel()
	r := ResolveGraphTemplateRoute("sequential", "parallel_review")
	if !r.Builtin || r.Mode != "parallel" || r.CompileTemplate != "parallel_review" || r.LinkedGraphID != "" {
		t.Fatalf("%+v", r)
	}
	if !IsBuiltinGraphTemplateID("pipeline") || IsBuiltinGraphTemplateID("g-asset-1") {
		t.Fatal("builtin vs asset")
	}
}

func TestResolveGraphTemplateRoutePersistedAsset(t *testing.T) {
	t.Parallel()
	r := ResolveGraphTemplateRoute("sequential", "tmpl-uuid-1")
	if r.Builtin || r.LinkedGraphID != "tmpl-uuid-1" {
		t.Fatalf("%+v", r)
	}
}

func TestResolveGraphTemplateRouteEmptyStaysOrdinaryTurn(t *testing.T) {
	t.Parallel()
	r := ResolveGraphTemplateRoute("coordinator", "")
	if r.Builtin || r.LinkedGraphID != "" || r.Mode != "coordinator" {
		t.Fatalf("%+v", r)
	}
}

func TestApplyAssembleOrgFacesStampsTemplateAndDeniesSpiritTools(t *testing.T) {
	t.Parallel()
	got := ApplyAssembleOrgFaces(`{"version":1,"mode":"sequential"}`, []string{"be", SpiritAgentKey}, "tmpl-1", []string{"kb-1", "", "kb-1"})
	if !strings.Contains(got, `"collection_ids":["kb-1"]`) {
		t.Fatalf("collection ids missing: %s", got)
	}
	if !strings.Contains(got, `"graph_template_id":"tmpl-1"`) {
		t.Fatalf("template missing: %s", got)
	}
	if !strings.Contains(got, `"plan_and_execute"`) {
		t.Fatalf("spirit deny missing: %s", got)
	}
	faces := SpecialistToolFaces([]string{"be", SpiritAgentKey})
	if _, ok := faces[SpiritAgentKey]; ok {
		t.Fatal("spirit must not get a deny face")
	}
	if _, ok := faces["be"]; !ok {
		t.Fatal("specialist must get a deny face")
	}
}

func TestClampSpecialistToolFaceDowngradesSpiritProfile(t *testing.T) {
	t.Parallel()
	s := AgentRuntimeSettings{ToolsProfile: "spirit"}
	ClampSpecialistToolFace(&s, Agent{AgentKey: "be"})
	if s.ToolsProfile != "coding" {
		t.Fatalf("worker profile=%q", s.ToolsProfile)
	}
	lead := AgentRuntimeSettings{ToolsProfile: "spirit"}
	ClampSpecialistToolFace(&lead, Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme__", AgentVariant: AgentVariantCompanyLead})
	if lead.ToolsProfile != "read_only" {
		t.Fatalf("lead profile=%q", lead.ToolsProfile)
	}
	spirit := AgentRuntimeSettings{ToolsProfile: "spirit"}
	ClampSpecialistToolFace(&spirit, Agent{AgentKey: SpiritAgentKey})
	if spirit.ToolsProfile != "spirit" {
		t.Fatalf("spirit profile=%q", spirit.ToolsProfile)
	}
}

func TestHighRiskVerificationGateIsConfirmTier(t *testing.T) {
	t.Parallel()
	if !HighRiskVerificationGate(VerificationGate{GateType: GateTypeBorrowApproval}) {
		t.Fatal("borrow is high risk")
	}
	if HighRiskVerificationGate(VerificationGate{GateType: GateTypeToolAssertion}) {
		t.Fatal("tool assertion is not the user-confirm tier")
	}
	if NeedsUserConfirm(ConfirmInput{HighRiskGate: HighRiskVerificationGate(VerificationGate{GateType: GateTypeDeptLeadApproval})}) != ConfirmHighRiskGate {
		t.Fatal("high risk gate must map to R18")
	}
}

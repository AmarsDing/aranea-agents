package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubOrgReader struct {
	nodes   map[string]biz.OrganizationNode
	creates int
}

func (s *stubOrgReader) GetOrgNode(_ context.Context, id string) (biz.OrganizationNode, error) {
	n, ok := s.nodes[id]
	if !ok {
		return biz.OrganizationNode{}, biz.ErrOrgBadRequest("not found")
	}
	return n, nil
}

func (s *stubOrgReader) GetOrgNodeByKey(_ context.Context, key string) (biz.OrganizationNode, error) {
	for _, n := range s.nodes {
		if n.Key == key {
			return n, nil
		}
	}
	return biz.OrganizationNode{}, biz.ErrOrgBadRequest("not found")
}

func (s *stubOrgReader) ListOrgNodes(_ context.Context) ([]biz.OrganizationNode, error) {
	out := make([]biz.OrganizationNode, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (s *stubOrgReader) ListOrgNodesByLevel(_ context.Context, level string) ([]biz.OrganizationNode, error) {
	var out []biz.OrganizationNode
	for _, n := range s.nodes {
		if n.Level == level {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *stubOrgReader) ListOrgNodesByParentID(_ context.Context, parentID string) ([]biz.OrganizationNode, error) {
	var out []biz.OrganizationNode
	for _, n := range s.nodes {
		if n.ParentID == parentID {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *stubOrgReader) ListOrgNodesByIDs(_ context.Context, ids []string) ([]biz.OrganizationNode, error) {
	out := make([]biz.OrganizationNode, 0, len(ids))
	for _, id := range ids {
		if n, ok := s.nodes[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *stubOrgReader) CreateOrgNode(_ context.Context, _ biz.OrganizationNode) (biz.OrganizationNode, error) {
	s.creates++
	return biz.OrganizationNode{}, biz.ErrOrgBadRequest("factory must not create org nodes")
}

func sampleOrgTree() map[string]biz.OrganizationNode {
	return map[string]biz.OrganizationNode{
		"co-1":          {ID: "co-1", Key: "acme", Name: "Acme", Level: "company"},
		"dept-eng":      {ID: "dept-eng", Key: "eng", Name: "研发部", Level: "department", ParentID: "co-1"},
		"dept-media":    {ID: "dept-media", Key: "media", Name: "内容运营部", Level: "department", ParentID: "co-1"},
		"pos-be":        {ID: "pos-be", Key: "backend", Name: "后端", Level: "position", ParentID: "dept-eng"},
		"pos-fe":        {ID: "pos-fe", Key: "frontend", Name: "前端", Level: "position", ParentID: "dept-eng"},
		"pos-copy":      {ID: "pos-copy", Key: "copywriter", Name: "文案", Level: "position", ParentID: "dept-media"},
		"pos-other-eng": {ID: "pos-other-eng", Key: "other", Name: "其他", Level: "position", ParentID: "dept-eng"},
	}
}

func TestBuildAll_FillsOrgPlacementAndKeepsUnpositioned(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "be", DisplayName: "后端", Status: "active", PositionID: "pos-be", Roles: []string{"backend"}},
		{AgentKey: "orphan", DisplayName: "无岗", Status: "active", Roles: []string{"general"}},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "eng__", DisplayName: "研发主管", Status: "active", AgentVariant: "dept_lead", PositionID: "dept-eng"},
		{AgentKey: biz.SpiritAgentKey, DisplayName: "Spirit", Status: "active"},
	}
	b := NewAgentCapabilityBuilder(&stubAgentReader{agents: agents}, loggateway.NewNoop())
	org := &stubOrgReader{nodes: sampleOrgTree()}
	b.SetOrganizationReader(org)

	caps, err := b.BuildAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]biz.AgentCapability{}
	for _, c := range caps {
		byKey[c.AgentKey] = c
	}
	if _, ok := byKey[biz.SpiritAgentKey]; ok {
		t.Fatal("system agent must not appear in capabilities")
	}
	be := byKey["be"]
	if be.DepartmentID != "dept-eng" || be.CompanyID != "co-1" || be.PositionKey != "backend" {
		t.Fatalf("backend placement: %+v", be)
	}
	if byKey["orphan"].DepartmentID != "" {
		t.Fatalf("unpositioned agent must not error; DepartmentID=%q", byKey["orphan"].DepartmentID)
	}
	lead := byKey[biz.DeptLeadAgentKeyPrefix+"eng__"]
	if lead.DepartmentID != "dept-eng" {
		t.Fatalf("dept_lead department: %q", lead.DepartmentID)
	}
	if lead.IsHeuristicAssignable() {
		t.Fatal("dept_lead must not be heuristically assignable")
	}
	if org.creates != 0 {
		t.Fatalf("CreateOrgNode calls=%d want 0", org.creates)
	}
}

func TestBuildAll_NilOrgReader_DoesNotFail(t *testing.T) {
	b := NewAgentCapabilityBuilder(&stubAgentReader{agents: []biz.Agent{
		{AgentKey: "be", Status: "active", PositionID: "pos-be"},
	}}, loggateway.NewNoop())
	caps, err := b.BuildAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 1 || caps[0].DepartmentID != "" {
		t.Fatalf("got %+v", caps)
	}
}

func TestOrgPruner_MatchFallbackAndEmptyDomain(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "be", DepartmentID: "dept-eng", DepartmentName: "研发部"},
		{AgentKey: "copy", DepartmentID: "dept-media", DepartmentName: "内容运营部"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "eng__", AgentVariant: "dept_lead", DepartmentID: "dept-eng", DepartmentName: "研发部"},
	}
	p := OrgPruner{}

	got := p.Prune("软件/后端", caps)
	if got.FallbackAll || got.Reason != OrgPruneReasonMatched {
		t.Fatalf("software prune: %+v", got)
	}
	if len(got.CandidateKeys) != 1 || got.CandidateKeys[0] != "be" {
		t.Fatalf("software candidates=%v (dept_lead must be excluded)", got.CandidateKeys)
	}

	empty := p.Prune("", caps)
	if !empty.FallbackAll || empty.Reason != OrgPruneReasonEmptyDomain {
		t.Fatalf("empty domain: %+v", empty)
	}

	noOrg := p.Prune("软件/后端", []biz.AgentCapability{{AgentKey: "x"}})
	if !noOrg.FallbackAll || noOrg.Reason != OrgPruneReasonNoOrg {
		t.Fatalf("no org: %+v", noOrg)
	}

	miss := p.Prune("研究/调研", caps)
	if !miss.FallbackAll || miss.Reason != OrgPruneReasonNoMatch {
		t.Fatalf("no match: %+v", miss)
	}
}

func TestPickComplementaryMembers_SameDeptThenCrossDept(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "lead", Roles: []string{"backend"}, DepartmentID: "d1"},
		{AgentKey: "same-dup", Roles: []string{"backend"}, DepartmentID: "d1"},
		{AgentKey: "same-fe", Roles: []string{"frontend"}, DepartmentID: "d1"},
		{AgentKey: "other", Roles: []string{"data"}, DepartmentID: "d2"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "d1__", AgentVariant: "dept_lead", DepartmentID: "d1", Roles: []string{"manager"}},
	}
	members, cross := pickComplementaryMembers("lead", caps, 1)
	if len(members) != 1 || members[0] != "same-fe" {
		t.Fatalf("members=%v want [same-fe]", members)
	}
	if len(cross) != 0 {
		t.Fatalf("cross=%v want empty", cross)
	}

	members, cross = pickComplementaryMembers("lead", []biz.AgentCapability{
		caps[0], caps[3],
	}, 1)
	if len(members) != 1 || members[0] != "other" {
		t.Fatalf("cross-dept members=%v", members)
	}
	if len(cross) != 1 || cross[0] != "other" {
		t.Fatalf("cross=%v", cross)
	}
}

func TestAllocate_ExcludesDeptLeadAndKeepsExplicit(t *testing.T) {
	leadKey := biz.DeptLeadAgentKeyPrefix + "media__"
	agents := []biz.Agent{
		{AgentKey: leadKey, DisplayName: "媒体主管", Status: "active", AgentVariant: "dept_lead", Roles: []string{"media"}, PositionID: "dept-media"},
		{AgentKey: "copy", DisplayName: "文案", Status: "active", Roles: []string{"media", "copy"}, PositionID: "pos-copy"},
	}
	reader := &stubAgentReader{agents: agents}
	org := &stubOrgReader{nodes: sampleOrgTree()}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	capBuilder.SetOrganizationReader(org)
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  capBuilder,
		lg:          loggateway.NewNoop(),
	}
	plan := &biz.TaskPlan{
		ID:       "tp1",
		TraceID:  "t1",
		Strategy: biz.StrategyDAG,
		SubTasks: []biz.SubTask{{
			ID: "st1", Name: "写文案", Description: "品牌文案",
			RequiredCapabilities: []string{"copy"},
			DomainPath:           "创作/文案",
		}},
	}
	saved, err := impl.Allocate(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Allocations[0].AssignedKey == leadKey {
		t.Fatal("dept_lead must not be AssignedKey")
	}
	if saved.Allocations[0].AssignedKey != "copy" {
		t.Fatalf("AssignedKey=%q want copy", saved.Allocations[0].AssignedKey)
	}
	for _, m := range saved.Allocations[0].TeamMemberKeys {
		if m == leadKey {
			t.Fatal("dept_lead must not be a complementary member")
		}
	}
	if org.creates != 0 {
		t.Fatalf("Allocate must not create org nodes, creates=%d", org.creates)
	}

	explicit, err := impl.AllocateExplicit(context.Background(), plan, []string{leadKey})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Allocations[0].AssignedKey != leadKey {
		t.Fatalf("explicit AssignedKey=%q", explicit.Allocations[0].AssignedKey)
	}
}

func TestDagAdditionalMemberCount(t *testing.T) {
	if got := dagAdditionalMemberCount("组建三人运维专家团队写说明"); got != 2 {
		t.Fatalf("三人团队 extra=%d want 2", got)
	}
	if got := dagAdditionalMemberCount("分派三个团队分别调研"); got != 1 {
		t.Fatalf("三个团队 extra=%d want 1", got)
	}
	if got := dagAdditionalMemberCount("dag 默认补一人"); got != 1 {
		t.Fatalf("default extra=%d want 1", got)
	}
}

func TestAllocate_DAGFallsBackToFullRosterForSecondMember(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "ops_auto_inspection", DisplayName: "自动巡检", Status: "active", DomainPath: "运维/巡检", PositionKey: "ops_auto_inspection"},
		{AgentKey: "ops_doc_generation", DisplayName: "文档生成", Status: "active", DomainPath: "办公/文档", PositionKey: "ops_doc_generation"},
	}}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	saved, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:          "tp-dag-fallback",
		Strategy:    biz.StrategyDAG,
		UserMessage: "组建三人运维专家团队协作完成一份机房巡检说明",
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "写巡检说明", DomainPath: "运维/巡检"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Allocations) != 1 {
		t.Fatalf("allocations=%d", len(saved.Allocations))
	}
	a := saved.Allocations[0]
	if a.AssignedKey != "ops_auto_inspection" {
		t.Fatalf("lead=%q", a.AssignedKey)
	}
	if len(a.TeamMemberKeys) == 0 {
		t.Fatal("dag must fall back to the full roster for a second member")
	}
	if a.TeamMemberKeys[0] != "ops_doc_generation" {
		t.Fatalf("members=%v", a.TeamMemberKeys)
	}
}

func TestAllocate_ResearchFallsBackToOfficeDocs(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "ops_doc_generation", DisplayName: "文档生成", Status: "active", DomainPath: "办公/文档", PositionKey: "ops_doc_generation"},
	}}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	saved, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:       "tp-research-fallback",
		Strategy: biz.StrategySingleAgent,
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "对比 HTTP/2 与 HTTP/3", DomainPath: "研究/调研"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Allocations[0].AssignedKey != "ops_doc_generation" {
		t.Fatalf("research fallback AssignedKey=%q", saved.Allocations[0].AssignedKey)
	}
	if !strings.Contains(saved.Allocations[0].MatchReason, "办公/文档") {
		t.Fatalf("reason=%q", saved.Allocations[0].MatchReason)
	}
}

func TestCapLLMColdStart_BoundsPromptSet(t *testing.T) {
	caps := make([]biz.AgentCapability, 20)
	for i := range caps {
		caps[i].AgentKey = "agent-" + string(rune('a'+i))
	}
	got := capLLMColdStart(caps)
	if len(got) != maxLLMColdStartCandidates {
		t.Fatalf("len=%d want %d", len(got), maxLLMColdStartCandidates)
	}
}

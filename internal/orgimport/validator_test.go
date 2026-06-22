package orgimport

import (
	"strings"
	"testing"
)

func TestValidateSpec_EmptySpec(t *testing.T) {
	err := ValidateSpec(&Spec{})
	if err != nil {
		t.Fatalf("empty spec has no required top-level fields, should be valid; got: %v", err)
	}
}

func TestValidateSpec_MissingIndustryKey(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{Name: "Tech"}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing industry key")
	}
	if !strings.Contains(err.Error(), "key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSpec_MissingIndustryName(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{Key: "tech"}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing industry name")
	}
}

func TestValidateSpec_MissingDepartmentKey(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key:         "tech",
				Name:        "Technology",
				Departments: []DepartmentSpec{{Name: "Engineering"}},
			}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing department key")
	}
}

func TestValidateSpec_MissingPositionKey(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key:  "tech",
				Name: "Technology",
				Departments: []DepartmentSpec{{
					Key:       "eng",
					Name:      "Engineering",
					Positions: []PositionSpec{{Name: "Developer"}},
				}},
			}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing position key")
	}
}

func TestValidateSpec_MissingAgentKey(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Agents: []AgentSpec{{DisplayName: "Bot"}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing agent key")
	}
}

func TestValidateSpec_MissingAgentDisplayName(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Agents: []AgentSpec{{Key: "bot-1"}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for missing agent display_name")
	}
}

func TestValidateSpec_InvalidCategoryPosition(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Agents: []AgentSpec{{
				Key:              "bot-1",
				DisplayName:      "Bot",
				CategoryPosition: "nonexistent/path",
			}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for invalid taxonomy_position")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSpec_TeamNoMembers(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Teams: []TeamSpec{{Key: "team-1", Name: "Alpha"}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for team with no members")
	}
}

func TestValidateSpec_IndustryDescriptionTooLong(t *testing.T) {
	longDesc := strings.Repeat("x", 601)
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key:         "tech",
				Name:        "Technology",
				Description: longDesc,
			}},
		},
	}
	err := ValidateSpec(spec)
	if err == nil {
		t.Fatal("expected error for description exceeding limit")
	}
	if !strings.Contains(err.Error(), "exceeds hard limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSpec_ValidSpec(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key:  "tech",
				Name: "Technology",
				Departments: []DepartmentSpec{{
					Key:  "eng",
					Name: "Engineering",
					Positions: []PositionSpec{{
						Key:  "dev",
						Name: "Developer",
					}},
				}},
			}},
			Agents: []AgentSpec{{
				Key:              "bot-1",
				DisplayName:      "Bot",
				CategoryPosition: "tech/eng/dev",
			}},
			Teams: []TeamSpec{{
				Key:  "team-1",
				Name: "Alpha",
				Members: []MemberSpec{{
					AgentKey: "bot-1",
					Role:     "member",
				}},
			}},
		},
	}
	if err := ValidateSpec(spec); err != nil {
		t.Fatalf("expected valid spec, got: %v", err)
	}
}

func TestBuildPlan_BasicOrder(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key:  "tech",
				Name: "Technology",
				Departments: []DepartmentSpec{{
					Key:  "eng",
					Name: "Engineering",
					Positions: []PositionSpec{{
						Key:  "dev",
						Name: "Developer",
					}},
				}},
			}},
			Agents: []AgentSpec{{Key: "bot-1", DisplayName: "Bot"}},
			Teams:  []TeamSpec{{Key: "team-1", Name: "Alpha"}},
		},
	}
	plan := BuildPlan(spec, EmptyExistingResources{})
	if len(plan.Actions) != 5 {
		t.Fatalf("expected 5 actions, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Kind != "create_industry" {
		t.Fatalf("first action should be industry, got %q", plan.Actions[0].Kind)
	}
	if plan.Actions[1].Kind != "create_department" {
		t.Fatalf("second action should be department, got %q", plan.Actions[1].Kind)
	}
	if plan.Actions[2].Kind != "create_position" {
		t.Fatalf("third action should be position, got %q", plan.Actions[2].Kind)
	}
	if plan.Actions[3].Kind != "create_agent" {
		t.Fatalf("fourth action should be agent, got %q", plan.Actions[3].Kind)
	}
	if plan.Actions[4].Kind != "create_team" {
		t.Fatalf("fifth action should be team, got %q", plan.Actions[4].Kind)
	}
}

func TestBuildPlan_WithExisting(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{Key: "tech", Name: "Technology"}},
		},
	}
	existing := &stubExisting{categories: map[string]bool{"tech": true}}
	plan := BuildPlan(spec, existing)
	if !plan.Actions[0].IsUpdate {
		t.Fatal("existing industry should be marked as update")
	}
}

type stubExisting struct {
	categories map[string]bool
	agents     map[string]bool
	teams      map[string]bool
}

func (s *stubExisting) HasCategory(key string) bool { return s.categories[key] }
func (s *stubExisting) HasAgent(key string) bool    { return s.agents[key] }
func (s *stubExisting) HasTeam(key string) bool     { return s.teams[key] }

func TestFormatPlanTree(t *testing.T) {
	plan := Plan{
		Actions: []PlanAction{
			{Kind: "create_industry", Key: "tech", DisplayName: "Technology"},
			{Kind: "create_agent", Key: "bot-1", DisplayName: "Bot", IsUpdate: true},
		},
	}
	out := FormatPlanTree(plan)
	if !strings.Contains(out, "CREATE") {
		t.Fatal("expected CREATE in output")
	}
	if !strings.Contains(out, "UPDATE") {
		t.Fatal("expected UPDATE in output")
	}
}

func TestStripCodeFence(t *testing.T) {
	input := "```yaml\nkey: value\n```"
	out := stripCodeFence(input)
	if out != "key: value" {
		t.Fatalf("expected 'key: value', got %q", out)
	}
}

func TestStripCodeFence_NoFence(t *testing.T) {
	input := "key: value"
	out := stripCodeFence(input)
	if out != "key: value" {
		t.Fatalf("expected unchanged, got %q", out)
	}
}

func TestStripCodeFence_PlainFence(t *testing.T) {
	input := "```\nkey: value\n```"
	out := stripCodeFence(input)
	if out != "key: value" {
		t.Fatalf("expected 'key: value', got %q", out)
	}
}

func TestHTTPError_IsConflict(t *testing.T) {
	err := &HTTPError{Status: 409}
	if !err.IsConflict() {
		t.Fatal("409 should be conflict")
	}
}

func TestHTTPError_IsNotFound(t *testing.T) {
	err := &HTTPError{Status: 404}
	if !err.IsNotFound() {
		t.Fatal("404 should be not found")
	}
}

func TestHTTPError_Error(t *testing.T) {
	err := &HTTPError{Method: "POST", Path: "/v1/test", Status: 500, Body: "internal error"}
	if !strings.Contains(err.Error(), "POST /v1/test returned 500") {
		t.Fatalf("unexpected error string: %v", err)
	}
}

func TestGenerateCorrelationID(t *testing.T) {
	id1 := generateCorrelationID()
	id2 := generateCorrelationID()
	if id1 == id2 {
		t.Fatal("correlation IDs should be unique")
	}
	if !strings.HasPrefix(id1, "cli-import-") {
		t.Fatalf("expected cli-import- prefix, got %q", id1)
	}
}

func TestNewApplier_DefaultTimeout(t *testing.T) {
	a := NewApplier(ApplyOptions{})
	if a.client.Timeout != 120*1e9 {
		t.Fatalf("expected 120s timeout, got %v", a.client.Timeout)
	}
}

func TestNewApplier_CustomCorrelationID(t *testing.T) {
	a := NewApplier(ApplyOptions{CorrelationID: "test-123"})
	if a.correlationID != "test-123" {
		t.Fatalf("expected test-123, got %q", a.correlationID)
	}
}

func TestApplier_DryRun(t *testing.T) {
	a := NewApplier(ApplyOptions{DryRun: true})
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{Key: "tech", Name: "Technology"}},
			Agents:    []AgentSpec{{Key: "bot-1", DisplayName: "Bot"}},
			Teams:     []TeamSpec{{Key: "team-1", Name: "Alpha", Members: []MemberSpec{{AgentKey: "bot-1", Role: "member"}}}},
		},
	}
	result, err := a.Apply(spec)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 3 {
		t.Fatalf("expected 3 skipped in dry-run, got %d", result.Skipped)
	}
}

func TestCollectPositionPaths(t *testing.T) {
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{
				Key: "tech",
				Departments: []DepartmentSpec{{
					Key:       "eng",
					Positions: []PositionSpec{{Key: "dev"}},
				}},
			}},
		},
	}
	paths := collectPositionPaths(spec)
	if _, ok := paths["tech/eng/dev"]; !ok {
		t.Fatal("expected tech/eng/dev in paths")
	}
}

func TestEmptyExistingResources(t *testing.T) {
	e := EmptyExistingResources{}
	if e.HasCategory("x") || e.HasAgent("x") || e.HasTeam("x") {
		t.Fatal("EmptyExistingResources should always return false")
	}
}

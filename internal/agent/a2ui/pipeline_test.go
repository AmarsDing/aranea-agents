package a2ui

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestStrPtr(t *testing.T) {
	p := StrPtr("hello")
	if p == nil || *p != "hello" {
		t.Fatal("StrPtr failed")
	}
}

func TestNumPtr(t *testing.T) {
	p := NumPtr(3.14)
	if p == nil || *p != 3.14 {
		t.Fatal("NumPtr failed")
	}
}

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || *p != true {
		t.Fatal("BoolPtr failed")
	}
}

func TestPipeline_NextSurfaceID_Increments(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	id1 := p.NextSurfaceID()
	id2 := p.NextSurfaceID()
	if id1 == id2 {
		t.Fatal("surface IDs should be unique")
	}
}

func TestPipeline_StorePlan_GetPlan(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	plan := &Plan{ID: "p1", Goal: "test", Steps: []PlanStep{{ID: "s1", Name: "step1"}}}
	p.StorePlan(plan)
	got, ok := p.GetPlan("p1")
	if !ok {
		t.Fatal("plan not found")
	}
	if got.ID != "p1" || got.Goal != "test" {
		t.Fatalf("%+v", got)
	}
	if len(got.Steps) != 1 || got.Steps[0].Name != "step1" {
		t.Fatalf("steps=%+v", got.Steps)
	}
}

func TestPipeline_GetPlan_NotFound(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	_, ok := p.GetPlan("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent plan")
	}
}

func TestPipeline_GetPlan_DefensiveCopy(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	plan := &Plan{ID: "p1", Goal: "g", Steps: []PlanStep{{ID: "s1"}}}
	p.StorePlan(plan)
	got, _ := p.GetPlan("p1")
	got.Steps[0].Name = "modified"
	orig, _ := p.GetPlan("p1")
	if orig.Steps[0].Name != "" {
		t.Fatal("GetPlan should return a defensive copy")
	}
}

func TestPipeline_GetPlan_DependenciesCopy(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	plan := &Plan{ID: "p1", Dependencies: map[string][]string{"s1": {"s2"}}}
	p.StorePlan(plan)
	got, _ := p.GetPlan("p1")
	got.Dependencies["s1"][0] = "modified"
	orig, _ := p.GetPlan("p1")
	if orig.Dependencies["s1"][0] != "s2" {
		t.Fatal("GetPlan should deep-copy dependencies")
	}
}

func TestPipeline_IsApproval(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	action := &UserAction{Name: "approve", Context: map[string]any{"planId": "p1"}}
	if !p.IsApproval(action) {
		t.Fatal("should be approval")
	}
}

func TestPipeline_IsRejection(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	action := &UserAction{Name: "reject", Context: map[string]any{"planId": "p1"}}
	if !p.IsRejection(action) {
		t.Fatal("should be rejection")
	}
}

func TestPipeline_ActionPlanID(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	action := &UserAction{Name: "approve", Context: map[string]any{"planId": "plan_123"}}
	if got := p.ActionPlanID(action); got != "plan_123" {
		t.Fatalf("got %q", got)
	}
}

func TestPipeline_ActionPlanID_Missing(t *testing.T) {
	p := NewPipeline(loggateway.NewNoop())
	action := &UserAction{Name: "approve", Context: map[string]any{}}
	if got := p.ActionPlanID(action); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSurfaceManager_BeginSurface(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	s, ok := m.GetSurface("s1")
	if !ok {
		t.Fatal("surface not found")
	}
	if s.RootID != "root" {
		t.Fatalf("rootID=%q", s.RootID)
	}
}

func TestSurfaceManager_ApplySurfaceUpdate(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	m.ApplySurfaceUpdate(SurfaceUpdate{
		SurfaceID:  "s1",
		Components: []Component{{ID: "c1", Component: ComponentBody{Text: &TextComponent{Text: DataBinding{LiteralString: StrPtr("hi")}}}}},
	})
	s, _ := m.GetSurface("s1")
	if _, ok := s.Components["c1"]; !ok {
		t.Fatal("component c1 not found")
	}
}

func TestSurfaceManager_ApplySurfaceUpdate_NewSurface(t *testing.T) {
	m := NewSurfaceManager()
	m.ApplySurfaceUpdate(SurfaceUpdate{
		SurfaceID:  "auto",
		Components: []Component{{ID: "c1"}},
	})
	s, ok := m.GetSurface("auto")
	if !ok {
		t.Fatal("auto-created surface not found")
	}
	if _, ok := s.Components["c1"]; !ok {
		t.Fatal("component c1 not found")
	}
}

func TestSurfaceManager_DeleteSurface(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	m.DeleteSurface("s1")
	_, ok := m.GetSurface("s1")
	if ok {
		t.Fatal("surface should be deleted")
	}
}

func TestSurfaceManager_ListSurfaces(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	m.BeginSurface("s2", "root", nil)
	ids := m.ListSurfaces()
	if len(ids) != 2 {
		t.Fatalf("expected 2 surfaces, got %d", len(ids))
	}
}

func TestSurfaceManager_GetSurface_NotFound(t *testing.T) {
	m := NewSurfaceManager()
	_, ok := m.GetSurface("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent surface")
	}
}

func TestSurfaceManager_ApplyDataModelUpdate(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	m.ApplyDataModelUpdate(DataModelUpdate{
		SurfaceID: "s1",
		Contents:  []DataEntry{{Key: "name", ValueString: StrPtr("Alice")}},
	})
	s, _ := m.GetSurface("s1")
	if s.DataModel["name"] != "Alice" {
		t.Fatalf("dataModel=%+v", s.DataModel)
	}
}

func TestSurfaceManager_ApplyDataModelUpdate_NestedPath(t *testing.T) {
	m := NewSurfaceManager()
	m.BeginSurface("s1", "root", nil)
	m.ApplyDataModelUpdate(DataModelUpdate{
		SurfaceID: "s1",
		Contents:  []DataEntry{{Key: "name", ValueString: StrPtr("Alice")}},
	})
	m.ApplyDataModelUpdate(DataModelUpdate{
		SurfaceID: "s1",
		Path:      "/config",
		Contents:  []DataEntry{{Key: "debug", ValueBoolean: BoolPtr(true)}},
	})
	s, _ := m.GetSurface("s1")
	cfg, ok := s.DataModel["config"].(map[string]any)
	if !ok || cfg["debug"] != true {
		t.Fatalf("dataModel=%+v", s.DataModel)
	}
}

func TestSurfaceManager_ApplyDataModelUpdate_MissingSurface(t *testing.T) {
	m := NewSurfaceManager()
	m.ApplyDataModelUpdate(DataModelUpdate{
		SurfaceID: "nonexistent",
		Contents:  []DataEntry{{Key: "k", ValueString: StrPtr("v")}},
	})
}

func TestDataEntryValue_String(t *testing.T) {
	v := dataEntryValue(DataEntry{Key: "k", ValueString: StrPtr("hello")})
	if v != "hello" {
		t.Fatalf("got %v", v)
	}
}

func TestDataEntryValue_Number(t *testing.T) {
	v := dataEntryValue(DataEntry{Key: "k", ValueNumber: NumPtr(42)})
	if v != 42.0 {
		t.Fatalf("got %v", v)
	}
}

func TestDataEntryValue_Boolean(t *testing.T) {
	v := dataEntryValue(DataEntry{Key: "k", ValueBoolean: BoolPtr(true)})
	if v != true {
		t.Fatalf("got %v", v)
	}
}

func TestDataEntryValue_Map(t *testing.T) {
	v := dataEntryValue(DataEntry{Key: "k", ValueMap: []DataEntry{{Key: "inner", ValueString: StrPtr("val")}}})
	m, ok := v.(map[string]any)
	if !ok || m["inner"] != "val" {
		t.Fatalf("got %v", v)
	}
}

func TestDataEntryValue_Nil(t *testing.T) {
	v := dataEntryValue(DataEntry{Key: "k"})
	if v != nil {
		t.Fatalf("got %v", v)
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"/", 0},
		{"a", 1},
		{"/a/b/c", 3},
		{"a/b", 2},
	}
	for _, tt := range tests {
		got := splitPath(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitPath(%q) = %v, want %d parts", tt.input, got, tt.want)
		}
	}
}

func TestGeneratePlanID(t *testing.T) {
	if got := generatePlanID("surface_1"); got != "plan_surface_1" {
		t.Fatalf("got %q", got)
	}
}

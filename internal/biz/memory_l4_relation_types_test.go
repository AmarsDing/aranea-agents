package biz

import "testing"

// TestRelationTypeConstants verifies the 5 relation type constants are defined
// with the correct string values matching the design document §15.3.
func TestRelationTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RelationRelatedTo", RelationRelatedTo, "RELATED_TO"},
		{"RelationEvolvedFrom", RelationEvolvedFrom, "EVOLVED_FROM"},
		{"RelationCausal", RelationCausal, "CAUSAL"},
		{"RelationTemporalNext", RelationTemporalNext, "TEMPORAL_NEXT"},
		{"RelationInhibit", RelationInhibit, "INHIBIT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestRelationTypeProps verifies the relationTypeProps map has correct properties
// for each of the 5 relation types.
func TestRelationTypeProps(t *testing.T) {
	tests := []struct {
		relationType      string
		bidirectional     bool
		reinforcesTarget  bool
		inhibitsTarget    bool
	}{
		{RelationRelatedTo, true, true, false},
		{RelationEvolvedFrom, false, false, true},
		{RelationCausal, false, true, false},
		{RelationTemporalNext, false, false, false},
		{RelationInhibit, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.relationType, func(t *testing.T) {
			prop, ok := relationTypeProps[tt.relationType]
			if !ok {
				t.Fatalf("relationTypeProps missing entry for %q", tt.relationType)
			}
			if prop.Bidirectional != tt.bidirectional {
				t.Errorf("Bidirectional = %v, want %v", prop.Bidirectional, tt.bidirectional)
			}
			if prop.ReinforcesTarget != tt.reinforcesTarget {
				t.Errorf("ReinforcesTarget = %v, want %v", prop.ReinforcesTarget, tt.reinforcesTarget)
			}
			if prop.InhibitsTarget != tt.inhibitsTarget {
				t.Errorf("InhibitsTarget = %v, want %v", prop.InhibitsTarget, tt.inhibitsTarget)
			}
		})
	}
}

// TestRelationTypeProps_AllFiveTypesCovered ensures exactly 5 types are registered.
func TestRelationTypeProps_AllFiveTypesCovered(t *testing.T) {
	if len(relationTypeProps) != 5 {
		t.Errorf("expected 5 relation types in map, got %d", len(relationTypeProps))
	}
}

// TestLookupRelationTypeProp is a convenience test for the lookup helper function.
func TestLookupRelationTypeProp(t *testing.T) {
	// Known type returns prop + true.
	prop, ok := LookupRelationTypeProp(RelationCausal)
	if !ok {
		t.Fatal("LookupRelationTypeProp(CAUSAL) returned ok=false")
	}
	if !prop.ReinforcesTarget {
		t.Error("CAUSAL should reinforce target")
	}

	// Unknown type returns zero prop + false.
	prop, ok = LookupRelationTypeProp("UNKNOWN_TYPE")
	if ok {
		t.Error("LookupRelationTypeProp(UNKNOWN_TYPE) should return ok=false")
	}
	if prop != (RelationTypeProp{}) {
		t.Errorf("unknown type should return zero RelationTypeProp, got %+v", prop)
	}
}

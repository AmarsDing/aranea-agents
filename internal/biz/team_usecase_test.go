package biz_test

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
)

func TestValidateTeamDefinition(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"valid sequential", `{"mode":"sequential","members":[{"agent_id":"a1"}]}`, false},
		{"valid parallel with synthesizer", `{"mode":"parallel","members":[{"agent_id":"a1","role":"synthesizer"},{"agent_id":"a2"}]}`, false},
		{"valid coordinator with synthesizer", `{"mode":"coordinator","members":[{"agent_id":"a1","role":"synthesizer"},{"agent_id":"a2"}]}`, false},
		{"valid critic_loop", `{"mode":"critic_loop","members":[{"agent_id":"a1","role":"generator"},{"agent_id":"a2","role":"critic"},{"agent_id":"a3","role":"synthesizer"}]}`, false},
		{"valid swarm", `{"mode":"swarm","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`, false},
		{"valid adaptive", `{"mode":"adaptive","members":[{"agent_id":"a1"}]}`, false},
		{"invalid json", `{not json}`, true},
		{"unsupported mode", `{"mode":"invalid_mode"}`, true},
		{"empty agent_id", `{"mode":"sequential","members":[{"agent_id":""}]}`, true},
		{"no enabled members", `{"mode":"sequential","members":[{"agent_id":"a1","enabled":false}]}`, true},
		{"parallel without synthesizer multiple members", `{"mode":"parallel","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`, true},
		{"parallel single member ok", `{"mode":"parallel","members":[{"agent_id":"a1"}]}`, false},
		{"coordinator without synthesizer", `{"mode":"coordinator","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`, true},
		{"critic_loop missing critic", `{"mode":"critic_loop","members":[{"agent_id":"a1","role":"generator"}]}`, true},
		{"critic_loop missing generator", `{"mode":"critic_loop","members":[{"agent_id":"a1","role":"critic"}]}`, true},
		{"incompatible role for mode", `{"mode":"sequential","members":[{"agent_id":"a1","role":"synthesizer"}]}`, true},
		{"generator role in parallel", `{"mode":"parallel","members":[{"agent_id":"a1","role":"generator"},{"agent_id":"a2"}]}`, true},
		{"empty members array", `{"mode":"sequential","members":[]}`, false},
		{"parallel with synthesizer_agent_id", `{"mode":"parallel","synthesizer_agent_id":"s1","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`, false},
		{"coordinator with synthesizer_agent_id", `{"mode":"coordinator","synthesizer_agent_id":"s1","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`, false},
		// ADR-08 A4: embedded graph 为拓扑唯一真相源时，跳过 role-mode 耦合校验。
		{"graph skips role-mode compatibility", `{"mode":"sequential","members":[{"agent_id":"a1","role":"coordinator"}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"}],"edges":[]}}`, false},
		{"graph skips parallel synthesizer requirement", `{"mode":"parallel","members":[{"agent_id":"a1"},{"agent_id":"a2"}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"},{"id":"n2","type":"agent","agent_id":"a2"}],"edges":[]}}`, false},
		{"graph skips coordinator requirement", `{"mode":"coordinator","members":[{"agent_id":"a1"},{"agent_id":"a2"}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"},{"id":"n2","type":"agent","agent_id":"a2"}],"edges":[]}}`, false},
		{"graph skips critic_loop requirement", `{"mode":"critic_loop","members":[{"agent_id":"a1","role":"worker"}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"}],"edges":[]}}`, false},
		{"graph still requires enabled member", `{"mode":"sequential","members":[{"agent_id":"a1","enabled":false}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"}],"edges":[]}}`, true},
		{"graph still requires member agent_id", `{"mode":"sequential","members":[{"agent_id":""}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"n1","type":"agent","agent_id":"a1"}],"edges":[]}}`, true},
		{"graph with empty nodes does not skip", `{"mode":"sequential","members":[{"agent_id":"a1","role":"coordinator"}],"graph":{"version":1,"layout":"custom","nodes":[],"edges":[]}}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := biz.ValidateTeamDefinition(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTeamDefinition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	_ = boolTrue
	_ = boolFalse
}

func TestValidRolesForMode(t *testing.T) {
	tests := []struct {
		mode      string
		wantRoles []string
		wantEmpty bool
	}{
		{"critic_loop", []string{"generator", "critic", "synthesizer"}, false},
		{"parallel", []string{"synthesizer"}, false},
		{"coordinator", []string{"synthesizer"}, false},
		{"sequential", []string{"worker"}, false},
		{"swarm", nil, true},
		{"adaptive", nil, true},
		{"unknown", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			roles := biz.ValidRolesForMode(tt.mode)
			if tt.wantEmpty {
				if len(roles) != 0 {
					t.Errorf("ValidRolesForMode(%q) = %v, want empty", tt.mode, roles)
				}
			} else {
				for _, r := range tt.wantRoles {
					if !roles[r] {
						t.Errorf("ValidRolesForMode(%q) missing role %q", tt.mode, r)
					}
				}
			}
		})
	}
}

func TestParseDefinitionForUpdate(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantMode string
		wantLen  int
		wantErr  bool
	}{
		{"empty string", "", "sequential", 0, false},
		{"whitespace", "   ", "sequential", 0, false},
		{"valid definition", `{"version":2,"mode":"parallel","members":[{"agent_id":"a1"}],"max_concurrency":4}`, "parallel", 1, false},
		{"invalid json", `{bad}`, "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := biz.ParseOrchestrationSpec(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOrchestrationSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if d.Mode != tt.wantMode {
					t.Errorf("Mode = %q, want %q", d.Mode, tt.wantMode)
				}
				if len(d.Members) != tt.wantLen {
					t.Errorf("len(Members) = %d, want %d", len(d.Members), tt.wantLen)
				}
			}
		})
	}
}

func TestDefaultTeamDefinitionJSON(t *testing.T) {
	result := biz.DefaultTeamDefinitionJSON()
	if result == "" {
		t.Fatal("DefaultTeamDefinitionJSON() returned empty string")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("DefaultTeamDefinitionJSON() produced invalid JSON: %v", err)
	}
	if mode, ok := parsed["mode"].(string); !ok || mode == "" {
		t.Error("DefaultTeamDefinitionJSON() missing or empty 'mode' field")
	}
	if v, ok := parsed["version"].(float64); !ok || v < 1 {
		t.Error("DefaultTeamDefinitionJSON() missing or invalid 'version' field")
	}
}

func TestRequireNonEmpty(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		domain  string
		field   string
		wantVal string
		wantErr bool
	}{
		{"non-empty value", "hello", "TEST", "field", "hello", false},
		{"whitespace trimmed", "  hello  ", "TEST", "field", "hello", false},
		{"empty value", "", "TEST", "field", "", true},
		{"whitespace only", "   ", "TEST", "field", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := biz.RequireNonEmpty(tt.val, tt.domain, tt.field)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireNonEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.wantVal {
				t.Errorf("RequireNonEmpty() val = %q, want %q", val, tt.wantVal)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{"a non-empty", "first", "second", "first"},
		{"a whitespace non-empty", "  hi  ", "second", "hi"},
		{"a empty", "", "second", "second"},
		{"a whitespace only", "   ", "second", "second"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := biz.FirstNonEmpty(tt.a, tt.b); got != tt.want {
				t.Errorf("FirstNonEmpty(%q,%q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

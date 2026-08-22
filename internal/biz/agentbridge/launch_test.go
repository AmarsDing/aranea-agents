package agentbridge

import "testing"

func TestDefaultACPLaunch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key     string
		command string
		args    []string
		ok      bool
	}{
		{"codebuddy", "codebuddy", []string{"--acp"}, true},
		{"CodeBuddy", "codebuddy", []string{"--acp"}, true},
		{"claude_code", "claude-code-acp", nil, true},
		{"claude-code", "claude-code-acp", nil, true},
		{"codex", "npx", []string{"-y", "@zed-industries/codex-acp"}, true},
		{"unknown", "", nil, false},
	}
	for _, tc := range cases {
		cmd, args, ok := DefaultACPLaunch(tc.key)
		if ok != tc.ok || cmd != tc.command {
			t.Errorf("DefaultACPLaunch(%q) = (%q,%v,%v), want (%q,%v,%v)",
				tc.key, cmd, args, ok, tc.command, tc.args, tc.ok)
		}
		if ok && len(tc.args) > 0 && (len(args) != len(tc.args) || args[0] != tc.args[0]) {
			t.Errorf("DefaultACPLaunch(%q) args = %v, want %v", tc.key, args, tc.args)
		}
	}
}

func TestApplyDefaultLaunch(t *testing.T) {
	t.Parallel()
	ag := &CodingAgent{AgentKey: "codebuddy"}
	ApplyDefaultLaunch(ag)
	if ag.Command != "codebuddy" || len(ag.Args) != 1 || ag.Args[0] != "--acp" {
		t.Fatalf("codebuddy default = %+v", ag)
	}
	keep := &CodingAgent{AgentKey: "codebuddy", Command: "custom", Args: []string{"--flag"}}
	ApplyDefaultLaunch(keep)
	if keep.Command != "custom" || keep.Args[0] != "--flag" {
		t.Fatalf("explicit argv must be kept, got %+v", keep)
	}
}

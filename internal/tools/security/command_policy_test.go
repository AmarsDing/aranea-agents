package security

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestCommandSafetyPolicy_ProtectedPath(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("exec_command", []byte(`{"command": "cat ~/.ssh/id_rsa"}`))
	if violation == nil {
		t.Fatal("expected violation for accessing .ssh/id_rsa")
	}
	if violation.Rule != "sensitive_path_access" {
		t.Fatalf("expected rule sensitive_path_access, got %s", violation.Rule)
	}
}

func TestCommandSafetyPolicy_SSHGlobPattern(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("exec_command", []byte(`{"command": "cat ~/.ssh/authorized_keys"}`))
	if violation == nil {
		t.Fatal("expected violation for accessing .ssh/authorized_keys (glob .ssh/*)")
	}
	violation = policy.Evaluate("exec_command", []byte(`{"command": "cat ~/.ssh/config"}`))
	if violation == nil {
		t.Fatal("expected violation for accessing .ssh/config (glob .ssh/*)")
	}
}

func TestCommandSafetyPolicy_RecursiveGlobEnv(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("read_file", []byte(`{"path": "/home/user/project/.env"}`))
	if violation == nil {
		t.Fatal("expected violation for **/.env pattern")
	}
	violation = policy.Evaluate("read_file", []byte(`{"path": "/app/config/.env.production"}`))
	if violation == nil {
		t.Fatal("expected violation for **/.env.* pattern")
	}
}

func TestCommandSafetyPolicy_RecursiveGlobCredentials(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("read_file", []byte(`{"path": "/opt/secrets/credentials.json"}`))
	if violation == nil {
		t.Fatal("expected violation for **/credentials.json pattern")
	}
	violation = policy.Evaluate("read_file", []byte(`{"path": "/etc/service-account-prod.json"}`))
	if violation == nil {
		t.Fatal("expected violation for **/service-account*.json pattern")
	}
}

func TestCommandSafetyPolicy_AllowedPath(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("exec_command", []byte(`{"command": "cat /etc/hosts"}`))
	if violation != nil {
		t.Fatalf("unexpected violation: %v", violation)
	}
}

func TestCommandSafetyPolicy_NoFalsePositiveOnDescription(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("exec_command", []byte(`{"command": "echo copy the .environment variables guide"}`))
	if violation != nil {
		t.Fatalf("expected no violation for non-path .env mention, got: %v", violation)
	}
}

func TestCommandSafetyPolicy_NoFalsePositiveOnBashrcInDescription(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("exec_command", []byte(`{"command": "echo how to set up bashrc aliases"}`))
	if violation != nil {
		t.Fatalf("expected no violation for non-path .bashrc mention, got: %v", violation)
	}
}

func TestCommandSafetyPolicy_UnprotectedTool(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("web_research", []byte(`{"query": ".ssh/id_rsa"}`))
	if violation != nil {
		t.Fatalf("unexpected violation for unprotected tool: %v", violation)
	}
}

func TestCommandSafetyPolicy_AWSCredentials(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("read_file", []byte(`{"file_path": "/home/user/.aws/credentials"}`))
	if violation == nil {
		t.Fatal("expected violation for accessing .aws/credentials")
	}
}

func TestCommandSafetyPolicy_KubeConfig(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	violation := policy.Evaluate("shell_exec", []byte(`{"command": "cat ~/.kube/config"}`))
	if violation == nil {
		t.Fatal("expected violation for accessing .kube/config")
	}
}

func TestCommandSafetyPolicy_IsProtectedTool(t *testing.T) {
	policy := NewCommandSafetyPolicy(loggateway.NewNoop())
	if !policy.IsProtectedTool("exec_command") {
		t.Fatal("exec_command should be a protected tool")
	}
	if !policy.IsProtectedTool("read_file") {
		t.Fatal("read_file should be a protected tool")
	}
	if policy.IsProtectedTool("duckduckgo") {
		t.Fatal("duckduckgo should not be a protected tool")
	}
}

func TestCommandSafetyPolicyWithConfig(t *testing.T) {
	policy := NewCommandSafetyPolicyWithConfig(
		loggateway.NewNoop(),
		[]string{"custom_secret_dir/*"},
		map[string]bool{"custom_tool": true},
	)
	if !policy.IsProtectedTool("custom_tool") {
		t.Fatal("custom_tool should be protected")
	}
	if !policy.IsProtectedTool("read_file") {
		t.Fatal("default read_file tool should still be protected")
	}
	violation := policy.Evaluate("custom_tool", []byte(`{"path": "/opt/custom_secret_dir/data.pem"}`))
	if violation == nil {
		t.Fatal("expected violation for custom protected path")
	}
}

func TestPolicyViolation_Error(t *testing.T) {
	v := &PolicyViolation{
		ToolName: "exec_command",
		Rule:     "sensitive_path_access",
		Path:     ".ssh/id_rsa",
		Message:  "Access to sensitive path is blocked for security",
	}
	if v.Error() != v.Message {
		t.Fatalf("expected Error() to return Message, got %s", v.Error())
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{".ssh/*", "./.ssh/id_rsa", true},
		{".ssh/*", "./.ssh/config", true},
		{".ssh/*", "./.ssh/known_hosts", true},
		{".ssh/*", "./.aws/credentials", false},
		{"**/.env", "./project/.env", true},
		{"**/.env", "./.env", true},
		{"**/.env.*", "./project/.env.local", true},
		{"**/credentials.json", "./secrets/credentials.json", true},
		{"**/service-account*.json", "./config/service-account-prod.json", true},
		{"**/id_rsa*", "./.ssh/id_rsa.pub", true},
		{".aws/credentials", "./.aws/credentials", true},
		{".aws/credentials", "/home/user/.aws/credentials", true},
		{".aws/credentials", "./other/credentials", false},
	}
	for _, tt := range tests {
		got := matchPath(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

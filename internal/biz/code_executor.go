package biz

import (
	"strings"

	"aranea-agents/pkg/apierror"
)

const (
	CodeExecutorLocal     = "local"
	CodeExecutorDocker    = "docker"
	CodeExecutorE2B       = "e2b"
	CodeExecutorContainer = "container"
)

// ValidCodeExecutorTypes lists allowed AgentRuntimeSettings.CodeExecutorType values.
func ValidCodeExecutorTypes() []string {
	return []string{
		CodeExecutorLocal,
		CodeExecutorDocker,
		CodeExecutorE2B,
		CodeExecutorContainer,
	}
}

// ValidateCodeExecutorType rejects unknown backend identifiers at persistence boundary.
func ValidateCodeExecutorType(raw string) error {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return nil
	}
	for _, allowed := range ValidCodeExecutorTypes() {
		if t == allowed {
			return nil
		}
	}
	return apierror.BadRequest("AGENT", "invalid code_executor_type %q; allowed: local, docker, e2b, container", raw)
}

package service

import (
	"fmt"
	"os"
	"strings"
)

// ResolveSecretRef resolves MVP secret_ref forms. Supported: env:VAR_NAME.
func ResolveSecretRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty secret_ref")
	}
	if strings.HasPrefix(ref, "env:") {
		name := strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
		if name == "" {
			return "", fmt.Errorf("env: variable name is empty")
		}
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("environment variable %q is not set", name)
		}
		return v, nil
	}
	if strings.HasPrefix(ref, "local:") {
		return "", fmt.Errorf("local: secret_ref cannot be resolved at runtime; configure env:VAR_NAME instead")
	}
	return "", fmt.Errorf("unsupported secret_ref (use env:NAME)")
}

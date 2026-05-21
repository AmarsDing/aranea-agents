package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/biz"
)

// ResolveSecretRef resolves secret_ref forms: env:VAR_NAME and enc: (AES-GCM blob).
func ResolveSecretRef(ctx context.Context, ref string) (string, error) {
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
	if strings.HasPrefix(ref, "enc:") {
		plain, err := biz.DecryptChannelSecretRef(ctx, ref)
		if err != nil {
			return "", err
		}
		return plain, nil
	}
	if strings.HasPrefix(ref, "local:") {
		return "", fmt.Errorf("local: secret_ref is deprecated; re-save credentials or use env:VAR_NAME")
	}
	return "", fmt.Errorf("unsupported secret_ref (use env:NAME or enc:)")
}

package service

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ResolveSecretRef resolves secret_ref forms: env:VAR_NAME and enc: (AES-GCM blob).
func ResolveSecretRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", kerrors.BadRequest("SECRET", "empty secret_ref")
	}
	if strings.HasPrefix(ref, "env:") {
		name := strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
		if name == "" {
			return "", kerrors.BadRequest("SECRET", "env: variable name is empty")
		}
		v := os.Getenv(name)
		if v == "" {
			return "", kerrors.NotFound("SECRET", "environment variable "+name+" is not set")
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
		return "", kerrors.BadRequest("SECRET", "local: secret_ref is deprecated; re-save credentials or use env:VAR_NAME")
	}
	return "", kerrors.BadRequest("SECRET", "unsupported secret_ref (use env:NAME or enc:)")
}

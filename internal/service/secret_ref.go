package service

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func ResolveSecretRef(ctx context.Context, channels *biz.ChannelUsecase, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", apierror.BadRequest("SECRET", "empty secret_ref")
	}
	if strings.HasPrefix(ref, "env:") {
		name := strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
		if name == "" {
			return "", apierror.BadRequest("SECRET", "env: variable name is empty")
		}
		v := os.Getenv(name)
		if v == "" {
			return "", apierror.NotFound("SECRET", "environment variable "+name+" is not set")
		}
		return v, nil
	}
	if strings.HasPrefix(ref, "enc:") {
		plain, err := channels.DecryptSecretRef(ctx, ref)
		if err != nil {
			return "", err
		}
		return plain, nil
	}
	if strings.HasPrefix(ref, "local:") {
		return "", apierror.BadRequest("SECRET", "local: secret_ref is deprecated; re-save credentials or use env:VAR_NAME")
	}
	return "", apierror.BadRequest("SECRET", "unsupported secret_ref (use env:NAME or enc:)")
}

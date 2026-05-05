package data

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/admin"
	authpkg "aranea-agents/pkg/auth"
)

// DevBypassAdminPassword is the plaintext password for the seeded id=1 admin when KRATOS_HTTP_AUTH_DISABLED is set.
const DevBypassAdminPassword = "dev"

func adminPwdMD5(plain string) string {
	sum := md5.Sum([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func ensureDevBypassAdminIfEnabled(ctx context.Context, client *ent.Client) error {
	if !authpkg.HTTPAuthBypassEnabled() || client == nil {
		return nil
	}
	ok, err := client.Admin.Query().Where(admin.ID(1)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("dev admin lookup: %w", err)
	}
	if ok {
		return nil
	}
	_, err = client.Admin.Create().
		SetID(1).
		SetName("dev").
		SetEmail("dev@local.invalid").
		SetAccess("admin").
		SetAvatar("").
		SetPassword(adminPwdMD5(DevBypassAdminPassword)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed dev admin: %w", err)
	}
	return nil
}

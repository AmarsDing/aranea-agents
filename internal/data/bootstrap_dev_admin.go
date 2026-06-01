package data

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/admin"
	authpkg "aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// DevBypassAdminPassword is the plaintext password for the seeded id=1 admin when KRATOS_HTTP_AUTH_DISABLED is set.
const DevBypassAdminPassword = "dev"

func adminPwdMD5(plain string) string {
	sum := md5.Sum([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func ensureDevBypassAdminIfEnabled(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if !authpkg.HTTPAuthBypassEnabled() || client == nil {
		return nil
	}
	existing, err := client.Admin.Query().Where(admin.ID(1)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.dev_admin_lookup"), loggateway.Err(err))
		return fmt.Errorf("dev admin lookup: %w", err)
	}
	wantPwd := adminPwdMD5(DevBypassAdminPassword)
	if existing != nil {
		if existing.Name == "dev" && existing.Password == wantPwd {
			lg.Info("dev admin already exists and up-to-date",
				loggateway.StepID("admin.dev_seed"))
			return nil
		}
		_, err = client.Admin.UpdateOneID(1).
			SetName("dev").
			SetEmail("dev@local.invalid").
			SetAccess("admin").
			SetPassword(wantPwd).
			Save(ctx)
		if err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.dev_admin_sync"), loggateway.Err(err))
			return fmt.Errorf("sync dev admin: %w", err)
		}
		lg.Info("dev admin synced (bypass mode)",
			loggateway.StepID("admin.dev_seed"))
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
		lg.Warn("seed step failed", loggateway.StepID("data.seed.dev_admin_create"), loggateway.Err(err))
		return fmt.Errorf("seed dev admin: %w", err)
	}
	lg.Info("dev admin seeded (bypass mode)",
		loggateway.StepID("admin.dev_seed"))
	return nil
}

package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureInitialAdminFromConfig(ctx context.Context, client *ent.Client, d *conf.Data) error {
	if client == nil || d == nil {
		return nil
	}
	ia := d.GetInitialAdmin()
	if ia == nil {
		return nil
	}
	name := ia.GetName()
	pwd := ia.GetPassword()
	if name == "" || pwd == "" {
		return nil
	}
	count, err := client.Admin.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("initial admin count: %w", err)
	}
	if count > 0 {
		loggateway.Global().Info("initial admin skipped: admins already exist",
			loggateway.StepID("system.admin.seed"), loggateway.Int("count", count))
		return nil
	}
	_, err = client.Admin.Create().
		SetName(name).
		SetEmail(ia.GetEmail()).
		SetAccess(ia.GetAccess()).
		SetAvatar("").
		SetPassword(adminPwdMD5(pwd)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed initial admin: %w", err)
	}
	loggateway.Global().Info("initial admin seeded from config",
		loggateway.StepID("system.admin.seed"), loggateway.Str("admin_name", name))
	return nil
}

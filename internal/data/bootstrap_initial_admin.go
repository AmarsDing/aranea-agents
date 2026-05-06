package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"
)

// ensureInitialAdminFromConfig inserts id=1 admin when the admins table is empty and
// data.initial_admin is set in YAML (password plaintext → MD5 hex, same as Login).
func ensureInitialAdminFromConfig(ctx context.Context, client *ent.Client, d *conf.Data) error {
	if client == nil || d == nil {
		return nil
	}
	ia := d.GetInitialAdmin()
	if ia == nil {
		return nil
	}
	password := strings.TrimSpace(ia.GetPassword())
	if password == "" {
		return nil
	}
	name := strings.TrimSpace(ia.GetName())
	email := strings.TrimSpace(ia.GetEmail())
	if name == "" && email == "" {
		return nil
	}
	if name == "" {
		name = "admin"
	}
	if email == "" {
		email = name + "@local.invalid"
	}
	access := strings.TrimSpace(ia.GetAccess())
	if access == "" {
		access = "admin"
	}
	n, err := client.Admin.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("initial admin: count admins: %w", err)
	}
	if n > 0 {
		return nil
	}
	_, err = client.Admin.Create().
		SetID(1).
		SetName(name).
		SetEmail(email).
		SetAccess(access).
		SetAvatar("").
		SetPassword(adminPwdMD5(password)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("initial admin: seed: %w", err)
	}
	return nil
}

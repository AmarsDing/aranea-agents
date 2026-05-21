package biz

import (
	"strings"

	"aranea-agents/pkg/webhookurl"

	"github.com/go-kratos/kratos/v2/errors"
)

// ValidateHookConfigForSave checks config_json before Hook CRUD persistence.
func ValidateHookConfigForSave(configJSON string) error {
	cfg, err := ParseHookConfig(configJSON)
	if err != nil {
		return errors.BadRequest("HOOK", "invalid config_json: "+err.Error())
	}
	action := strings.ToLower(strings.TrimSpace(cfg.Action.Type))
	if action != "notify" {
		return nil
	}
	url := strings.TrimSpace(cfg.Action.WebhookURL)
	if url == "" {
		return errors.BadRequest("HOOK", "webhook_url required for notify action")
	}
	if err := webhookurl.ValidateNotifyURL(url); err != nil {
		return errors.BadRequest("HOOK", err.Error())
	}
	return nil
}

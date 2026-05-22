package lark

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
)

// AppAndRegionFromConfig reads config_json.config.app_id and region for Feishu/Lark APIs.
func AppAndRegionFromConfig(configJSON string) (region, appID string, err error) {
	var cfg struct {
		Config struct {
			AppID  string `json:"app_id"`
			Region string `json:"region"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &cfg); err != nil {
		return "", "", fmt.Errorf("feishu config: %w", err)
	}
	appID = strings.TrimSpace(cfg.Config.AppID)
	if appID == "" {
		return "", "", fmt.Errorf("feishu config_json.config.app_id is required")
	}
	region = strings.TrimSpace(strings.ToLower(cfg.Config.Region))
	if region == "" {
		region = RegionFeishu
	}
	return region, appID, nil
}

// WSAppCredentials resolves app_id + app_secret for larkws long connection.
func WSAppCredentials(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
) (appID, appSecret string, err error) {
	_, appID, err = AppAndRegionFromConfig(ch.ConfigJSON)
	if err != nil {
		return "", "", err
	}
	appSecret, err = lookup(ctx, creds, "app_secret")
	appSecret = strings.TrimSpace(appSecret)
	if appSecret == "" {
		return "", "", fmt.Errorf("feishu websocket: app_secret required")
	}
	return appID, appSecret, nil
}

package lark

import (
	"context"
	"encoding/json"
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
		return "", "", feishuParseError("feishu config", err)
	}
	appID = strings.TrimSpace(cfg.Config.AppID)
	if appID == "" {
		return "", "", errAppIDRequired
	}
	region = strings.TrimSpace(strings.ToLower(cfg.Config.Region))
	if region == "" {
		region = RegionFeishu
	}
	return region, appID, nil
}

// FeishuChannelConfig holds Feishu-specific config_json.config flags.
type FeishuChannelConfig struct {
	ThreadSessionsPerUser bool
	ReplyInThread         bool
	ProcessingReaction    bool
}

// ParseFeishuChannelConfig reads thread/reaction toggles from config_json.
func ParseFeishuChannelConfig(configJSON string) FeishuChannelConfig {
	var cfg struct {
		Config struct {
			ThreadSessionsPerUser bool `json:"thread_sessions_per_user"`
			ReplyInThread         bool `json:"reply_in_thread"`
			ProcessingReaction    bool `json:"processing_reaction"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &cfg) != nil {
		return FeishuChannelConfig{}
	}
	return FeishuChannelConfig{
		ThreadSessionsPerUser: cfg.Config.ThreadSessionsPerUser,
		ReplyInThread:         cfg.Config.ReplyInThread,
		ProcessingReaction:    cfg.Config.ProcessingReaction,
	}
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
		return "", "", errAppSecretRequired
	}
	return appID, appSecret, nil
}

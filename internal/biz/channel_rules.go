package biz

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

type ChannelConfig struct {
	Type        string         `json:"type"`
	Variant     string         `json:"variant"`
	ReceiveMode string         `json:"receive_mode"`
	Webhook     map[string]any `json:"webhook"`
	Routing     map[string]any `json:"routing"`
	Config      map[string]any `json:"config"`
	Accounts    []any          `json:"accounts"`
}

func channelValidationError(format string, args ...any) error {
	return errors.BadRequest("CHANNEL", fmt.Sprintf(format, args...))
}

func catalogHasType(channelType string) bool {
	for _, item := range channelTypeRegistry {
		if item.Type == channelType {
			return true
		}
	}
	return false
}

func catalogSorted() []ChannelTypeItem {
	items := append([]ChannelTypeItem{}, channelTypeRegistry...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Type < items[j].Type
	})
	return items
}

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func compactJSON(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}
	out, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(out)
}

func parseChannelConfig(raw string) (ChannelConfig, error) {
	raw = defaultJSON(raw)
	var cfg ChannelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ChannelConfig{}, channelValidationError("config_json must be valid JSON")
	}
	cfg.Type = strings.TrimSpace(cfg.Type)
	cfg.ReceiveMode = strings.TrimSpace(cfg.ReceiveMode)
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	return cfg, nil
}

func normalizeChannel(row *Channel) error {
	row.Key = strings.TrimSpace(row.Key)
	row.Name = strings.TrimSpace(row.Name)
	row.Status = strings.TrimSpace(row.Status)
	if row.Key == "" || row.Name == "" {
		return channelValidationError("key and name are required")
	}
	if row.Status == "" {
		row.Status = "active"
	}
	cfg, err := parseChannelConfig(row.ConfigJSON)
	if err != nil {
		return err
	}
	if cfg.Type == "" {
		return channelValidationError("config_json.type is required")
	}
	if !catalogHasType(cfg.Type) {
		return channelValidationError("unsupported channel type: %s", cfg.Type)
	}
	row.ConfigJSON = compactJSON(row.ConfigJSON, "{}")
	row.MetadataJSON = compactJSON(row.MetadataJSON, "{}")
	return nil
}

func requiredCredentials(channelType string) []string {
	switch channelType {
	case "telegram":
		return []string{"bot_token"}
	case "feishu":
		return []string{"app_secret"}
	case "slack":
		return []string{"bot_token", "signing_secret"}
	case "discord":
		return []string{"bot_token"}
	case "wechat":
		return []string{"app_secret"}
	case "dingtalk":
		return []string{"secret"}
	case "wecom", "wecom-app":
		return []string{"token"}
	case "qq":
		return []string{"app_secret"}
	case "personal_qq":
		return []string{"receive_token", "send_token"}
	case "line":
		return []string{"channel_secret", "channel_token"}
	case "mattermost":
		return []string{"server_url", "bot_token"}
	case "teams":
		return []string{"app_id", "app_secret"}
	default:
		return nil
	}
}

func missingCredentials(credentials []ChannelCredential, required []string) []string {
	available := map[string]bool{}
	for _, item := range credentials {
		if strings.TrimSpace(item.SecretRef) != "" && item.DeletedAt == "" {
			available[item.CredentialKey] = true
		}
	}
	var missing []string
	for _, key := range required {
		if !available[key] {
			missing = append(missing, key)
		}
	}
	return missing
}

func supportsLightweightTest(channelType string) bool {
	switch channelType {
	case "qq", "personal_qq", "feishu", "wechat", "wecom", "wecom-app", "telegram", "slack", "discord", "dingtalk":
		return true
	default:
		return false
	}
}

func credentialCount(credentials []ChannelCredential) int {
	count := 0
	for _, item := range credentials {
		if strings.TrimSpace(item.SecretRef) != "" {
			count++
		}
	}
	return count
}

func evaluateChannelTest(row Channel, cfg ChannelConfig, credentials []ChannelCredential) ChannelTestResult {
	if !row.Enabled {
		return ChannelTestResult{OK: false, Status: "disabled", Message: "channel is saved but currently disabled"}
	}
	required := requiredCredentials(cfg.Type)
	missing := missingCredentials(credentials, required)
	if len(missing) > 0 {
		return ChannelTestResult{OK: false, Status: "pending_auth", Message: "missing credentials: " + strings.Join(missing, ", ")}
	}
	if cfg.ReceiveMode == "webhook" {
		if cfg.Webhook == nil || strings.TrimSpace(fmt.Sprint(cfg.Webhook["path"])) == "" {
			return ChannelTestResult{OK: false, Status: "pending_config", Message: "webhook.path is required for webhook channels"}
		}
	}
	if cfg.Type == "feishu" {
		appID := strings.TrimSpace(fmt.Sprint(cfg.Config["app_id"]))
		if appID == "" || appID == "<nil>" {
			return ChannelTestResult{
				OK:      false,
				Status:  "pending_config",
				Message: "config_json.config.app_id is required (Feishu App ID from open platform, e.g. cli_xxx)",
			}
		}
	}
	if cfg.Type == "telegram" {
		return ChannelTestResult{OK: true, Status: "ok", Message: "Telegram credentials are configured; live getMe is deferred until secret storage is connected"}
	}
	if supportsLightweightTest(cfg.Type) {
		return ChannelTestResult{OK: true, Status: "ok", Message: "channel configuration is structurally valid"}
	}
	return ChannelTestResult{OK: false, Status: "unsupported", Message: "live connection test is not implemented for this channel type yet"}
}

func errorMessageForTest(result ChannelTestResult) string {
	if result.OK {
		return ""
	}
	return result.Message
}

// EvaluateChannelTest runs structural/policy checks without persisting deliveries.
func EvaluateChannelTest(row Channel, credentials []ChannelCredential) (ChannelTestResult, error) {
	cfg, err := parseChannelConfig(row.ConfigJSON)
	if err != nil {
		return ChannelTestResult{}, err
	}
	return evaluateChannelTest(row, cfg, credentials), nil
}

func sanitizeCredentials(items []ChannelCredential) []ChannelCredential {
	out := make([]ChannelCredential, len(items))
	for i, item := range items {
		item.Configured = strings.TrimSpace(item.SecretRef) != ""
		item.MaskedPreview = maskReference(item.SecretRef)
		item.SecretRef = ""
		out[i] = item
	}
	return out
}

func maskReference(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if len(ref) <= 12 {
		return "********"
	}
	return ref[:8] + "..." + ref[len(ref)-4:]
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

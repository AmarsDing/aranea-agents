package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type ChannelRuntimeReloader interface {
	ReloadChannels(context.Context) error
}

type ChannelService struct {
	repo    repository.Store
	runtime ChannelRuntimeReloader
}

func NewChannelService(repo repository.Store) *ChannelService {
	return &ChannelService{repo: repo}
}

func (s *ChannelService) SetRuntimeReloader(runtime ChannelRuntimeReloader) {
	s.runtime = runtime
}

func (s *ChannelService) Catalog() []domain.ChannelCatalogItem {
	items := append([]domain.ChannelCatalogItem{}, channelCatalog...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Type < items[j].Type
	})
	return items
}

func (s *ChannelService) List() ([]domain.PlatformResource, error) {
	return s.repo.ListPlatformResources("channels")
}

func (s *ChannelService) Get(id string) (domain.PlatformResource, error) {
	return s.repo.GetPlatformResource("channels", strings.TrimSpace(id))
}

func (s *ChannelService) Create(in domain.PlatformResource, credentials []domain.ChannelCredentialInput) (domain.PlatformResource, error) {
	in.Resource = "channels"
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		in.ID = newID()
	}
	if err := normalizeChannelResource(&in); err != nil {
		return domain.PlatformResource{}, err
	}
	created, err := s.repo.CreatePlatformResource(in)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if _, err = s.UpsertCredentials(created.ID, credentials); err != nil {
		return domain.PlatformResource{}, err
	}
	_ = s.reload(context.Background())
	return created, nil
}

func (s *ChannelService) Update(id string, in domain.PlatformResource, credentials []domain.ChannelCredentialInput) (domain.PlatformResource, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.PlatformResource{}, validationError("id is required")
	}
	current, err := s.repo.GetPlatformResource("channels", id)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	in.ID = id
	in.Resource = "channels"
	if in.Key == "" {
		in.Key = current.Key
	}
	if in.Name == "" {
		in.Name = current.Name
	}
	if in.ConfigJSON == "" {
		in.ConfigJSON = current.ConfigJSON
	}
	if in.MetadataJSON == "" {
		in.MetadataJSON = current.MetadataJSON
	}
	if in.Status == "" {
		in.Status = current.Status
	}
	if err = normalizeChannelResource(&in); err != nil {
		return domain.PlatformResource{}, err
	}
	updated, err := s.repo.UpdatePlatformResource(in)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if _, err = s.UpsertCredentials(updated.ID, credentials); err != nil {
		return domain.PlatformResource{}, err
	}
	_ = s.reload(context.Background())
	return updated, nil
}

func (s *ChannelService) Delete(id string) error {
	if err := s.repo.DeletePlatformResource("channels", strings.TrimSpace(id)); err != nil {
		return err
	}
	return s.reload(context.Background())
}

func (s *ChannelService) Toggle(id string, enabled bool) (domain.PlatformResource, error) {
	row, err := s.repo.GetPlatformResource("channels", strings.TrimSpace(id))
	if err != nil {
		return domain.PlatformResource{}, err
	}
	row.Enabled = enabled
	if row.Status == "" || row.Status == "deleted" {
		row.Status = "active"
	}
	updated, err := s.repo.UpdatePlatformResource(row)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	return updated, s.reload(context.Background())
}

func (s *ChannelService) ListCredentials(channelID string) ([]domain.ChannelCredential, error) {
	items, err := s.repo.ListChannelCredentials(strings.TrimSpace(channelID))
	if err != nil {
		return nil, err
	}
	return sanitizeCredentials(items), nil
}

func (s *ChannelService) UpsertCredentials(channelID string, inputs []domain.ChannelCredentialInput) ([]domain.ChannelCredential, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, validationError("channel id is required")
	}
	var result []domain.ChannelCredential
	for _, input := range inputs {
		key := strings.TrimSpace(input.CredentialKey)
		if key == "" {
			continue
		}
		secret := strings.TrimSpace(input.Secret)
		secretRef := strings.TrimSpace(input.SecretRef)
		if secret == "" && secretRef == "" {
			continue
		}
		if secretRef == "" {
			secretRef = localSecretRef(channelID, key, secret)
		}
		status := strings.TrimSpace(input.Status)
		if status == "" {
			status = "active"
		}
		metadata := strings.TrimSpace(input.MetadataJSON)
		if metadata == "" {
			metadata = "{}"
		}
		if !json.Valid([]byte(metadata)) {
			return nil, validationError("credential %s metadata_json must be valid JSON", key)
		}
		created, err := s.repo.UpsertChannelCredential(domain.ChannelCredential{
			ID:            newID(),
			ChannelID:     channelID,
			CredentialKey: key,
			Status:        status,
			SecretRef:     secretRef,
			MetadataJSON:  metadata,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, created)
	}
	return sanitizeCredentials(result), nil
}

func (s *ChannelService) DeleteCredential(channelID string, key string) error {
	return s.repo.DeleteChannelCredential(strings.TrimSpace(channelID), strings.TrimSpace(key))
}

func (s *ChannelService) ListDeliveries(channelID string, limit int) ([]domain.ChannelDelivery, error) {
	return s.repo.ListChannelDeliveries(strings.TrimSpace(channelID), limit)
}

func (s *ChannelService) Test(ctx context.Context, id string) (domain.ChannelTestResult, error) {
	row, err := s.repo.GetPlatformResource("channels", strings.TrimSpace(id))
	if err != nil {
		return domain.ChannelTestResult{}, err
	}
	cfg, err := parseChannelConfig(row.ConfigJSON)
	if err != nil {
		return domain.ChannelTestResult{}, err
	}
	credentials, err := s.repo.ListChannelCredentials(row.ID)
	if err != nil {
		return domain.ChannelTestResult{}, err
	}
	result := evaluateChannelTest(row, cfg, credentials)
	payload, _ := json.Marshal(map[string]any{
		"type":          cfg.Type,
		"receive_mode":  cfg.ReceiveMode,
		"credential_ok": credentialCount(credentials),
		"result_status": result.Status,
	})
	_, _ = s.repo.AddChannelDelivery(domain.ChannelDelivery{
		ID:           newID(),
		ChannelID:    row.ID,
		Status:       result.Status,
		PayloadJSON:  string(payload),
		ErrorMessage: errorMessageForTest(result),
	})
	return result, s.updateTestMetadata(row, result)
}

func (s *ChannelService) EnabledChannelConfigs(ctx context.Context) ([]domain.ChannelRuntimeConfig, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return s.repo.ListEnabledChannelRuntimeConfigs()
}

func (s *ChannelService) reload(ctx context.Context) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.ReloadChannels(ctx)
}

func (s *ChannelService) updateTestMetadata(row domain.PlatformResource, result domain.ChannelTestResult) error {
	var metadata map[string]any
	if json.Unmarshal([]byte(defaultJSON(row.MetadataJSON)), &metadata) != nil {
		metadata = map[string]any{}
	}
	if result.OK {
		metadata["last_error_code"] = ""
		metadata["last_error_message"] = ""
		metadata["connected_at"] = nowUTCString()
		row.Status = "active"
	} else {
		metadata["last_error_code"] = result.Status
		metadata["last_error_message"] = result.Message
		if result.Status == "error" {
			row.Status = "error"
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row.MetadataJSON = string(raw)
	_, err = s.repo.UpdatePlatformResource(row)
	return err
}

type channelConfigEnvelope struct {
	Type        string         `json:"type"`
	Variant     string         `json:"variant"`
	ReceiveMode string         `json:"receive_mode"`
	Webhook     map[string]any `json:"webhook"`
	Routing     map[string]any `json:"routing"`
	Config      map[string]any `json:"config"`
	Accounts    []any          `json:"accounts"`
}

func normalizeChannelResource(row *domain.PlatformResource) error {
	row.Key = strings.TrimSpace(row.Key)
	row.Name = strings.TrimSpace(row.Name)
	row.Status = strings.TrimSpace(row.Status)
	if row.Key == "" || row.Name == "" {
		return validationError("key and name are required")
	}
	if row.Status == "" {
		row.Status = "active"
	}
	cfg, err := parseChannelConfig(row.ConfigJSON)
	if err != nil {
		return err
	}
	if cfg.Type == "" {
		return validationError("config_json.type is required")
	}
	if !catalogHasType(cfg.Type) {
		return validationError("unsupported channel type: %s", cfg.Type)
	}
	row.ConfigJSON = compactJSON(row.ConfigJSON, "{}")
	row.MetadataJSON = compactJSON(row.MetadataJSON, "{}")
	return nil
}

func parseChannelConfig(raw string) (channelConfigEnvelope, error) {
	raw = defaultJSON(raw)
	var cfg channelConfigEnvelope
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return channelConfigEnvelope{}, validationError("config_json must be valid JSON")
	}
	cfg.Type = strings.TrimSpace(cfg.Type)
	cfg.ReceiveMode = strings.TrimSpace(cfg.ReceiveMode)
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	return cfg, nil
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

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func evaluateChannelTest(row domain.PlatformResource, cfg channelConfigEnvelope, credentials []domain.ChannelCredential) domain.ChannelTestResult {
	if !row.Enabled {
		return domain.ChannelTestResult{OK: false, Status: "disabled", Message: "channel is saved but currently disabled"}
	}
	required := requiredCredentials(cfg.Type)
	missing := missingCredentials(credentials, required)
	if len(missing) > 0 {
		return domain.ChannelTestResult{OK: false, Status: "pending_auth", Message: "missing credentials: " + strings.Join(missing, ", ")}
	}
	if cfg.ReceiveMode == "webhook" {
		if cfg.Webhook == nil || strings.TrimSpace(fmt.Sprint(cfg.Webhook["path"])) == "" {
			return domain.ChannelTestResult{OK: false, Status: "pending_config", Message: "webhook.path is required for webhook channels"}
		}
	}
	if cfg.Type == "telegram" {
		return domain.ChannelTestResult{OK: true, Status: "ok", Message: "Telegram credentials are configured; live getMe is deferred until secret storage is connected"}
	}
	if supportsLightweightTest(cfg.Type) {
		return domain.ChannelTestResult{OK: true, Status: "ok", Message: "channel configuration is structurally valid"}
	}
	return domain.ChannelTestResult{OK: false, Status: "unsupported", Message: "live connection test is not implemented for this channel type yet"}
}

func requiredCredentials(channelType string) []string {
	switch channelType {
	case "telegram":
		return []string{"bot_token"}
	case "feishu":
		return []string{"app_secret"}
	case "whatsapp":
		return []string{"access_token"}
	case "slack":
		return []string{"bot_token", "signing_secret"}
	case "discord":
		return []string{"bot_token"}
	case "wechat", "openclaw-weixin":
		return []string{"app_secret"}
	case "wecom", "wecom-app":
		return []string{"corp_secret"}
	case "qq", "qqbot":
		return []string{"access_token"}
	default:
		return nil
	}
}

func missingCredentials(credentials []domain.ChannelCredential, required []string) []string {
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
	case "qq", "qqbot", "feishu", "wechat", "openclaw-weixin", "wecom", "wecom-app", "telegram", "whatsapp", "slack", "discord", "dingtalk":
		return true
	default:
		return false
	}
}

func credentialCount(credentials []domain.ChannelCredential) int {
	count := 0
	for _, item := range credentials {
		if strings.TrimSpace(item.SecretRef) != "" {
			count++
		}
	}
	return count
}

func errorMessageForTest(result domain.ChannelTestResult) string {
	if result.OK {
		return ""
	}
	return result.Message
}

func sanitizeCredentials(items []domain.ChannelCredential) []domain.ChannelCredential {
	out := make([]domain.ChannelCredential, len(items))
	for i, item := range items {
		item.Configured = strings.TrimSpace(item.SecretRef) != ""
		item.MaskedPreview = maskReference(item.SecretRef)
		item.SecretRef = ""
		out[i] = item
	}
	return out
}

func localSecretRef(channelID string, key string, secret string) string {
	sum := sha256.Sum256([]byte(channelID + ":" + key + ":" + secret))
	return "local:" + hex.EncodeToString(sum[:])
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

func catalogHasType(channelType string) bool {
	for _, item := range channelCatalog {
		if item.Type == channelType {
			return true
		}
	}
	return false
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

var channelCatalog = []domain.ChannelCatalogItem{
	channelCatalogItem("qq", "QQ (NapCat)", "国内", "websocket", "QQ 个人号，NapCat OneBot11 协议", 10, true),
	channelCatalogItem("qqbot", "QQ 官方机器人", "国内", "webhook", "QQ 开放平台机器人", 20, true),
	channelCatalogItem("feishu", "飞书 / Lark", "办公协作", "webhook", "事件订阅、机器人消息、多账号", 30, true),
	channelCatalogItem("dingtalk", "钉钉", "办公协作", "webhook", "钉钉机器人与事件回调", 40, true),
	channelCatalogItem("wecom", "企业微信智能机器人", "办公协作", "webhook", "群机器人或智能机器人", 50, true),
	channelCatalogItem("wecom-app", "企业微信自建应用", "办公协作", "webhook", "企业微信自建应用通道", 60, true),
	channelCatalogItem("openclaw-weixin", "微信（ClawBot）", "国内", "qrcode", "腾讯官方 WeChat ClawBot", 70, true),
	channelCatalogItem("wechat", "微信开放平台", "国内", "webhook", "公众号、小程序、企业微信", 80, true),
	channelCatalogItem("telegram", "Telegram", "海外", "webhook", "Bot API 接入", 90, true),
	channelCatalogItem("whatsapp", "WhatsApp", "海外", "webhook", "Meta Cloud API 或 BSP", 100, true),
	channelCatalogItem("facebook", "Facebook Messenger", "海外", "webhook", "Page Webhook 与 Messenger", 110, false),
	channelCatalogItem("discord", "Discord", "海外", "gateway", "Bot Gateway / Interaction", 120, true),
	channelCatalogItem("slack", "Slack", "办公协作", "event", "Slack App Events / Bot", 130, true),
	channelCatalogItem("msteams", "Microsoft Teams", "办公协作", "webhook", "Teams Bot / Graph", 140, false),
	channelCatalogItem("googlechat", "Google Chat", "办公协作", "webhook", "Google Chat App", 150, false),
	channelCatalogItem("line", "LINE", "海外", "webhook", "LINE Messaging API", 160, false),
	channelCatalogItem("matrix", "Matrix", "海外", "sync", "Matrix Bot", 170, false),
	channelCatalogItem("mattermost", "Mattermost", "办公协作", "websocket", "Mattermost Bot", 180, false),
	channelCatalogItem("signal", "Signal", "海外", "daemon", "Signal CLI/daemon", 190, false),
	channelCatalogItem("zalo", "Zalo", "海外", "webhook", "Zalo Official Account", 200, false),
	channelCatalogItem("zalouser", "Zalo User", "海外", "polling", "Zalo 用户侧通道", 210, false),
	channelCatalogItem("imessage", "iMessage", "海外", "bridge", "BlueBubbles / macOS 桥接", 220, false),
	channelCatalogItem("bluebubbles", "BlueBubbles", "海外", "bridge", "iMessage Android/Server 桥接", 230, false),
	channelCatalogItem("nextcloud-talk", "Nextcloud Talk", "办公协作", "polling", "Nextcloud Talk Bot", 240, false),
	channelCatalogItem("synology-chat", "Synology Chat", "办公协作", "webhook", "Synology Chat Bot", 250, false),
	channelCatalogItem("irc", "IRC", "海外", "socket", "IRC Bot", 260, false),
	channelCatalogItem("nostr", "Nostr", "海外", "relay", "Nostr relay 订阅", 270, false),
	channelCatalogItem("twitch", "Twitch", "海外", "eventsub", "Twitch Chat / EventSub", 280, false),
	channelCatalogItem("tlon", "Tlon", "海外", "plugin", "Tlon / Urbit 集成", 290, false),
	channelCatalogItem("voice-call", "Voice Call", "语音", "webhook", "Twilio / Telnyx / Plivo / mock", 300, false),
	channelCatalogItem("qa-channel", "QA Channel", "测试", "plugin", "QA 与回归测试通道", 310, true),
}

func channelCatalogItem(channelType, label, group, receiveMode, description string, sortOrder int, supportsTest bool) domain.ChannelCatalogItem {
	return domain.ChannelCatalogItem{
		Type:            channelType,
		Label:           label,
		Group:           group,
		Description:     description,
		ReceiveModes:    []string{receiveMode},
		Icon:            channelType,
		Bundled:         true,
		SupportsTest:    supportsTest,
		SupportsWebhook: receiveMode == "webhook" || receiveMode == "event" || receiveMode == "eventsub",
		ConfigSchema: map[string]any{
			"type":        "object",
			"description": "Non-sensitive channel configuration",
		},
		CredentialSchema: map[string]any{
			"type":     "object",
			"required": requiredCredentials(channelType),
		},
		UIHints: map[string]any{
			"group":        group,
			"receive_mode": receiveMode,
		},
		SortOrder: sortOrder,
	}
}

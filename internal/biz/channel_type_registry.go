package biz

import (
	"sort"
	"sync"
)

// CredentialProperty describes a single credential field for a channel type.
type CredentialProperty struct {
	Key      string
	Title    string
	Format   string
	Required bool
}

// ToMap converts a CredentialProperty to the map[string]any format used by credential schemas.
func (p CredentialProperty) ToMap() map[string]any {
	return propField(p.Title, p.Format, p.Required)
}

// ChannelTypeSpec describes a channel type's capabilities and requirements.
type ChannelTypeSpec struct {
	TypeItem            ChannelTypeItem
	RequiredCredentials []string
	CredentialProps     []CredentialProperty
	SupportsLightTest   bool
}

// channelTypeRegistry holds all registered channel types with their specs.
type channelTypeRegistry struct {
	mu    sync.RWMutex
	specs map[string]ChannelTypeSpec
	order []string // preserves insertion order for sorted output
}

// Register adds or updates a channel type in the registry.
func (r *channelTypeRegistry) Register(spec ChannelTypeSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Build CredentialSchema from spec data
	spec.TypeItem.CredentialSchema = buildCredentialSchema(spec)

	if r.specs == nil {
		r.specs = make(map[string]ChannelTypeSpec)
	}
	if _, exists := r.specs[spec.TypeItem.Type]; !exists {
		r.order = append(r.order, spec.TypeItem.Type)
	}
	r.specs[spec.TypeItem.Type] = spec
}

// Get looks up a channel type spec by type name.
func (r *channelTypeRegistry) Get(channelType string) (ChannelTypeSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[channelType]
	return spec, ok
}

// All returns all channel type specs sorted by SortOrder then Type.
func (r *channelTypeRegistry) All() []ChannelTypeSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]ChannelTypeSpec, 0, len(r.order))
	for _, name := range r.order {
		if spec, ok := r.specs[name]; ok {
			items = append(items, spec)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TypeItem.SortOrder != items[j].TypeItem.SortOrder {
			return items[i].TypeItem.SortOrder < items[j].TypeItem.SortOrder
		}
		return items[i].TypeItem.Type < items[j].TypeItem.Type
	})
	return items
}

// Types returns all registered channel type names in sorted order.
func (r *channelTypeRegistry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := r.All()
	names := make([]string, len(all))
	for i, spec := range all {
		names[i] = spec.TypeItem.Type
	}
	return names
}

// defaultRegistry is the global channel type registry.
var defaultRegistry channelTypeRegistry

// RegisterChannelType registers a channel type spec externally.
func RegisterChannelType(spec ChannelTypeSpec) {
	defaultRegistry.Register(spec)
}

// buildCredentialSchema constructs the CredentialSchema from spec data.
func buildCredentialSchema(spec ChannelTypeSpec) map[string]any {
	props := make(map[string]any, len(spec.CredentialProps))
	for _, p := range spec.CredentialProps {
		props[p.Key] = p.ToMap()
	}
	schema := map[string]any{
		"type":     "object",
		"required": spec.RequiredCredentials,
	}
	if len(props) > 0 {
		schema["properties"] = props
	}
	return schema
}

func init() {
	specs := []ChannelTypeSpec{
		{
			TypeItem:            feishuTypeItem(),
			RequiredCredentials: []string{"app_secret"},
			CredentialProps: []CredentialProperty{
				{Key: "app_secret", Title: "lark_app_secret", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            dingtalkTypeItem(),
			RequiredCredentials: []string{"secret"},
			CredentialProps: []CredentialProperty{
				{Key: "client_secret", Title: "ding_client_secret", Format: "password", Required: false},
				{Key: "secret", Title: "secret", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            wecomTypeItem("wecom", "企业微信智能机器人", "群机器人或智能机器人", 30),
			RequiredCredentials: []string{"token"},
			CredentialProps: []CredentialProperty{
				{Key: "token", Title: "com_wechat_token", Format: "password", Required: true},
				{Key: "encoding_aes_key", Title: "com_wechat_encoding_aes_key", Format: "password", Required: false},
				{Key: "corp_secret", Title: "com_wechat_secret", Format: "password", Required: false},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            wecomTypeItem("wecom-app", "企业微信自建应用", "企业微信自建应用", 40),
			RequiredCredentials: []string{"token"},
			CredentialProps: []CredentialProperty{
				{Key: "token", Title: "com_wechat_token", Format: "password", Required: true},
				{Key: "encoding_aes_key", Title: "com_wechat_encoding_aes_key", Format: "password", Required: false},
				{Key: "corp_secret", Title: "com_wechat_secret", Format: "password", Required: false},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            wechatTypeItem(),
			RequiredCredentials: []string{"app_secret"},
			CredentialProps: []CredentialProperty{
				{Key: "app_secret", Title: "wechat_app_secret", Format: "password", Required: true},
				{Key: "token", Title: "wechat_token", Format: "password", Required: false},
				{Key: "encoding_aes_key", Title: "wechat_encoding_aes_key", Format: "password", Required: false},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            slackTypeItem(),
			RequiredCredentials: []string{"bot_token", "signing_secret"},
			CredentialProps: []CredentialProperty{
				{Key: "bot_token", Title: "slack_bot_token", Format: "password", Required: true},
				{Key: "app_token", Title: "slack_app_token", Format: "password", Required: false},
				{Key: "signing_secret", Title: "signing_secret", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            telegramTypeItem(),
			RequiredCredentials: []string{"bot_token"},
			CredentialProps: []CredentialProperty{
				{Key: "bot_token", Title: "telegram_bot_token", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            discordTypeItem(),
			RequiredCredentials: []string{"bot_token"},
			CredentialProps: []CredentialProperty{
				{Key: "bot_token", Title: "discord_bot_token", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            lineTypeItem(),
			RequiredCredentials: []string{"channel_secret", "channel_token"},
			CredentialProps: []CredentialProperty{
				{Key: "channel_secret", Title: "line_channel_secret", Format: "password", Required: true},
				{Key: "channel_token", Title: "line_channel_access_token", Format: "password", Required: true},
			},
			SupportsLightTest: false,
		},
		{
			TypeItem:            mattermostTypeItem(),
			RequiredCredentials: []string{"server_url", "bot_token"},
			CredentialProps: []CredentialProperty{
				{Key: "server_url", Title: "mattermost_server_url", Format: "", Required: true},
				{Key: "bot_token", Title: "mattermost_bot_token", Format: "password", Required: true},
			},
			SupportsLightTest: false,
		},
		{
			TypeItem:            teamsTypeItem(),
			RequiredCredentials: []string{"app_id", "app_secret"},
			CredentialProps: []CredentialProperty{
				{Key: "app_id", Title: "teams_app_id", Format: "", Required: true},
				{Key: "app_secret", Title: "teams_app_secret", Format: "password", Required: true},
			},
			SupportsLightTest: false,
		},
		{
			TypeItem:            qqTypeItem(),
			RequiredCredentials: []string{"app_secret"},
			CredentialProps: []CredentialProperty{
				{Key: "app_secret", Title: "qq_app_secret", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
		{
			TypeItem:            personalQQTypeItem(),
			RequiredCredentials: []string{"receive_token", "send_token"},
			CredentialProps: []CredentialProperty{
				{Key: "receive_token", Title: "qq_one_bot_receive_token", Format: "password", Required: true},
				{Key: "send_token", Title: "qq_one_bot_send_token", Format: "password", Required: true},
			},
			SupportsLightTest: true,
		},
	}
	for _, spec := range specs {
		defaultRegistry.Register(spec)
	}
}

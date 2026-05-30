package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestDefaultJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "{}"},
		{"whitespace", "  ", "{}"},
		{"valid json", `{"key":"val"}`, `{"key":"val"}`},
		{"valid json with spaces", `  {"key":"val"}  `, `  {"key":"val"}  `},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.DefaultJSON(tc.input)
			if got != tc.want {
				t.Fatalf("DefaultJSON(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompactJSON(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{"empty uses fallback", "", `{"default":1}`, `{"default":1}`},
		{"whitespace uses fallback", "  ", `{"default":1}`, `{"default":1}`},
		{"valid json compacted", `{"a": 1, "b": 2}`, "{}", `{"a":1,"b":2}`},
		{"invalid json uses fallback", "not json", `{"fb":1}`, `{"fb":1}`},
		{"valid json no spaces", `{"x":1}`, "{}", `{"x":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.CompactJSON(tc.input, tc.fallback)
			if got != tc.want {
				t.Fatalf("CompactJSON(%q, %q) = %q, want %q", tc.input, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestParseChannelConfig(t *testing.T) {
	t.Run("empty string defaults to empty object", func(t *testing.T) {
		cfg, err := biz.ParseChannelConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "" {
			t.Fatalf("Type should be empty, got %q", cfg.Type)
		}
	})

	t.Run("valid config", func(t *testing.T) {
		raw := `{"type":"telegram","receive_mode":"webhook","config":{"bot_name":"test"}}`
		cfg, err := biz.ParseChannelConfig(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "telegram" {
			t.Fatalf("Type = %q, want telegram", cfg.Type)
		}
		if cfg.ReceiveMode != "webhook" {
			t.Fatalf("ReceiveMode = %q, want webhook", cfg.ReceiveMode)
		}
		if cfg.Config["bot_name"] != "test" {
			t.Fatalf("Config.bot_name = %v, want test", cfg.Config["bot_name"])
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		_, err := biz.ParseChannelConfig("not json")
		if err == nil {
			t.Fatalf("expected error for invalid json")
		}
	})

	t.Run("nil config becomes empty map", func(t *testing.T) {
		raw := `{"type":"telegram"}`
		cfg, err := biz.ParseChannelConfig(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Config == nil {
			t.Fatalf("Config should not be nil")
		}
		if len(cfg.Config) != 0 {
			t.Fatalf("Config should be empty map, got %v", cfg.Config)
		}
	})

	t.Run("type is trimmed", func(t *testing.T) {
		raw := `{"type":"  telegram  "}`
		cfg, err := biz.ParseChannelConfig(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "telegram" {
			t.Fatalf("Type = %q, want telegram", cfg.Type)
		}
	})
}

func TestNormalizeChannel(t *testing.T) {
	t.Run("missing key returns error", func(t *testing.T) {
		row := &biz.Channel{Name: "test", ConfigJSON: `{"type":"telegram"}`}
		err := biz.NormalizeChannel(row)
		if err == nil {
			t.Fatalf("expected error for missing key")
		}
	})

	t.Run("missing name returns error", func(t *testing.T) {
		row := &biz.Channel{Key: "test", ConfigJSON: `{"type":"telegram"}`}
		err := biz.NormalizeChannel(row)
		if err == nil {
			t.Fatalf("expected error for missing name")
		}
	})

	t.Run("missing type returns error", func(t *testing.T) {
		row := &biz.Channel{Key: "test", Name: "Test", ConfigJSON: `{}`}
		err := biz.NormalizeChannel(row)
		if err == nil {
			t.Fatalf("expected error for missing type")
		}
	})

	t.Run("unsupported type returns error", func(t *testing.T) {
		row := &biz.Channel{Key: "test", Name: "Test", ConfigJSON: `{"type":"unsupported_type_xyz"}`}
		err := biz.NormalizeChannel(row)
		if err == nil {
			t.Fatalf("expected error for unsupported type")
		}
	})

	t.Run("valid channel normalizes successfully", func(t *testing.T) {
		row := &biz.Channel{Key: "  test  ", Name: "  Test  ", ConfigJSON: `{"type":"telegram"}`}
		err := biz.NormalizeChannel(row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if row.Key != "test" {
			t.Fatalf("Key = %q, want test", row.Key)
		}
		if row.Name != "Test" {
			t.Fatalf("Name = %q, want Test", row.Name)
		}
		if row.Status != "active" {
			t.Fatalf("Status = %q, want active", row.Status)
		}
	})
}

func TestRequiredCredentials(t *testing.T) {
	cases := []struct {
		channelType string
		wantLen     int
	}{
		{"telegram", 1},
		{"feishu", 1},
		{"slack", 2},
		{"discord", 1},
		{"wechat", 1},
		{"dingtalk", 1},
		{"wecom", 1},
		{"wecom-app", 1},
		{"qq", 1},
		{"personal_qq", 2},
		{"line", 2},
		{"mattermost", 2},
		{"teams", 2},
		{"unknown", 0},
	}
	for _, tc := range cases {
		t.Run(tc.channelType, func(t *testing.T) {
			got := biz.RequiredCredentials(tc.channelType)
			if len(got) != tc.wantLen {
				t.Fatalf("RequiredCredentials(%q) = %d items, want %d", tc.channelType, len(got), tc.wantLen)
			}
		})
	}
}

func TestMissingCredentials(t *testing.T) {
	t.Run("all present", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "ref1"},
			{CredentialKey: "signing_secret", SecretRef: "ref2"},
		}
		required := []string{"bot_token", "signing_secret"}
		got := biz.MissingCredentials(creds, required)
		if len(got) != 0 {
			t.Fatalf("expected no missing, got %v", got)
		}
	})

	t.Run("some missing", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "ref1"},
		}
		required := []string{"bot_token", "signing_secret"}
		got := biz.MissingCredentials(creds, required)
		if len(got) != 1 || got[0] != "signing_secret" {
			t.Fatalf("expected [signing_secret], got %v", got)
		}
	})

	t.Run("empty secret ref is not available", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: ""},
		}
		required := []string{"bot_token"}
		got := biz.MissingCredentials(creds, required)
		if len(got) != 1 || got[0] != "bot_token" {
			t.Fatalf("expected [bot_token], got %v", got)
		}
	})

	t.Run("deleted credential is not available", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "ref1", DeletedAt: "2024-01-01"},
		}
		required := []string{"bot_token"}
		got := biz.MissingCredentials(creds, required)
		if len(got) != 1 {
			t.Fatalf("expected missing, got %v", got)
		}
	})

	t.Run("no required", func(t *testing.T) {
		got := biz.MissingCredentials(nil, nil)
		if len(got) != 0 {
			t.Fatalf("expected no missing, got %v", got)
		}
	})
}

func TestSupportsLightweightTest(t *testing.T) {
	lightweightTypes := []string{"qq", "personal_qq", "feishu", "wechat", "wecom", "wecom-app", "telegram", "slack", "discord", "dingtalk"}
	for _, ct := range lightweightTypes {
		t.Run(ct, func(t *testing.T) {
			if !biz.SupportsLightweightTest(ct) {
				t.Fatalf("SupportsLightweightTest(%q) should be true", ct)
			}
		})
	}

	t.Run("unsupported type", func(t *testing.T) {
		if biz.SupportsLightweightTest("unknown") {
			t.Fatalf("SupportsLightweightTest(unknown) should be false")
		}
	})
}

func TestCredentialCount(t *testing.T) {
	cases := []struct {
		name  string
		creds []biz.ChannelCredential
		want  int
	}{
		{"nil", nil, 0},
		{"empty", []biz.ChannelCredential{}, 0},
		{"with secret ref", []biz.ChannelCredential{
			{CredentialKey: "a", SecretRef: "ref1"},
			{CredentialKey: "b", SecretRef: "ref2"},
		}, 2},
		{"empty secret ref excluded", []biz.ChannelCredential{
			{CredentialKey: "a", SecretRef: "ref1"},
			{CredentialKey: "b", SecretRef: ""},
			{CredentialKey: "c", SecretRef: "  "},
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.CredentialCount(tc.creds)
			if got != tc.want {
				t.Fatalf("CredentialCount = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEvaluateChannelTest(t *testing.T) {
	t.Run("disabled channel", func(t *testing.T) {
		row := biz.Channel{Enabled: false}
		cfg := biz.ChannelConfig{Type: "telegram"}
		got := biz.EvaluateChannelTestInternal(row, cfg, nil)
		if got.OK || got.Status != "disabled" {
			t.Fatalf("expected disabled, got %+v", got)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{Type: "telegram"}
		got := biz.EvaluateChannelTestInternal(row, cfg, nil)
		if got.OK || got.Status != "pending_auth" {
			t.Fatalf("expected pending_auth, got %+v", got)
		}
	})

	t.Run("webhook missing path", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{Type: "telegram", ReceiveMode: "webhook"}
		creds := []biz.ChannelCredential{{CredentialKey: "bot_token", SecretRef: "ref1"}}
		got := biz.EvaluateChannelTestInternal(row, cfg, creds)
		if got.OK || got.Status != "pending_config" {
			t.Fatalf("expected pending_config, got %+v", got)
		}
	})

	t.Run("webhook with path", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{
			Type:        "telegram",
			ReceiveMode: "webhook",
			Webhook:     map[string]any{"path": "/webhooks/test"},
		}
		creds := []biz.ChannelCredential{{CredentialKey: "bot_token", SecretRef: "ref1"}}
		got := biz.EvaluateChannelTestInternal(row, cfg, creds)
		if !got.OK {
			t.Fatalf("expected OK, got %+v", got)
		}
	})

	t.Run("feishu missing app_id", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{Type: "feishu", Config: map[string]any{}}
		creds := []biz.ChannelCredential{{CredentialKey: "app_secret", SecretRef: "ref1"}}
		got := biz.EvaluateChannelTestInternal(row, cfg, creds)
		if got.OK || got.Status != "pending_config" {
			t.Fatalf("expected pending_config, got %+v", got)
		}
	})

	t.Run("feishu with app_id", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{
			Type:   "feishu",
			Config: map[string]any{"app_id": "cli_test"},
		}
		creds := []biz.ChannelCredential{{CredentialKey: "app_secret", SecretRef: "ref1"}}
		got := biz.EvaluateChannelTestInternal(row, cfg, creds)
		if !got.OK {
			t.Fatalf("expected OK, got %+v", got)
		}
	})

	t.Run("telegram special message", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{Type: "telegram"}
		creds := []biz.ChannelCredential{{CredentialKey: "bot_token", SecretRef: "ref1"}}
		got := biz.EvaluateChannelTestInternal(row, cfg, creds)
		if !got.OK {
			t.Fatalf("expected OK, got %+v", got)
		}
		if got.Status != "ok" {
			t.Fatalf("expected ok status, got %q", got.Status)
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		row := biz.Channel{Enabled: true}
		cfg := biz.ChannelConfig{Type: "unknown_type"}
		got := biz.EvaluateChannelTestInternal(row, cfg, nil)
		if got.OK || got.Status != "unsupported" {
			t.Fatalf("expected unsupported, got %+v", got)
		}
	})
}

func TestErrorMessageForTest(t *testing.T) {
	cases := []struct {
		name   string
		result biz.ChannelTestResult
		want   string
	}{
		{"ok result", biz.ChannelTestResult{OK: true, Message: "all good"}, ""},
		{"failed result", biz.ChannelTestResult{OK: false, Message: "something wrong"}, "something wrong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ErrorMessageForTest(tc.result)
			if got != tc.want {
				t.Fatalf("ErrorMessageForTest = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeCredentials(t *testing.T) {
	t.Run("masks and clears secret ref", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "abcdefgh12345678"},
			{CredentialKey: "app_secret", SecretRef: ""},
		}
		got := biz.SanitizeCredentials(creds)
		if got[0].SecretRef != "" {
			t.Fatalf("SecretRef should be cleared, got %q", got[0].SecretRef)
		}
		if !got[0].Configured {
			t.Fatalf("first should be Configured=true")
		}
		if got[0].MaskedPreview != "abcdefgh...5678" {
			t.Fatalf("MaskedPreview = %q, want abcdefgh...5678", got[0].MaskedPreview)
		}
		if got[1].Configured {
			t.Fatalf("second should be Configured=false")
		}
		if got[1].MaskedPreview != "" {
			t.Fatalf("MaskedPreview should be empty, got %q", got[1].MaskedPreview)
		}
	})
}

func TestMaskReference(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "  ", ""},
		{"short ref", "short", "********"},
		{"12 chars", "123456789012", "********"},
		{"13 chars", "1234567890123", "12345678...0123"},
		{"long ref", "abcdefghijklmnop", "abcdefgh...mnop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MaskReference(tc.ref)
			if got != tc.want {
				t.Fatalf("MaskReference(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestCatalogHasType(t *testing.T) {
	t.Run("known type", func(t *testing.T) {
		if !biz.CatalogHasType("telegram") {
			t.Fatalf("telegram should be in catalog")
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		if biz.CatalogHasType("nonexistent_channel") {
			t.Fatalf("nonexistent_channel should not be in catalog")
		}
	})
}

func TestCatalogSorted(t *testing.T) {
	items := biz.CatalogSorted()
	if len(items) == 0 {
		t.Fatalf("catalog should not be empty")
	}
	for i := 1; i < len(items); i++ {
		if items[i].SortOrder < items[i-1].SortOrder {
			t.Fatalf("catalog not sorted: items[%d].SortOrder=%d > items[%d].SortOrder=%d",
				i-1, items[i-1].SortOrder, i, items[i].SortOrder)
		}
	}
}

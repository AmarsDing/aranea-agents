package hook

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		input := `{"callback_point":"before_tool","condition":{"agent_id":"a1"},"action":{"type":"notify","webhook_url":"https://example.com"}}`
		cfg, err := ParseConfig(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CallbackPoint != "before_tool" {
			t.Errorf("CallbackPoint = %q, want %q", cfg.CallbackPoint, "before_tool")
		}
		if cfg.Condition.AgentID != "a1" {
			t.Errorf("Condition.AgentID = %q, want %q", cfg.Condition.AgentID, "a1")
		}
		if cfg.Action.Type != "notify" {
			t.Errorf("Action.Type = %q, want %q", cfg.Action.Type, "notify")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		cfg, err := ParseConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CallbackPoint != "" {
			t.Errorf("CallbackPoint = %q, want empty", cfg.CallbackPoint)
		}
	})

	t.Run("whitespace only", func(t *testing.T) {
		cfg, err := ParseConfig("   ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CallbackPoint != "" {
			t.Errorf("CallbackPoint = %q, want empty", cfg.CallbackPoint)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := ParseConfig("{not json}")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("normalizes callback_point", func(t *testing.T) {
		input := `{"callback_point":"BeforeTool"}`
		cfg, err := ParseConfig(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CallbackPoint != "before_tool" {
			t.Errorf("CallbackPoint = %q, want %q", cfg.CallbackPoint, "before_tool")
		}
	})
}

func TestNormalizeCallbackPoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"before_agent", "before_agent"},
		{"BeforeAgent", "before_agent"},
		{"beforeagent", "before_agent"},
		{"after_agent", "after_agent"},
		{"AfterAgent", "after_agent"},
		{"afteragent", "after_agent"},
		{"before_model", "before_model"},
		{"BeforeModel", "before_model"},
		{"beforemodel", "before_model"},
		{"after_model", "after_model"},
		{"AfterModel", "after_model"},
		{"aftermodel", "after_model"},
		{"before_tool", "before_tool"},
		{"BeforeTool", "before_tool"},
		{"beforetool", "before_tool"},
		{"after_tool", "after_tool"},
		{"AfterTool", "after_tool"},
		{"aftertool", "after_tool"},
		{"on_event", "on_event"},
		{"OnEvent", "on_event"},
		{"onevent", "on_event"},
		{"unknown_point", "unknown_point"},
		{"", ""},
		{"  before_tool  ", "before_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeCallbackPoint(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeCallbackPoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAppliesToAgent(t *testing.T) {
	tests := []struct {
		name     string
		cond     Condition
		agentID  string
		agentKey string
		want     bool
	}{
		{"empty condition matches all", Condition{}, "any-id", "any-key", true},
		{"matching agentID", Condition{AgentID: "agent-1"}, "agent-1", "other-key", true},
		{"matching agentKey", Condition{AgentID: "my-key"}, "other-id", "my-key", true},
		{"non-matching", Condition{AgentID: "agent-1"}, "agent-2", "key-2", false},
		{"whitespace agentID match", Condition{AgentID: "  agent-1  "}, "agent-1", "", true},
		{"whitespace agentKey match", Condition{AgentID: "  my-key  "}, "", "my-key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppliesToAgent(tt.cond, tt.agentID, tt.agentKey)
			if got != tt.want {
				t.Errorf("AppliesToAgent(%+v, %q, %q) = %v, want %v", tt.cond, tt.agentID, tt.agentKey, got, tt.want)
			}
		})
	}
}

func TestAppliesToTool(t *testing.T) {
	tests := []struct {
		name     string
		cond     Condition
		toolName string
		want     bool
	}{
		{"empty condition matches all", Condition{}, "any-tool", true},
		{"matching toolName", Condition{ToolName: "search"}, "search", true},
		{"non-matching", Condition{ToolName: "search"}, "code_exec", false},
		{"whitespace match", Condition{ToolName: "  search  "}, "search", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppliesToTool(tt.cond, tt.toolName)
			if got != tt.want {
				t.Errorf("AppliesToTool(%+v, %q) = %v, want %v", tt.cond, tt.toolName, got, tt.want)
			}
		})
	}
}

func TestParseNotifyOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		opts := ParseNotifyOptions(Action{})
		if opts.MaxAttempts != 3 {
			t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
		}
		if opts.TimeoutSec != 8 {
			t.Errorf("TimeoutSec = %d, want 8", opts.TimeoutSec)
		}
		if opts.WebhookSecret != "" {
			t.Errorf("WebhookSecret = %q, want empty", opts.WebhookSecret)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		opts := ParseNotifyOptions(Action{
			NotifyMaxRetries: 5,
			NotifyTimeoutSec: 30,
			WebhookSecret:    "  s3cret  ",
		})
		if opts.MaxAttempts != 5 {
			t.Errorf("MaxAttempts = %d, want 5", opts.MaxAttempts)
		}
		if opts.TimeoutSec != 30 {
			t.Errorf("TimeoutSec = %d, want 30", opts.TimeoutSec)
		}
		if opts.WebhookSecret != "s3cret" {
			t.Errorf("WebhookSecret = %q, want %q", opts.WebhookSecret, "s3cret")
		}
	})

	t.Run("zero values use defaults", func(t *testing.T) {
		opts := ParseNotifyOptions(Action{
			NotifyMaxRetries: 0,
			NotifyTimeoutSec: 0,
		})
		if opts.MaxAttempts != 3 {
			t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
		}
		if opts.TimeoutSec != 8 {
			t.Errorf("TimeoutSec = %d, want 8", opts.TimeoutSec)
		}
	})

	t.Run("empty action", func(t *testing.T) {
		opts := ParseNotifyOptions(Action{})
		if opts.MaxAttempts != 3 {
			t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
		}
		if opts.TimeoutSec != 8 {
			t.Errorf("TimeoutSec = %d, want 8", opts.TimeoutSec)
		}
	})
}

func TestNormalizeDeliveryStatus(t *testing.T) {
	tests := []struct {
		input string
		want  DeliveryStatus
	}{
		{"pending", DeliveryPending},
		{"success", DeliverySuccess},
		{"ok", DeliverySuccess},
		{"failed", DeliveryFailed},
		{"error", DeliveryFailed},
		{"unknown", DeliveryPending},
		{"SUCCESS", DeliverySuccess},
		{"  Failed  ", DeliveryFailed},
		{"", DeliveryPending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeDeliveryStatus(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeDeliveryStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeliveryIdempotencyKey(t *testing.T) {
	t.Run("same input produces same key", func(t *testing.T) {
		payload := map[string]any{
			"event_type": "after_tool",
			"run_id":     "run-1",
			"session_id": "sess-1",
		}
		key1 := DeliveryIdempotencyKey("hook-1", payload)
		key2 := DeliveryIdempotencyKey("hook-1", payload)
		if key1 != key2 {
			t.Errorf("same input produced different keys: %q vs %q", key1, key2)
		}
	})

	t.Run("different input produces different key", func(t *testing.T) {
		payload1 := map[string]any{
			"event_type": "after_tool",
			"run_id":     "run-1",
			"session_id": "sess-1",
		}
		payload2 := map[string]any{
			"event_type": "after_tool",
			"run_id":     "run-2",
			"session_id": "sess-1",
		}
		key1 := DeliveryIdempotencyKey("hook-1", payload1)
		key2 := DeliveryIdempotencyKey("hook-1", payload2)
		if key1 == key2 {
			t.Errorf("different input produced same key: %q", key1)
		}
	})

	t.Run("different hookID produces different key", func(t *testing.T) {
		payload := map[string]any{
			"event_type": "after_tool",
			"run_id":     "run-1",
		}
		key1 := DeliveryIdempotencyKey("hook-1", payload)
		key2 := DeliveryIdempotencyKey("hook-2", payload)
		if key1 == key2 {
			t.Errorf("different hookID produced same key: %q", key1)
		}
	})

	t.Run("key length is 32", func(t *testing.T) {
		key := DeliveryIdempotencyKey("hook-1", map[string]any{})
		if len(key) != 32 {
			t.Errorf("key length = %d, want 32", len(key))
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		key := DeliveryIdempotencyKey("hook-1", map[string]any{})
		if key == "" {
			t.Error("key is empty, want non-empty")
		}
	})
}

func TestHook_CallbackPoint(t *testing.T) {
	t.Run("valid config JSON", func(t *testing.T) {
		h := Hook{ConfigJSON: `{"callback_point":"after_tool"}`}
		got := h.CallbackPoint()
		if got != "after_tool" {
			t.Errorf("CallbackPoint() = %q, want %q", got, "after_tool")
		}
	})

	t.Run("empty config JSON", func(t *testing.T) {
		h := Hook{ConfigJSON: ""}
		got := h.CallbackPoint()
		if got != "" {
			t.Errorf("CallbackPoint() = %q, want empty", got)
		}
	})

	t.Run("invalid config JSON", func(t *testing.T) {
		h := Hook{ConfigJSON: "not-json"}
		got := h.CallbackPoint()
		if got != "" {
			t.Errorf("CallbackPoint() = %q, want empty on parse error", got)
		}
	})

	t.Run("normalizes callback_point alias", func(t *testing.T) {
		h := Hook{ConfigJSON: `{"callback_point":"BeforeTool"}`}
		got := h.CallbackPoint()
		if got != "before_tool" {
			t.Errorf("CallbackPoint() = %q, want %q", got, "before_tool")
		}
	})
}

func TestHook_ConditionFromConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		h := Hook{ConfigJSON: `{"callback_point":"before_tool","condition":{"agent_id":"a1","tool_name":"search"}}`}
		cond := h.ConditionFromConfig()
		if cond.AgentID != "a1" {
			t.Errorf("AgentID = %q, want %q", cond.AgentID, "a1")
		}
		if cond.ToolName != "search" {
			t.Errorf("ToolName = %q, want %q", cond.ToolName, "search")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		h := Hook{ConfigJSON: ""}
		cond := h.ConditionFromConfig()
		if cond.AgentID != "" || cond.ToolName != "" || cond.EventType != "" {
			t.Errorf("ConditionFromConfig() = %+v, want zero Condition", cond)
		}
	})

	t.Run("invalid config JSON", func(t *testing.T) {
		h := Hook{ConfigJSON: "bad"}
		cond := h.ConditionFromConfig()
		if cond.AgentID != "" || cond.ToolName != "" || cond.EventType != "" {
			t.Errorf("ConditionFromConfig() = %+v, want zero Condition on parse error", cond)
		}
	})
}

func TestHook_ActionFromConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := Config{
			Action: Action{
				Type:             "notify",
				WebhookURL:       "https://example.com/hook",
				WebhookSecret:    "secret",
				NotifyMaxRetries: 5,
				NotifyTimeoutSec: 20,
			},
		}
		raw, _ := json.Marshal(cfg)
		h := Hook{ConfigJSON: string(raw)}
		act := h.ActionFromConfig()
		if act.Type != "notify" {
			t.Errorf("Type = %q, want %q", act.Type, "notify")
		}
		if act.WebhookURL != "https://example.com/hook" {
			t.Errorf("WebhookURL = %q, want %q", act.WebhookURL, "https://example.com/hook")
		}
		if act.WebhookSecret != "secret" {
			t.Errorf("WebhookSecret = %q, want %q", act.WebhookSecret, "secret")
		}
		if act.NotifyMaxRetries != 5 {
			t.Errorf("NotifyMaxRetries = %d, want 5", act.NotifyMaxRetries)
		}
		if act.NotifyTimeoutSec != 20 {
			t.Errorf("NotifyTimeoutSec = %d, want 20", act.NotifyTimeoutSec)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		h := Hook{ConfigJSON: ""}
		act := h.ActionFromConfig()
		if act.Type != "" {
			t.Errorf("Type = %q, want empty", act.Type)
		}
		if act.WebhookURL != "" {
			t.Errorf("WebhookURL = %q, want empty", act.WebhookURL)
		}
	})

	t.Run("invalid config JSON", func(t *testing.T) {
		h := Hook{ConfigJSON: "not-json"}
		act := h.ActionFromConfig()
		if act.Type != "" {
			t.Errorf("Type = %q, want empty on parse error", act.Type)
		}
	})

	t.Run("config with modify_patch", func(t *testing.T) {
		h := Hook{ConfigJSON: `{"action":{"type":"modify","modify_patch":{"key":"value"}}}`}
		act := h.ActionFromConfig()
		if act.Type != "modify" {
			t.Errorf("Type = %q, want %q", act.Type, "modify")
		}
		if act.ModifyPatch == nil {
			t.Fatal("ModifyPatch is nil, want non-nil")
		}
		if v, ok := act.ModifyPatch["key"].(string); !ok || v != "value" {
			t.Errorf("ModifyPatch[\"key\"] = %v, want %q", act.ModifyPatch["key"], "value")
		}
	})
}

func TestNormalizeCallbackPoint_AllAliases(t *testing.T) {
	aliases := []struct {
		input string
		want  string
	}{
		{"before_agent", "before_agent"},
		{"beforeagent", "before_agent"},
		{"after_agent", "after_agent"},
		{"afteragent", "after_agent"},
		{"before_model", "before_model"},
		{"beforemodel", "before_model"},
		{"after_model", "after_model"},
		{"aftermodel", "after_model"},
		{"before_tool", "before_tool"},
		{"beforetool", "before_tool"},
		{"after_tool", "after_tool"},
		{"aftertool", "after_tool"},
		{"on_event", "on_event"},
		{"onevent", "on_event"},
	}
	for _, tt := range aliases {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeCallbackPoint(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeCallbackPoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !strings.Contains(got, "_") && got != "" {
				t.Errorf("NormalizeCallbackPoint(%q) = %q, expected underscore in canonical form", tt.input, got)
			}
		})
	}
}

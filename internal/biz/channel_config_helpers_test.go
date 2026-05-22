package biz

import "testing"

func TestParseChannelLongTaskConfig_defaults(t *testing.T) {
	cfg := ParseChannelLongTaskConfig(`{"config":{}}`)
	if cfg.AckMessage != defaultChannelAckMessage {
		t.Fatalf("AckMessage=%q want default", cfg.AckMessage)
	}
	if cfg.AckOnQueued != defaultChannelAckOnQueued {
		t.Fatalf("AckOnQueued=%q want default", cfg.AckOnQueued)
	}
	if cfg.TurnTimeoutSec != 0 || cfg.FirstByteTimeoutSec != 0 {
		t.Fatalf("timeouts should be 0 for service default")
	}
	if cfg.ProgressMode != "off" || cfg.ExecutionMode != "sync" {
		t.Fatalf("ProgressMode=%q ExecutionMode=%q", cfg.ProgressMode, cfg.ExecutionMode)
	}
}

func TestParseChannelLongTaskConfig_overrides(t *testing.T) {
	raw := `{"config":{
		"ack_message":"ok",
		"ack_on_queued":"wait",
		"turn_timeout_sec":900,
		"first_byte_timeout_sec":120,
		"progress_mode":"text",
		"progress_quiet_sec":15,
		"execution_mode":"async"
	}}`
	cfg := ParseChannelLongTaskConfig(raw)
	if cfg.AckMessage != "ok" || cfg.AckOnQueued != "wait" {
		t.Fatalf("ack overrides")
	}
	if cfg.TurnTimeoutSec != 900 || cfg.FirstByteTimeoutSec != 120 {
		t.Fatalf("timeout overrides")
	}
	if cfg.ProgressMode != "text" || cfg.ProgressQuietSec != 15 {
		t.Fatalf("progress overrides")
	}
	if cfg.ExecutionMode != "async" {
		t.Fatalf("execution_mode")
	}
}

func TestRenderChannelTemplate(t *testing.T) {
	got := RenderChannelTemplate("id={{pending_id}}", map[string]string{"pending_id": "p1"})
	if got != "id=p1" {
		t.Fatalf("got %q", got)
	}
}

func TestChannelSupportsLongTaskIngress(t *testing.T) {
	if !ChannelSupportsLongTaskIngress("feishu", `{}`) {
		t.Fatal("feishu should support long task")
	}
	if ChannelSupportsLongTaskIngress("wechat", `{"config":{}}`) {
		t.Fatal("wechat passive should not support long task ingress")
	}
	if !ChannelSupportsLongTaskIngress("wechat", `{"config":{"active_mode":true}}`) {
		t.Fatal("wechat active should support long task ingress")
	}
}

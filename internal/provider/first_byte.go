package provider

import (
	"encoding/json"
	"strings"
	"time"
)

// FirstByteTimeoutFromConfigJSON reads first_byte_timeout_sec from a
// provider-model ConfigJSON blob. Thinking packs overlay 60–90s here;
// 0 / missing / invalid means callers should use the product 30s default.
func FirstByteTimeoutFromConfigJSON(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return 0
	}
	var c struct {
		FirstByteTimeoutSec int `json:"first_byte_timeout_sec"`
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil || c.FirstByteTimeoutSec <= 0 {
		return 0
	}
	return time.Duration(c.FirstByteTimeoutSec) * time.Second
}

// 活性守卫包默认值（2026-09-01 治理）：首字节后事件间隔预算与重连上限。
// 模型目录 config_json 的 stall_timeout_sec / stall_max_attempts 可覆盖。
const (
	// DefaultStallTimeout 首字节后允许的最大事件间隔静默（90s）。
	// 远超实测 thinking 静默（首字节 stall 实证 30s 级）；长思考模型经
	// config_json 调大。
	DefaultStallTimeout = 90 * time.Second
	// DefaultStallMaxReconnects 单次 GenerateContent 允许的最大重连次数
	// （5 次；总尝试 = 1 首发 + 5 重连）。穷尽后流尾产出终态错误响应。
	DefaultStallMaxReconnects = 5
)

// StallPolicyFromConfigJSON reads stall_timeout_sec / stall_max_attempts
// from a provider-model ConfigJSON blob. 0 / missing / invalid fields mean
// callers should fall back to the package defaults above.
func StallPolicyFromConfigJSON(raw string) (stallTimeout time.Duration, maxReconnects int) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return 0, 0
	}
	var c struct {
		StallTimeoutSec  int `json:"stall_timeout_sec"`
		StallMaxAttempts int `json:"stall_max_attempts"`
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return 0, 0
	}
	if c.StallTimeoutSec > 0 {
		stallTimeout = time.Duration(c.StallTimeoutSec) * time.Second
	}
	if c.StallMaxAttempts > 0 {
		maxReconnects = c.StallMaxAttempts
	}
	return stallTimeout, maxReconnects
}

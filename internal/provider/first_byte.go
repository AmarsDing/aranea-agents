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

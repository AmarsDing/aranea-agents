package data

import "testing"

// D3 user 域召回 PII 门（spec §6.3 过渡期保守口径）：仅 user 域且
// pii_flag=true 的命中被丢弃；其它域与 pii_flag=false/缺失不受影响。
func TestSkipUserScopePIIHit(t *testing.T) {
	cases := []struct {
		name      string
		scopeType string
		raw       string
		want      bool
	}{
		{"user+pii→跳过", "user", `{"id":"f1","pii_flag":true}`, true},
		{"user+无pii→保留", "user", `{"id":"f1","pii_flag":false}`, false},
		{"user+pii缺失→保留", "user", `{"id":"f1"}`, false},
		{"agent+pii→保留（门只管user域）", "agent", `{"id":"f1","pii_flag":true}`, false},
		{"workspace+pii→保留", "workspace", `{"id":"f1","pii_flag":true}`, false},
		{"user+大小写混合→跳过", "User", `{"id":"f1","pii_flag":true}`, true},
		{"user+坏JSON→保守保留", "user", `{not-json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := skipUserScopePIIHit(c.scopeType, []byte(c.raw)); got != c.want {
				t.Fatalf("skipUserScopePIIHit(%q, %s) = %v, want %v", c.scopeType, c.raw, got, c.want)
			}
		})
	}
}

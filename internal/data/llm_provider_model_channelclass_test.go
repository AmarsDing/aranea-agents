package data

// D6 L-1 channel_class 载体（metadata_json 键）纯函数测试。
// 拍板（W4 登记）：Ent 实列迁移被 peer 在制品阻断，V2 以 metadata_json
// 键承载；peer 收口后切 Ent 实列，本测试的语义断言（local 命中/空=cloud）
// 对两种载体必须保持一致。

import (
	"testing"
)

func TestProviderModelChannelClass_Extraction(t *testing.T) {
	cases := []struct {
		name string
		meta string
		want string
	}{
		{"explicit local", `{"channel_class":"local"}`, "local"},
		{"explicit cloud", `{"channel_class":"cloud"}`, "cloud"},
		{"case-insensitive local", `{"channel_class":"LOCAL"}`, "local"},
		{"mixed keys preserved", `{"region":"cn","channel_class":"local"}`, "local"},
		{"missing key = cloud", `{"region":"cn"}`, "cloud"},
		{"empty metadata = cloud", ``, "cloud"},
		{"empty object = cloud", `{}`, "cloud"},
		{"garbage json = cloud", `{not-json`, "cloud"},
		{"non-string value = cloud", `{"channel_class":17}`, "cloud"},
		{"null value = cloud", `{"channel_class":null}`, "cloud"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerModelChannelClass(tc.meta); got != tc.want {
				t.Fatalf("providerModelChannelClass(%q) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}

func TestWithChannelClassMetadata_Merge(t *testing.T) {
	// local 写入键且保留既有键。
	got := withChannelClassMetadata(`{"region":"cn"}`, "local")
	wantContains := []string{`"region":"cn"`, `"channel_class":"local"`}
	for _, w := range wantContains {
		if !contains(got, w) {
			t.Fatalf("withChannelClassMetadata local: %q missing %q", got, w)
		}
	}

	// 空 metadata + local → 仅键。
	got = withChannelClassMetadata("", "LOCAL") // 大小写不敏感
	if !contains(got, `"channel_class":"local"`) {
		t.Fatalf("withChannelClassMetadata empty+local: %q", got)
	}

	// cloud/空 类 → 移除键（空=cloud 规范形态），其余键保留。
	got = withChannelClassMetadata(`{"region":"cn","channel_class":"local"}`, "cloud")
	if contains(got, "channel_class") {
		t.Fatalf("withChannelClassMetadata cloud must drop key: %q", got)
	}
	if !contains(got, `"region":"cn"`) {
		t.Fatalf("withChannelClassMetadata cloud must keep other keys: %q", got)
	}

	// 空类别 = cloud 语义（显式回 cloud 的规范写法就是不带键）。
	got = withChannelClassMetadata(`{"channel_class":"local"}`, "")
	if contains(got, "channel_class") {
		t.Fatalf("withChannelClassMetadata empty class must drop key: %q", got)
	}

	// 脏 JSON 输入：local 写入 → 重建合法 JSON（丢失原内容，可接受——
	// 原内容本就不可解析）；cloud 写入 → 原样回退（无键可删）。
	got = withChannelClassMetadata(`{bad`, "local")
	if !contains(got, `"channel_class":"local"`) {
		t.Fatalf("withChannelClassMetadata garbage+local: %q", got)
	}
	if got := withChannelClassMetadata(`{bad`, "cloud"); got != `{bad` {
		t.Fatalf("withChannelClassMetadata garbage+cloud must fallback verbatim: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

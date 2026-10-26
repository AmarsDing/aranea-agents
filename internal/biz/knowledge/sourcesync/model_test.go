package sourcesync

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKindsByteStable kind 枚举字节稳定钉：值集合与设计 ③-1 CHECK 两值
// 字节级一致；Valid/ParseKind 严格（大小写漂移、空串、未知值一律拒）。
func TestKindsByteStable(t *testing.T) {
	t.Parallel()
	got := AllKinds()
	want := []Kind{KindSharedDir, KindNotion}
	if len(got) != len(want) {
		t.Fatalf("AllKinds len=%d want %d —— 值集合变更必须同批迁移 CHECK + 钉阵", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("AllKinds[%d]=%q want %q", i, got[i], w)
		}
		if KindSharedDir != "shared_dir" || KindNotion != "notion" {
			t.Fatalf("kind literal drift: %q %q", KindSharedDir, KindNotion)
		}
	}
	if _, err := ParseKind("Notion"); err == nil {
		t.Fatal("ParseKind must reject case drift")
	}
	if _, err := ParseKind(""); err == nil {
		t.Fatal("ParseKind must reject empty")
	}
	if _, err := ParseKind("web"); err == nil {
		t.Fatal("ParseKind must reject unknown kind (web connector 为 G-38 二期欠账)")
	}
	k, err := ParseKind("notion")
	if err != nil || k != KindNotion {
		t.Fatalf("ParseKind(notion)=(%q,%v)", k, err)
	}
}

// TestSyncStatesByteStable 状态机四值钉（③-1：idle|running|auth_paused|error）。
func TestSyncStatesByteStable(t *testing.T) {
	t.Parallel()
	want := []SyncState{SyncStateIdle, SyncStateRunning, SyncStateAuthPaused, SyncStateError}
	got := AllSyncStates()
	if len(got) != len(want) {
		t.Fatalf("sync_state set drift: %v", got)
	}
	for i := range want {
		if got[i] != want[i] || !got[i].Valid() {
			t.Fatalf("sync_state[%d]=%q", i, got[i])
		}
	}
	if SyncStateAuthPaused != "auth_paused" {
		t.Fatalf("auth_paused literal drift: %q", SyncStateAuthPaused)
	}
	if SyncState("Paused").Valid() {
		t.Fatal("case-drift state must be invalid")
	}
}

// TestItemRunStatusesByteStable 指纹行/轮次状态集钉（③-2/③-4/⑦ items 视图栅）。
func TestItemRunStatusesByteStable(t *testing.T) {
	t.Parallel()
	if got := AllItemStatuses(); len(got) != 3 ||
		got[0] != ItemStatusActive || got[1] != ItemStatusMissing || got[2] != ItemStatusDeleted {
		t.Fatalf("item_status set drift: %v", got)
	}
	if ItemStatusMissing != "missing" || ItemStatusDeleted != "deleted" {
		t.Fatalf("item literal drift: %q %q", ItemStatusMissing, ItemStatusDeleted)
	}
	if got := AllRunStatuses(); len(got) != 4 ||
		got[0] != RunStatusSuccess || got[1] != RunStatusPartial ||
		got[2] != RunStatusFailed || got[3] != RunStatusSkipped {
		t.Fatalf("run_status set drift: %v", got)
	}
	if RunStatusSkipped != "skipped" {
		t.Fatalf("skipped literal drift: %q", RunStatusSkipped)
	}
}

// TestDeletionVerdictStable 三态钉（⑥-5/P5 a）：Unknown 是零值安全默认，
// 数值/字符串映射字节稳定（Unknown 保留 missing 不降级删除）。
func TestDeletionVerdictStable(t *testing.T) {
	t.Parallel()
	if VerdictUnknown != 0 {
		t.Fatalf("unknown must stay zero default: %d", VerdictUnknown)
	}
	cases := map[DeletionVerdict]string{
		VerdictUnknown:     "unknown",
		VerdictConfirmed:   "confirmed",
		VerdictStillExists: "still_exists",
	}
	for v, s := range cases {
		if !v.Valid() || v.String() != s {
			t.Fatalf("verdict %d string=%q want %q", int(v), v.String(), s)
		}
	}
	if DeletionVerdict(9).Valid() {
		t.Fatal("out-of-range verdict must be invalid")
	}
}

// TestErrorCodesByteStable ③-1 错误码纯函数钉（核心钉之一）：遍历全集断言
// 蛇形小写形状——无路径/（斜杠）、无 hash（十六进制长串）、无页 ID
// （数字段起手）、无模板花括等动态片段；ByteStable 对集外值一律拒。
func TestErrorCodesByteStable(t *testing.T) {
	t.Parallel()
	// 值集合黄金表——任何增删此处必须同改（65 手册联动）。
	golden := []ErrorCode{
		"auth_failed",
		"credential_decrypt_failed",
		"credential_missing",
		"walk_failed",
		"fetch_failed",
		"confirm_failed",
		"walk_empty_abort",
	}
	got := AllErrorCodes()
	if len(got) != len(golden) {
		t.Fatalf("error_code set drift: %v (golden %v)", got, golden)
	}
	seen := map[ErrorCode]bool{}
	for i, c := range got {
		if c != golden[i] {
			t.Fatalf("error_code[%d]=%q want %q", i, c, golden[i])
		}
		if seen[c] {
			t.Fatalf("duplicate error_code %q", c)
		}
		seen[c] = true
		if !c.ByteStable() {
			t.Fatalf("declared code %q failed shape guard", c)
		}
		s := string(c)
		if strings.ContainsAny(s, "/{}%#@.=-0123456789") && !errorCodeShape.MatchString(s) {
			t.Fatalf("code %q smuggles dynamic fragment", s)
		}
		// 路径/页 ID/hash 动态片段专项拒。
		for _, frag := range []string{"/", "\\", "{", "}", ":", ".", "-"} {
			if strings.Contains(s, frag) {
				t.Fatalf("code %q contains dynamic-fragment rune %q", s, frag)
			}
		}
	}
	if ErrCodeNone.ByteStable() != true {
		t.Fatal("empty code must be byte-stable (缺省合法)")
	}
	for _, bad := range []string{
		"AUTH_FAILED", "auth/failed", "auth_failed_001page", "folders/abc",
		"fetch_failed:{doc}", "walk failed", "a", "x_y", "9lives",
	} {
		if ErrorCode(bad).ByteStable() {
			t.Fatalf("out-of-set code %q must be rejected", bad)
		}
	}
}

// TestBackoffTable 退避表钉（③-1：2^n 分钟 cap 60，负数钳 0）。
func TestBackoffTable(t *testing.T) {
	t.Parallel()
	want := map[int]int{-1: 1, 0: 1, 1: 2, 2: 4, 3: 8, 4: 16, 5: 32, 6: 60, 7: 60, 30: 60}
	for f, m := range want {
		if got := BackoffMinutes(f); got != m {
			t.Fatalf("BackoffMinutes(%d)=%d want %d", f, got, m)
		}
		if d := BackoffDuration(f); d != time.Duration(m)*time.Minute {
			t.Fatalf("BackoffDuration(%d)=%v want %dm", f, d, m)
		}
	}
}

// TestTruncateErrorMessage 512 rune 截断钉（UTF-8 安全：多字节不截半）。
func TestTruncateErrorMessage(t *testing.T) {
	t.Parallel()
	short := "ok"
	if got := TruncateErrorMessage(short); got != short {
		t.Fatalf("short string altered: %q", got)
	}
	long := strings.Repeat("知", 600)
	got := TruncateErrorMessage(long)
	if n := len([]rune(got)); n != 512 {
		t.Fatalf("truncate runes=%d want 512", n)
	}
	exact := strings.Repeat("x", 512)
	if TruncateErrorMessage(exact) != exact {
		t.Fatal("exact 512 must pass through")
	}
}

// TestValidateConfigGate 校验闸钉（⑤ 接入面与 W5 HTTP 层共用 biz 唯一处）：
// interval<900 拒 / 三预算非正拒 / 空名空库拒 / 集外 error_code 拒。
func TestValidateConfigGate(t *testing.T) {
	t.Parallel()
	base := Connector{
		Kind:                KindSharedDir,
		Name:                "team-share",
		CollectionID:        "c1",
		IntervalSeconds:     MinIntervalSeconds,
		MaxItemsPerRun:      DefaultMaxItemsPerRun,
		MaxDeletesPerRun:    DefaultMaxDeletesPerRun,
		MaxFetchBytesPerRun: DefaultMaxFetchBytesPerRun,
	}
	if err := ValidateConfig(base); err != nil {
		t.Fatalf("base config rejected: %v", err)
	}
	c := base
	c.IntervalSeconds = 899
	if err := ValidateConfig(c); err == nil {
		t.Fatal("interval below 900 must be rejected")
	}
	c = base
	c.MaxDeletesPerRun = 0
	if err := ValidateConfig(c); err == nil {
		t.Fatal("zero delete budget must be rejected")
	}
	c = base
	c.Name = "  "
	if err := ValidateConfig(c); err == nil {
		t.Fatal("blank name must be rejected")
	}
	c = base
	c.Kind = "WEB"
	if err := ValidateConfig(c); err == nil {
		t.Fatal("invalid kind must be rejected")
	}
	c = base
	c.ErrorCode = "nope:{dynamic}"
	if err := ValidateConfig(c); err == nil {
		t.Fatal("non-byte-stable error_code must be rejected")
	}
	c = base
	c.ErrorCode = ErrCodeWalkFailed
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("stable error_code rejected: %v", err)
	}
}

// TestRetentionConstants P10 拍板常量钉死（RunKeep=100 / ItemRetention=90 天）。
func TestRetentionConstants(t *testing.T) {
	t.Parallel()
	if RunKeep != 100 {
		t.Fatalf("RunKeep drift: %d", RunKeep)
	}
	if ItemRetention != 90*24*time.Hour {
		t.Fatalf("ItemRetention drift: %v", ItemRetention)
	}
	if DefaultMaxItemsPerRun != 500 || DefaultMaxDeletesPerRun != 200 || DefaultMaxFetchBytesPerRun != 512<<20 {
		t.Fatal("budget defaults drift")
	}
	if MinIntervalSeconds != 900 {
		t.Fatalf("MinIntervalSeconds drift: %d", MinIntervalSeconds)
	}
}

// ──────────────────────────────── W3 分 kind 配置解析 ────────────────────────────────

// TestParseSharedDirConfig 闸群钉（W3）：合法 root 提取 + TrimSpace（绝对路径
// 用 t.TempDir——POSIX 字面量 /data 在 Windows 非绝对，跨平台钉必须取真 abs）；
// 空串/非法 JSON/未知字段/root 空白/敏感键一律拒（③-1 凭据行表纪律：
// config_json 永不藏秘密——凭据一律走 credential 行表）。
func TestParseSharedDirConfig(t *testing.T) {
	t.Parallel()
	abs := filepath.ToSlash(t.TempDir())
	cfg, err := ParseSharedDirConfig(`{"root":" ` + abs + ` "}`)
	if err != nil || cfg.Root != abs {
		t.Fatalf("valid parse = %+v, %v", cfg, err)
	}
	for name, raw := range map[string]string{
		"empty":        "",
		"blank":        "   ",
		"bad_json":     `{"root":`,
		"unknown":      `{"root":"/a","extra":1}`,
		"bare_object":  `{}`,
		"empty_root":   `{"root":""}`,
		"blank_root":   `{"root":"  "}`,
		"secret_token": `{"root":"/a","access_token":"x"}`,
		"secret_pass":  `{"root":"/a","password":"x"}`,
		"secret_case":  `{"root":"/a","API_KEY":"x"}`,
	} {
		if _, err := ParseSharedDirConfig(raw); err == nil {
			t.Fatalf("%s 必拒: %q", name, raw)
		}
	}
}

// TestParseSharedDirConfigAbsAndBlacklist 评审修轮 config 钉（wf_6f89c52c-5be
// #W3-F4/#W3-CONFIG-1/2/3/4）：绝对路径硬闸（相对 root 随 CWD 漂移是配置事故）
// + 黑名单消息可触达（probe-first 排列钉——typed decode 前置会把 secret-ish
// 键当未知字段吃掉，本消息存在即非死代码）+ 尾随垃圾拒 + 重复键 last-wins。
func TestParseSharedDirConfigAbsAndBlacklist(t *testing.T) {
	t.Parallel()
	abs := filepath.ToSlash(t.TempDir())

	// 相对 root 必拒且消息可辨
	if _, err := ParseSharedDirConfig(`{"root":"data/vault"}`); err == nil {
		t.Fatal("相对 root 必拒（CWD 漂移事故形态）")
	} else if !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("相对 root 拒绝消息可辨: %v", err)
	}
	// Windows 语义 abs（C:\…）收
	cfg, err := ParseSharedDirConfig(`{"root":"` + abs + `"}`)
	if err != nil || cfg.Root != abs {
		t.Fatalf("平台绝对 root 收: %+v, %v", cfg, err)
	}
	// 黑名单消息可触达（非 unknown-field 泛错——证明 probe-first 排列）
	if _, err := ParseSharedDirConfig(`{"root":"/a","access_token":"x"}`); err == nil {
		t.Fatal("敏感键必拒")
	} else if !strings.Contains(err.Error(), "forbids secret-ish key") {
		t.Fatalf("黑名单消息可触达钉（非死代码）: %v", err)
	}
	// 尾随垃圾拒（probe json.Unmarshal 兜住 Decoder 多值盲区）
	if _, err := ParseSharedDirConfig(`{"root":"/a"} {"root":"/b"}`); err == nil {
		t.Fatal("尾随垃圾必拒")
	}
	// 重复键 last-wins（encoding/json 语义钉住——配置生成器重复输出不炸）
	cfg, err = ParseSharedDirConfig(`{"root":"` + abs + `/a","root":"` + abs + `/b"}`)
	if err != nil || cfg.Root != abs+"/b" {
		t.Fatalf("重复键 last-wins: %+v, %v", cfg, err)
	}
}

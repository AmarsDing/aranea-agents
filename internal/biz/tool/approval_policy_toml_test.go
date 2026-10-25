package tool

import (
	"strings"
	"testing"
)

// TestParseApprovalPoliciesTOML_ValidRows 覆盖合法多行文档：四档 mode（含 D5
// 新增 auto）、enabled 缺省视为 true、glob/re: pattern 与可空维度。
func TestParseApprovalPoliciesTOML_ValidRows(t *testing.T) {
	t.Parallel()
	doc := `
[[policies]]
id = "ci-ask"
scope = "session"
scope_key = "sess-1"
tool_pattern = "shell*"
command_prefix = "re:^git\\s"
mode = "ask"
priority = 10

[[policies]]
id = "ci-auto"
scope = "agent"
scope_key = "agent-1"
tool_pattern = "*"
mode = "auto"
enabled = false
shadow_only = true
expires_at = 1893456000
`
	policies, rowErrs, err := ParseApprovalPoliciesTOML([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rowErrs) != 0 {
		t.Fatalf("unexpected row errors: %v", rowErrs)
	}
	if len(policies) != 2 {
		t.Fatalf("policies len = %d, want 2", len(policies))
	}
	if policies[0].ID != "ci-ask" || policies[0].Priority != 10 || !policies[0].Enabled {
		t.Fatalf("row0 = %+v", policies[0])
	}
	p2 := policies[1]
	if p2.Mode != string(ApprovalModeAuto) || p2.Enabled || !p2.ShadowOnly || p2.ExpiresAt != 1893456000 {
		t.Fatalf("row1 = %+v", p2)
	}
}

// TestParseApprovalPoliciesTOML_RowErrorsPartialImport 钉死行级校验失败不中断
// 整批：坏行逐条收入 "policies[i]: …"，合法行照常返回。
func TestParseApprovalPoliciesTOML_RowErrorsPartialImport(t *testing.T) {
	t.Parallel()
	doc := `
[[policies]]
id = "ok-1"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "trusted"

[[policies]]
id = "bad-mode"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "assisted"

[[policies]]
id = "bad-regex"
scope = "session"
scope_key = "sess-1"
tool_pattern = "re:["
mode = "ask"

[[policies]]
id = "ok-2"
scope = "agent"
scope_key = "agent-9"
tool_pattern = "shell*"
mode = "never"
`
	policies, rowErrs, err := ParseApprovalPoliciesTOML([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(policies) != 2 || policies[0].ID != "ok-1" || policies[1].ID != "ok-2" {
		t.Fatalf("partial import rows wrong: %+v", policies)
	}
	if len(rowErrs) != 2 {
		t.Fatalf("rowErrs = %v, want 2", rowErrs)
	}
	if !strings.HasPrefix(rowErrs[0], "policies[1]: ") || !strings.Contains(rowErrs[0], "mode") {
		t.Fatalf("rowErrs[0] = %q", rowErrs[0])
	}
	if !strings.HasPrefix(rowErrs[1], "policies[2]: ") {
		t.Fatalf("rowErrs[1] = %q", rowErrs[1])
	}
}

// TestParseApprovalPoliciesTOML_SyntaxErrorSanitized 钉死文档级语法错误的
// 脱敏纪律：只回行/列定位，不回显载荷原文（TOML 解析器错误消息可能携带
// 载荷片段）。
func TestParseApprovalPoliciesTOML_SyntaxErrorSanitized(t *testing.T) {
	t.Parallel()
	payload := "[[policies]\nsuper_secret_payload_xyz"
	_, _, err := ParseApprovalPoliciesTOML([]byte(payload))
	if err == nil {
		t.Fatal("broken TOML must be rejected")
	}
	if strings.Contains(err.Error(), "super_secret_payload_xyz") {
		t.Fatalf("syntax error must not echo payload, got %v", err)
	}
	if !strings.Contains(err.Error(), "语法错误") {
		t.Fatalf("want structured position error, got %v", err)
	}
}

// TestParseApprovalPoliciesTOML_RowCountLimit 钉死防御性行数上限。
func TestParseApprovalPoliciesTOML_RowCountLimit(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	row := "[[policies]]\nid = \"x\"\nscope = \"session\"\nscope_key = \"s\"\ntool_pattern = \"*\"\nmode = \"ask\"\n"
	for i := 0; i < approvalPolicyTOMLMaxRows+1; i++ {
		b.WriteString(row)
	}
	if _, _, err := ParseApprovalPoliciesTOML([]byte(b.String())); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("row count over limit must be rejected, got %v", err)
	}
	// F16：恰在上限（5000 行）必须放行——上限只拒超界，不误伤合法大批量
	// 导入（200+ 岗位的 agent 作用域姿态表可逼近千行量级，边界放行须钉死）。
	var ok strings.Builder
	for i := 0; i < approvalPolicyTOMLMaxRows; i++ {
		ok.WriteString(row)
	}
	policies, rowErrs, err := ParseApprovalPoliciesTOML([]byte(ok.String()))
	if err != nil || len(rowErrs) != 0 {
		t.Fatalf("at-limit doc must parse clean: err=%v rowErrs=%v", err, rowErrs)
	}
	if len(policies) != approvalPolicyTOMLMaxRows {
		t.Fatalf("at-limit rows = %d, want %d", len(policies), approvalPolicyTOMLMaxRows)
	}
}

// TestParseApprovalPoliciesTOML_ByteLimit 钉死字节上限（D5-f F5/F9）：超
// 1MiB 载荷在 toml.Unmarshal 之前拒收——行数上限只能挡海量短行，单行长
// payload 的攻击面由字节级闸门封住；合法小文档不误伤。
func TestParseApprovalPoliciesTOML_ByteLimit(t *testing.T) {
	t.Parallel()
	over := make([]byte, approvalPolicyTOMLMaxBytes+1)
	copy(over, "[[policies]]\n")
	if _, _, err := ParseApprovalPoliciesTOML(over); err == nil || !strings.Contains(err.Error(), "字节") {
		t.Fatalf("over-size payload must be rejected pre-unmarshal, got %v", err)
	}
	doc := "[[policies]]\nid = \"x\"\nscope = \"session\"\nscope_key = \"s\"\ntool_pattern = \"*\"\nmode = \"ask\"\n"
	policies, rowErrs, err := ParseApprovalPoliciesTOML([]byte(doc))
	if err != nil || len(rowErrs) != 0 || len(policies) != 1 {
		t.Fatalf("small valid doc must parse: err=%v rowErrs=%v policies=%v", err, rowErrs, policies)
	}
}

// TestMarshalApprovalPoliciesTOML_EmptyRoundtrip D5-f F17：空集合导出→导入
// 回环=0 行零错误（空文档不得触发语法错误或幻影行）。
func TestMarshalApprovalPoliciesTOML_EmptyRoundtrip(t *testing.T) {
	t.Parallel()
	data, err := MarshalApprovalPoliciesTOML(nil)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	policies, rowErrs, err := ParseApprovalPoliciesTOML(data)
	if err != nil || len(rowErrs) != 0 || len(policies) != 0 {
		t.Fatalf("empty roundtrip must yield zero rows: policies=%v rowErrs=%v err=%v", policies, rowErrs, err)
	}
}

// TestMarshalApprovalPoliciesTOML_Roundtrip 钉死导出→导入回环：交换格式不
// 携带 created_at/created_by（审计元数据 server 侧权威），其余字段逐点一致。
func TestMarshalApprovalPoliciesTOML_Roundtrip(t *testing.T) {
	t.Parallel()
	in := []ApprovalPolicy{
		{
			ID: "r-1", Scope: "session", ScopeKey: "sess-1", ToolPattern: "shell*",
			CommandPrefix: "re:^git\\s", Mode: string(ApprovalModeAuto), Priority: 5,
			Enabled: true, ExpiresAt: 1893456000, CreatedAt: 1700000000, CreatedBy: "admin",
		},
		{
			ID: "r-2", Scope: "agent", ScopeKey: "agent-1", ToolPattern: "*",
			Mode: string(ApprovalModeTrusted), Enabled: false, ShadowOnly: true,
			CreatedAt: 1700000001, CreatedBy: "admin",
		},
	}
	data, err := MarshalApprovalPoliciesTOML(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "created_at") || strings.Contains(string(data), "created_by") {
		t.Fatalf("exported TOML must not carry audit metadata: %s", data)
	}
	out, rowErrs, err := ParseApprovalPoliciesTOML(data)
	if err != nil || len(rowErrs) != 0 {
		t.Fatalf("re-parse: %v %v", err, rowErrs)
	}
	if len(out) != 2 {
		t.Fatalf("roundtrip len = %d, want 2", len(out))
	}
	for i := range in {
		want, got := in[i], out[i]
		if want.ID != got.ID || want.Scope != got.Scope || want.ScopeKey != got.ScopeKey ||
			want.ToolPattern != got.ToolPattern || want.CommandPrefix != got.CommandPrefix ||
			want.ArgsPattern != got.ArgsPattern || want.Mode != got.Mode ||
			want.Priority != got.Priority || want.Enabled != got.Enabled ||
			want.ShadowOnly != got.ShadowOnly || want.ExpiresAt != got.ExpiresAt {
			t.Fatalf("row %d mismatch:\nwant %+v\ngot  %+v", i, want, got)
		}
		if got.CreatedAt != 0 || got.CreatedBy != "" {
			t.Fatalf("row %d must not carry audit metadata through exchange format: %+v", i, got)
		}
	}
}

// TestParseApprovalPoliciesTOML_UnknownKeysIgnored 钉死宽容解析：未知键
// （如手写的 created_at）静默忽略——交换格式只管姿态配置。
func TestParseApprovalPoliciesTOML_UnknownKeysIgnored(t *testing.T) {
	t.Parallel()
	doc := `
[[policies]]
id = "x"
scope = "session"
scope_key = "s"
tool_pattern = "*"
mode = "ask"
created_at = 123
created_by = "mallory"
future_field = { nested = "aGVsbG8=" }
`
	policies, rowErrs, err := ParseApprovalPoliciesTOML([]byte(doc))
	if err != nil || len(rowErrs) != 0 {
		t.Fatalf("unknown keys must be ignored: %v %v", err, rowErrs)
	}
	if len(policies) != 1 || policies[0].CreatedAt != 0 || policies[0].CreatedBy != "" {
		t.Fatalf("audit metadata must stay zero after import, got %+v", policies)
	}
}

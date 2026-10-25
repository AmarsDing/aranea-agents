package tool

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"aranea-agents/internal/tools/preview"
)

// P0-5 phase 2（D5）：审批姿态表的 TOML 批量导入/导出。形状刻意与 ApprovalPolicy
// struct 解耦（独立 DTO，不依赖 struct tag——未来业务 struct 加字段不泄漏进
// 交换格式）：
//
//	[[policies]]
//	id            = "ci-nightly-shell"
//	scope         = "session"          # session | agent
//	scope_key     = "sess-1"
//	tool_pattern  = "shell*"
//	command_prefix = "re:^git\\s"      # 可空
//	args_pattern  = ""                 # 可空
//	mode          = "ask"              # never | auto | ask | trusted
//	priority      = 100
//	enabled       = true
//	shadow_only   = false
//	expires_at    = 0                  # unix 秒；0 = 不过期
//
// created_at / created_by 不接受导入（server 侧权威：CreatedBy=操作者、
// CreatedAt=now；更新路径保留原值），导出时亦不携带（交换格式只管姿态配置，
// 不碰审计元数据）。

// approvalPolicyTOMLDoc 是 TOML 文档的传输形状。
type approvalPolicyTOMLDoc struct {
	Policies []approvalPolicyTOMLRow `toml:"policies"`
}

type approvalPolicyTOMLRow struct {
	ID            string `toml:"id"`
	Scope         string `toml:"scope"`
	ScopeKey      string `toml:"scope_key"`
	ToolPattern   string `toml:"tool_pattern"`
	CommandPrefix string `toml:"command_prefix"`
	ArgsPattern   string `toml:"args_pattern"`
	Mode          string `toml:"mode"`
	Priority      int    `toml:"priority"`
	Enabled       *bool  `toml:"enabled"`
	ShadowOnly    bool   `toml:"shadow_only"`
	ExpiresAt     int64  `toml:"expires_at"`
}

const approvalPolicyTOMLMaxRows = 5000 // 防御性上限：单文档行数，防巨型载荷拖垮管理面

// ParseApprovalPoliciesTOML 解析 TOML 文档为经 ValidateApprovalPolicy 单源校验
// 的策略行集合。文档级语法错误返回 err（脱敏后结构化信息——不回显原文，防
// TOML 解析器把载荷片段拼进错误消息造成信息回流）；行级校验失败不中断整批，
// 逐条收入 rowErrs（"policies[i]: …" 预脱敏格式），合法行照常返回交由调用方
// 决定部分导入策略。enabled 缺省视为 true（与表列默认对齐）。
func ParseApprovalPoliciesTOML(data []byte) (policies []ApprovalPolicy, rowErrs []string, err error) {
	var doc approvalPolicyTOMLDoc
	if uerr := toml.Unmarshal(data, &doc); uerr != nil {
		var derr *toml.DecodeError
		if errors.As(uerr, &derr) {
			row, col := derr.Position()
			return nil, nil, fmt.Errorf("approval policies TOML 语法错误（第 %d 行第 %d 列）", row, col)
		}
		return nil, nil, fmt.Errorf("approval policies TOML 解析失败: %s", preview.RedactAndTruncate(uerr.Error(), 200))
	}
	if len(doc.Policies) > approvalPolicyTOMLMaxRows {
		return nil, nil, fmt.Errorf("approval policies TOML 行数 %d 超过上限 %d", len(doc.Policies), approvalPolicyTOMLMaxRows)
	}
	policies = make([]ApprovalPolicy, 0, len(doc.Policies))
	for i := range doc.Policies {
		row := &doc.Policies[i]
		enabled := true
		if row.Enabled != nil {
			enabled = *row.Enabled
		}
		p := ApprovalPolicy{
			ID:            strings.TrimSpace(row.ID),
			Scope:         strings.TrimSpace(row.Scope),
			ScopeKey:      strings.TrimSpace(row.ScopeKey),
			ToolPattern:   strings.TrimSpace(row.ToolPattern),
			CommandPrefix: strings.TrimSpace(row.CommandPrefix),
			ArgsPattern:   strings.TrimSpace(row.ArgsPattern),
			Mode:          strings.TrimSpace(row.Mode),
			Priority:      row.Priority,
			Enabled:       enabled,
			ShadowOnly:    row.ShadowOnly,
			ExpiresAt:     row.ExpiresAt,
		}
		if verr := ValidateApprovalPolicy(p); verr != nil {
			rowErrs = append(rowErrs, fmt.Sprintf("policies[%d]: %s", i, preview.RedactAndTruncate(verr.Error(), 200)))
			continue
		}
		policies = append(policies, p)
	}
	return policies, rowErrs, nil
}

// MarshalApprovalPoliciesTOML 导出策略行集合为规范 TOML 文档（键序固定、
// 审计元数据不携带）。确定性输出——diff/归档友好。
func MarshalApprovalPoliciesTOML(policies []ApprovalPolicy) ([]byte, error) {
	doc := approvalPolicyTOMLDoc{Policies: make([]approvalPolicyTOMLRow, 0, len(policies))}
	for i := range policies {
		p := &policies[i]
		doc.Policies = append(doc.Policies, approvalPolicyTOMLRow{
			ID:            p.ID,
			Scope:         p.Scope,
			ScopeKey:      p.ScopeKey,
			ToolPattern:   p.ToolPattern,
			CommandPrefix: p.CommandPrefix,
			ArgsPattern:   p.ArgsPattern,
			Mode:          p.Mode,
			Priority:      p.Priority,
			Enabled:       boolPtr(p.Enabled),
			ShadowOnly:    p.ShadowOnly,
			ExpiresAt:     p.ExpiresAt,
		})
	}
	return toml.Marshal(doc)
}

func boolPtr(b bool) *bool { return &b }

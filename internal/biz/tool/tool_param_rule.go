package tool

import (
	"context"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz/policyrule"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// 79-runtime-governance R9（Phase 5.4/5.5）：工具参数模式权限。
// BeforeTool paramRuleGate 在循环守卫（priority 4）之前求值；生效语义
// deny > ask > allow > fallback（fallback = 工具自身 requires_confirmation）。

// BuiltinParamRuleIDPrefix 标记内置 seed 规则。builtin 规则 effect 只读、
// 不可删除（与 builtin policy 只读纪律一致）；pattern/priority/enabled 可改，
// 可增自定义规则。
const BuiltinParamRuleIDPrefix = "builtin-"

// ToolParamRule 是 tool_param_rules 表的一行。
type ToolParamRule struct {
	ID        string
	ToolKey   string // 别名家簇定点键（CanonicalParamRuleToolKey 归一后的运行时名）
	Pattern   string // glob；`re:` 前缀为正则（见 policyrule.MatchText）
	Effect    string // deny | ask | allow
	Priority  int
	Enabled   bool
	CreatedAt int64 // unix 秒
}

// ToolParamRuleReader 是运行时 gate 的读口。
// Stability:evolving
type ToolParamRuleReader interface {
	ListEnabledParamRules(ctx context.Context, toolKey string) ([]ToolParamRule, error)
}

// ToolParamRuleAdmin 是管理面写口。
// Stability:evolving
type ToolParamRuleAdmin interface {
	// ListParamRules 返回该工具全部规则（含 disabled），按 priority 升序。
	ListParamRules(ctx context.Context, toolKey string) ([]ToolParamRule, error)
	// GetParamRuleByID 按主键查单条（跨 tool_key 全表）；不存在返回
	// shared.ErrNotFound。builtin 只读校验必须按 ID 查——按 tool_key 查会被
	// 「同 Upsert 换 tool_key」绕过（P5.4 审计 H3）。
	GetParamRuleByID(ctx context.Context, id string) (ToolParamRule, error)
	// UpsertParamRule：ID 已存在为更新，否则新建（ID 由调用方给定）。
	UpsertParamRule(ctx context.Context, rule ToolParamRule) error
	// DeleteParamRule 幂等。
	DeleteParamRule(ctx context.Context, id string) error
}

// ToolParamRuleStore combines reader and admin; used only for Wire wiring.
// Stability:evolving
type ToolParamRuleStore interface {
	ToolParamRuleReader
	ToolParamRuleAdmin
}

// WithToolParamRuleStore wires the param-rule store into the usecase.
func WithToolParamRuleStore(store ToolParamRuleStore) ToolUsecaseOption {
	return func(u *ToolUsecase) { u.paramRules = store }
}

// CanonicalParamRuleToolKey 归一工具键到别名家簇定点（与运行时 gate 的
// loopGuardCanonicalToolName 同一键空间：shell/shell_exec → exec_command）。
// 管理 API 传入任意别名都落到同一行集合。
func CanonicalParamRuleToolKey(key string) string {
	key = strings.TrimSpace(key)
	visited := map[string]bool{}
	for {
		next := NormalizeToolPolicyKey(key)
		if next == key || visited[next] {
			return key
		}
		visited[next] = true
		key = next
	}
}

// ListEnabledParamRulesForGate 是 gate 的运行时读口：键归一 + 查启用规则。
// store 未装配（nil）时返回 nil（调用方不注册 gate）。
func (u *ToolUsecase) ListEnabledParamRulesForGate(ctx context.Context, toolKey string) ([]ToolParamRule, error) {
	if u == nil || u.paramRules == nil {
		return nil, nil
	}
	return u.paramRules.ListEnabledParamRules(ctx, CanonicalParamRuleToolKey(toolKey))
}

// HasParamRuleStore 报告 paramRules store 是否已装配。agent 包确认门禁的
// 保活判定经可选接口断言消费本方法（ask verdict 需要 decide() 消费者），
// 不扩张 TeamToolLookup 契约。
func (u *ToolUsecase) HasParamRuleStore() bool {
	return u != nil && u.paramRules != nil
}

// ListToolParamRules 返回工具全部规则（管理面）。
func (u *ToolUsecase) ListToolParamRules(ctx context.Context, toolKey string) ([]ToolParamRule, error) {
	if u == nil || u.paramRules == nil {
		return nil, apierror.Internal("TOOL", "tool param rule store unavailable")
	}
	toolKey = CanonicalParamRuleToolKey(toolKey)
	if toolKey == "" {
		return nil, apierror.BadRequest("TOOL", "tool_key is required")
	}
	return u.paramRules.ListParamRules(ctx, toolKey)
}

// UpsertToolParamRule 新建/更新规则。校验：effect 枚举、pattern 可编译、
// builtin 规则 effect 只读（其余字段可改）。
func (u *ToolUsecase) UpsertToolParamRule(ctx context.Context, rule ToolParamRule) error {
	if u == nil || u.paramRules == nil {
		return apierror.Internal("TOOL", "tool param rule store unavailable")
	}
	rule.ToolKey = CanonicalParamRuleToolKey(rule.ToolKey)
	rule.Pattern = strings.TrimSpace(rule.Pattern)
	rule.ID = strings.TrimSpace(rule.ID)
	if rule.ToolKey == "" || rule.Pattern == "" || rule.ID == "" {
		return apierror.BadRequest("TOOL", "id, tool_key and pattern are required")
	}
	switch policyrule.Effect(rule.Effect) {
	case policyrule.EffectDeny, policyrule.EffectAsk, policyrule.EffectAllow:
	default:
		return apierror.BadRequest("TOOL", "effect must be deny|ask|allow")
	}
	if _, err := policyrule.MatchText(rule.Pattern, ""); err != nil {
		return apierror.BadRequest("TOOL", "pattern 不可编译: "+err.Error())
	}
	if strings.HasPrefix(rule.ID, BuiltinParamRuleIDPrefix) {
		// 按 ID 全表查（非按新 tool_key 查）——否则 Upsert 同 ID 换 tool_key
		// 可跳过校验，ON CONFLICT 随即把 builtin 行搬走并改写 effect（H3）。
		existing, err := u.paramRules.GetParamRuleByID(ctx, rule.ID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if err == nil {
			if existing.ToolKey != rule.ToolKey {
				return apierror.BadRequest("TOOL", "builtin 规则 tool_key 只读（可改 pattern/priority/enabled，或新增自定义规则）")
			}
			if existing.Effect != rule.Effect {
				return apierror.BadRequest("TOOL", "builtin 规则 effect 只读（可改 pattern/priority/enabled，或新增自定义规则）")
			}
		}
	}
	if rule.CreatedAt == 0 {
		rule.CreatedAt = time.Now().Unix()
	}
	return u.paramRules.UpsertParamRule(ctx, rule)
}

// DeleteToolParamRule 删除规则。builtin 规则不可删除（否则只读纪律可被
// 删除重建绕过）。
func (u *ToolUsecase) DeleteToolParamRule(ctx context.Context, id string) error {
	if u == nil || u.paramRules == nil {
		return apierror.Internal("TOOL", "tool param rule store unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest("TOOL", "id is required")
	}
	if strings.HasPrefix(id, BuiltinParamRuleIDPrefix) {
		return apierror.BadRequest("TOOL", "builtin 规则不可删除（可置 enabled=false）")
	}
	return u.paramRules.DeleteParamRule(ctx, id)
}

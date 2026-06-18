package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/prompt"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// TECH-DEBT(B-6): RenderStateTemplate 已实现但未接入生产路径。
// 阻塞类型：设计前提不匹配（同步渲染 vs 异步注入）。
// 项目使用 trpcagent.MergeRuntimeState() 在模型调用时异步注入运行时状态到框架内部状态管理，
// 而 RenderStateTemplate 在 prompt 构建时同步渲染状态到文本。两者机制不同，接入会造成双重状态注入。
// 解除方案：Phase 1 AgentFactory 实施时重新评估（与 B-5 同步评估）。
// 详见 docs/trpc-agent-go/alignment-plan.md §十一/B-6。

// RenderStateTemplate renders template placeholders using values from the
// invocation state and session state, following the same resolution rules as
// the framework's internal state.Render adapter.
//
// This is the framework-aligned approach for injecting runtime state into
// prompt templates. It supports:
//   - {name} or {{name}} for bare identifiers
//   - {name?} or {{name?}} for optional identifiers
//   - {app:key}, {user:key}, {temp:key} for namespaced session state
//   - {invocation:key} for invocation-scoped state
//   - {artifact.filename} or {artifact.filename?} for artifact references
//
// {invocation:*} placeholders are resolved from invocation state only.
// Other supported placeholders are resolved from session state.
// Optional placeholders collapse to empty when unresolved; non-optional
// unresolved placeholders remain literal.
//
// This function re-implements the resolution logic of the framework's internal
// state.Render (pkg/trpc-agent-go/internal/prompt/adapter/state/) using the
// public prompt.Text.Render API, since the internal package is not importable.
func RenderStateTemplate(template string, invocation *trpcagent.Invocation, sess *trpcsession.Session) (string, error) {
	if template == "" {
		return template, nil
	}

	resolver := &stateResolver{
		invocation: invocation,
		session:    sess,
	}

	text := prompt.Text{
		Template: template,
		Syntax:   prompt.SyntaxMixedBrace,
	}

	rendered, err := text.Render(
		prompt.RenderEnv{Resolver: resolver},
		prompt.WithUnknownBehavior(prompt.PreserveUnknown),
	)
	if err != nil {
		return template, err
	}
	return rendered, nil
}

// stateResolver resolves placeholder names from invocation state and session
// state, mirroring the framework's internal stateResolver logic.
type stateResolver struct {
	invocation *trpcagent.Invocation
	session    *trpcsession.Session
}

const invocationKeyPrefix = "invocation:"

// Resolve implements prompt.Resolver.
func (r *stateResolver) Resolve(ref prompt.Ref) (string, bool, error) {
	name := ref.Name

	// {invocation:key} → invocation state only.
	if key, ok := strings.CutPrefix(name, invocationKeyPrefix); ok {
		if r.invocation != nil {
			if val, exists := r.invocation.GetState(key); exists && val != nil {
				return fmt.Sprintf("%+v", val), true, nil
			}
		}
		return "", false, nil
	}

	// Other placeholders → session state.
	sess := r.session
	if sess == nil && r.invocation != nil {
		sess = r.invocation.Session
	}
	if sess != nil {
		if jsonBytes, exists := sess.GetState(name); exists {
			return renderStateValue(jsonBytes), true, nil
		}
	}

	return "", false, nil
}

// renderStateValue converts a raw state value to its string representation,
// preserving JSON semantics. This mirrors the framework's internal
// renderStateValue function.
func renderStateValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if !json.Valid(raw) {
		return string(raw)
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var jsonValue any
	if err := dec.Decode(&jsonValue); err != nil {
		return string(raw)
	}
	switch v := jsonValue.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		// Preserve JSON objects/arrays as JSON text so injection does not
		// degrade them into Go's fmt representation.
		return string(raw)
	}
}

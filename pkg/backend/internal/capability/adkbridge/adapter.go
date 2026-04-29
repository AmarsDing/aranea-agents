package adkbridge

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"arenea/backend/internal/capability/executor"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/capability/tooldef"
)

type Options struct {
	BaseContext *toolctx.ToolContext
	Executor    *executor.Executor
}

func ToADKTool(t tooldef.Tool, opts Options) (adktool.Tool, error) {
	if t == nil {
		return nil, fmt.Errorf("nil tool")
	}
	exec := opts.Executor
	if exec == nil {
		exec = executor.New()
	}
	cfg := functiontool.Config{
		Name:                t.Name(),
		Description:         t.Description(),
		InputSchema:         schemaFromMap(t.InputSchema()),
		OutputSchema:        schemaFromMap(t.OutputSchema()),
		RequireConfirmation: requiresConfirmation(t, opts.BaseContext),
	}
	if _, ok := t.(tooldef.StreamingTool); ok {
		cfg.IsLongRunning = true
	}
	return functiontool.New(cfg, func(ctx adktool.Context, args map[string]any) (map[string]any, error) {
		tc := toolctx.New(nil)
		if opts.BaseContext != nil {
			tc = opts.BaseContext.Clone(nil)
		}
		if ctx != nil {
			tc.FunctionCallID = ctx.FunctionCallID()
		}
		return exec.Run(tc, t, args)
	})
}

func requiresConfirmation(t tooldef.Tool, base *toolctx.ToolContext) bool {
	if approvable, ok := t.(tooldef.ApprovableTool); ok {
		return approvable.RequiresApproval(base, nil)
	}
	return false
}

func schemaFromMap(raw map[string]any) *jsonschema.Schema {
	if len(raw) == 0 {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(data, &schema); err != nil {
		return nil
	}
	return &schema
}

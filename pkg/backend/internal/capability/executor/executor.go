package executor

import (
	"arenea/backend/internal/capability/middleware"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/capability/tooldef"
)

type Executor struct {
	chain middleware.Middleware
}

func New(mws ...middleware.Middleware) *Executor {
	return &Executor{chain: middleware.BuildChain(mws...)}
}

func (e *Executor) Run(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any) (map[string]any, error) {
	if e == nil || e.chain == nil {
		return t.Execute(ctx, params)
	}
	if params == nil {
		params = map[string]any{}
	}
	return e.chain.Run(ctx, t, params, middleware.FinalExecutor())
}

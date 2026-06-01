package deferred

import (
	"context"
	"fmt"
	"sync"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type DeferredCallableTool struct {
	mu         sync.Mutex
	resolved   bool
	factory    func(ctx context.Context) (trpctool.Tool, error)
	tool       trpctool.Tool
	resolveErr error
	decl       *trpctool.Declaration
	lg         loggateway.Logger
}

func NewDeferredCallableTool(decl *trpctool.Declaration, factory func(ctx context.Context) (trpctool.Tool, error), lg loggateway.Logger) *DeferredCallableTool {
	return &DeferredCallableTool{
		decl:    decl,
		factory: factory,
		lg:      lg,
	}
}

func (d *DeferredCallableTool) Declaration() *trpctool.Declaration {
	return d.decl
}

func (d *DeferredCallableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if err := d.resolve(ctx); err != nil {
		return nil, fmt.Errorf("deferred tool resolution failed: %w", err)
	}
	if callable, ok := d.tool.(trpctool.CallableTool); ok {
		return callable.Call(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("deferred tool does not implement CallableTool")
}

func (d *DeferredCallableTool) resolve(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.resolved {
		return nil
	}
	d.tool, d.resolveErr = d.factory(ctx)
	if d.resolveErr != nil {
		d.lg.Warn("deferred tool factory failed",
			loggateway.StepID("tool.deferred.factory_fail"),
			loggateway.Str("tool", d.decl.Name),
			loggateway.Err(d.resolveErr),
		)
		return d.resolveErr
	}
	d.resolved = true
	return nil
}

package deferred

import (
	"context"
	"fmt"
	"sync"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type DeferredCallableTool struct {
	mu       sync.Mutex
	resolved bool
	factory  func(ctx context.Context) (trpctool.Tool, error)
	tool     trpctool.Tool
	decl     *trpctool.Declaration
	lg       loggateway.Logger
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
		return nil, kerrors.InternalServer("DEFERRED_TOOL", "deferred tool resolution failed: "+err.Error())
	}
	if callable, ok := d.tool.(trpctool.CallableTool); ok {
		return callable.Call(ctx, jsonArgs)
	}
	return nil, kerrors.InternalServer("DEFERRED_TOOL", "deferred tool does not implement CallableTool")
}

func (d *DeferredCallableTool) resolve(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.resolved {
		return nil
	}
	tool, err := d.factory(ctx)
	if err != nil {
		d.lg.Warn("deferred tool factory failed",
			loggateway.StepID("tool.deferred.factory_fail"),
			loggateway.Str("tool", d.decl.Name),
			loggateway.Err(err),
		)
		return err
	}
	if tool == nil {
		nilErr := fmt.Errorf("deferred tool %q: factory returned nil without error", d.decl.Name)
		d.lg.Warn("deferred tool factory returned nil",
			loggateway.StepID("tool.deferred.factory_nil"),
			loggateway.Str("tool", d.decl.Name),
			loggateway.Err(nilErr),
		)
		return nilErr
	}
	d.tool = tool
	d.resolved = true
	return nil
}

package app

import (
	"context"
	"errors"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/kernel/module"
)

// BootstrapPorts runs stages 1 and 2 of the four-stage Module lifecycle:
// every module first registers its output ports (RegisterPorts), then every
// module resolves its dependencies (ResolvePorts). Splitting the two phases
// is what allows mutually-aware Contexts to coexist without import cycles.
func BootstrapPorts(mods []module.Module, reg *contracts.Registry) {
	for _, m := range mods {
		m.RegisterPorts(reg)
	}
	for _, m := range mods {
		m.ResolvePorts(reg)
	}
}

// StartModules calls Start on every module sequentially, aborting on the
// first error and propagating it to the caller. Successfully-started modules
// are NOT auto-rolled back here; the launcher is responsible for invoking
// ShutdownModules in its defer block.
func StartModules(ctx context.Context, mods []module.Module) error {
	for _, m := range mods {
		if err := m.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ShutdownModules calls Shutdown on every module in reverse order, joining
// any errors via errors.Join. Each Shutdown is bounded by ctx's deadline.
func ShutdownModules(ctx context.Context, mods []module.Module) error {
	var errs []error
	for i := len(mods) - 1; i >= 0; i-- {
		if err := mods[i].Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

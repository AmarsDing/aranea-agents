package heal

import "github.com/google/wire"

// WireProviderSet exports the heal domain Wire providers.
var WireProviderSet = wire.NewSet(
	NewRootCauseEngine,
)

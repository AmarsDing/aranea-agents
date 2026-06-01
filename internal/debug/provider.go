package debug

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewRecorderFactory)

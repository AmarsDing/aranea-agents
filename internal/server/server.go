package server

import (
	"aranea-agents/internal/skill/importer"

	"github.com/google/wire"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewSSEServer, importer.NewEngine)

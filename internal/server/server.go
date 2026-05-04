package server

import (
	"aranea-agents/internal/skillimport"

	"github.com/google/wire"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewSSEServer, skillimport.NewEngine)

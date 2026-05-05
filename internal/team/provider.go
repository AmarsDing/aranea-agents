package team

import "github.com/google/wire"

// ProviderSet wires team runtime.
var ProviderSet = wire.NewSet(NewRunner)

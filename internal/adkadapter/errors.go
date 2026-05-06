package adkadapter

import "errors"

var (
	// ErrNilCatalog is returned when LlmProviderModelUsecase is nil.
	ErrNilCatalog = errors.New("adkadapter: nil catalog usecase")
)

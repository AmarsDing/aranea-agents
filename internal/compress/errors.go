package compress

import "errors"

var (
	ErrCatalogRequired       = errors.New("compress: catalog required")
	ErrHTTPClientRequired    = errors.New("compress: http client required")
	ErrProviderModelRequired = errors.New("compress: provider and model required")
	ErrEmptyTranscript       = errors.New("compress: transcript empty")
)

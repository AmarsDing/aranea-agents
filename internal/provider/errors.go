package provider

import (
	"errors"
)

// Sentinel errors. They are wrapped by fmt.Errorf("...: %w", err) at every
// failure site so callers can use errors.Is for mapping to gRPC / HTTP status
// codes (see internal/service/*_errors.go for the mapping table).
var (
	// ErrNilLlmCatalog is returned when the caller passes a nil catalog to
	// functions that require a non-nil one (e.g. TRPCModelForProviderModel).
	ErrNilLlmCatalog = errors.New("provider: llm catalog is nil")

	// ErrEmptyModelAPI is returned when a provider config has a non-empty
	// ProviderType but an empty/blank ModelAPI.
	ErrEmptyModelAPI = errors.New("provider: model api id is empty")

	// ErrInvalidConfigJSON is returned when the catalog's config_json is not
	// valid JSON. The underlying json error is wrapped via %w.
	ErrInvalidConfigJSON = errors.New("provider: invalid config_json")

	// ErrProviderRateLimit is returned by the rate-limit transport when the
	// caller has exhausted its token bucket.
	ErrProviderRateLimit = errors.New("provider: rate limit exceeded")

	// ErrModelNilChannel is returned by WrapModelWithMetrics when the inner
	// model returns (nil, nil) from GenerateContent.
	ErrModelNilChannel = errors.New("provider: inner model returned nil channel")

	// ErrImageNotSupported is returned by ValidateAttachmentCapabilities when
	// the model does not support image attachments.
	ErrImageNotSupported = errors.New("provider: model does not support image attachments")

	// ErrFileNotSupported is returned by ValidateAttachmentCapabilities when
	// the model does not support file attachments.
	ErrFileNotSupported = errors.New("provider: model does not support file attachments")
)

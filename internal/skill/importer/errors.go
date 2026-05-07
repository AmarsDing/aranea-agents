package importer

import "errors"

var (
	ErrImportJobNotFound     = errors.New("import job not found")
	ErrConflictGroupNotFound = errors.New("conflict group not found")
	ErrNoChatModelConfigured = errors.New("no configured model with API base URL and API key is available")
)

package biz

import "aranea-agents/internal/biz/shared"

// Re-export validateDocumentAgainstSchema from shared sub-package for backward compatibility.
var validateDocumentAgainstSchema = shared.ValidateDocumentAgainstSchema

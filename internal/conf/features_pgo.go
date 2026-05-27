// Package conf provides runtime configuration helpers.
package conf

import "os"

// PGO-PRE-01: M58 Prompt Governance & Org Automation feature flags.
// All flags default to false (off) except PGO_CLI_IMPORT_ENABLED.
// Enable in dev/staging by setting the env var to "1" or "true".

// PGODefaultFilesV2 controls whether new agents are created with the V2
// default prompt file set (5 core files) instead of the legacy 9-file set.
// PGO-1-BIZ-01: When true, defaultPromptFiles() returns 5 files.
func PGODefaultFilesV2() bool {
	return parseBoolFlag("PGO_DEFAULT_FILES_V2")
}

// PGOCategoryResponsibilityInject controls whether the position-level
// description (岗位职责) from agent_category_nodes is injected into the
// system instruction as a <role_responsibility> block.
// PGO-1-AGENT-02: When false, BuildSystemPrompt behaves exactly as before.
func PGOCategoryResponsibilityInject() bool {
	return parseBoolFlag("PGO_CATEGORY_RESPONSIBILITY_INJECT")
}

// PGOAIRefineV2 controls whether the new unified /v1/ai/refine endpoint
// and frontend AIRefineButton are active.
// PGO-3: When false, the legacy /v1/agents/{id}/files/{fid}/ai-edit path
// is used unchanged.
func PGOAIRefineV2() bool {
	return parseBoolFlag("PGO_AI_REFINE_V2")
}

// PGOCLIImportEnabled controls whether the `aranea import` sub-command
// is registered in the CLI binary. Defaults to true.
func PGOCLIImportEnabled() bool {
	v := os.Getenv("PGO_CLI_IMPORT_ENABLED")
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true // default on
}

package tool

var ValidateToolUpsert = validateToolUpsert
var AssertToolMutable = assertToolMutable
var AssertToolDeletable = assertToolDeletable
var RequireJSONObject = requireJSONObject
var HasOpenAPIMetadata = hasOpenAPIMetadata
var ToolConfigReady = func(t Tool, platform *WebResearchSetting) bool {
	return toolConfigReady(t, platform, nil)
}

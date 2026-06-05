package tool

var ValidateToolUpsert = validateToolUpsert
var AssertToolMutable = assertToolMutable
var AssertToolDeletable = assertToolDeletable
var RequireJSONObject = requireJSONObject
var HasOpenAPIMetadata = hasOpenAPIMetadata
var CatalogConfigReady = func(t Tool, platform *WebResearchSetting) bool {
	return catalogConfigReady(t, platform, nil)
}

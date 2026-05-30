package tools

var ParseDurationSec = parseDurationSec

var McpTimeoutDuration = mcpTimeoutDuration

var AliasNameOrUnknown = aliasNameOrUnknown

var NewAliasTool = func(name string, inner Tool) *aliasTool {
	return &aliasTool{name: name, inner: inner}
}

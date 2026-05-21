package tools

// ProductionAllowAdHocHTTP gates MCP Broker ad-hoc HTTP: server must opt-in and platform setting must allow.
func ProductionAllowAdHocHTTP(serverConfigured, platformEnabled bool) bool {
	return serverConfigured && platformEnabled
}

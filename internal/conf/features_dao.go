package conf

// DAO session table split feature flags.
// All flags default to false (off). Enable by setting the env var to "1" or "true".

// DAOSessionMetricsTable returns true if the session_metrics table should be used
// for metrics storage instead of the sessions table.
// Env: DAO_SESSION_METRICS_TABLE (default: false)
func DAOSessionMetricsTable() bool {
	return parseBoolFlag("DAO_SESSION_METRICS_TABLE")
}

// DAOSessionRuntimeTable returns true if the session_runtime table should be used
// for runtime state storage instead of the sessions table.
// Env: DAO_SESSION_RUNTIME_TABLE (default: false)
func DAOSessionRuntimeTable() bool {
	return parseBoolFlag("DAO_SESSION_RUNTIME_TABLE")
}

// DAOSessionDualWrite returns true if both old and new tables should be written to.
// Env: DAO_SESSION_DUAL_WRITE (default: false)
func DAOSessionDualWrite() bool {
	return parseBoolFlag("DAO_SESSION_DUAL_WRITE")
}

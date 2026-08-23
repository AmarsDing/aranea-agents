package main

// classifyBackendPort grades a backend-owned port (9900 gRPC / 8802 WS).
// A port held by the already-running healthy backend is informational; held
// by anything else it will block startup and deserves a warning.
func classifyBackendPort(open, backendHealthy bool) (checkLevel, string) {
	switch {
	case open && backendHealthy:
		return checkInfo, "in use by the already-running backend"
	case open:
		return checkWarn, "in use by another process; backend start will fail — free the port or stop the occupant"
	default:
		return checkOK, "available"
	}
}

// classifyBundledPGPort grades 127.0.0.1:5433 when bundled PG mode is active.
// ensurePostgres tolerates an already-open port (our own instance), so this
// is informational only.
func classifyBundledPGPort(open bool) (checkLevel, string) {
	if open {
		return checkInfo, "port already open (bundled PostgreSQL likely running; startup reuses it)"
	}
	return checkOK, "available"
}

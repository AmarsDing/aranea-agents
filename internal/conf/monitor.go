package conf

// ProcessLogEnabled reports whether Gateway process logs (WS EnvelopeTypeLog) are enabled.
// When monitor block is absent, defaults to true.
func (s *Server) ProcessLogEnabled() bool {
	if s == nil || s.Monitor == nil {
		return true
	}
	return s.Monitor.ProcessLogEnabled
}

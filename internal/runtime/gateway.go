package runtime

import trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"

// RunGateway is the shared per-session run control surface for Chat, Team, Cron,
// Channel ingress, and WebSocket cancel/status/enqueue handlers.
type RunGateway interface {
	HasActive(sessionID string) bool
	Cancel(sessionID string) bool
	EnqueueUserMessage(sessionID, content string) (bool, error)
	GetStatus(sessionID string) (RunStatusEntry, bool)
	ActiveRunner(sessionID string) (trpcrunner.Runner, string, bool)
}

var _ RunGateway = (*RunRegistry)(nil)

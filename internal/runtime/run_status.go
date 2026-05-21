package runtime

import (
	"time"

	chatagent "aranea-agents/internal/agent"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// FrameworkRunStatus holds fields aligned with trpc ManagedRunner.RunStatus.
type FrameworkRunStatus struct {
	InvocationID string
	AgentName    string
	StartedAt    time.Time
	LastEventAt  time.Time
	EventCount   int
}

// FrameworkRunStatusFromRunner reads framework run status when requestID matches
// the active trpc run (chat/team use session_id as request_id).
func FrameworkRunStatusFromRunner(runner trpcrunner.Runner, requestID string) (FrameworkRunStatus, bool) {
	st, ok := chatagent.TRPCRunStatus(runner, requestID)
	if !ok {
		return FrameworkRunStatus{}, false
	}
	return FrameworkRunStatus{
		InvocationID: st.InvocationID,
		AgentName:    st.AgentName,
		StartedAt:    st.StartedAt,
		LastEventAt:  st.LastEventAt,
		EventCount:   st.EventCount,
	}, true
}

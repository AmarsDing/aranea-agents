package biz

import (
	"aranea-agents/internal/biz/shared"
)

// AgentNodeEvent is the event type that triggers agent node state transitions.
type AgentNodeEvent string

const (
	AgentNodeEventQueue      AgentNodeEvent = "queue"
	AgentNodeEventSchedule   AgentNodeEvent = "schedule"
	AgentNodeEventStart      AgentNodeEvent = "start"
	AgentNodeEventThink      AgentNodeEvent = "think"
	AgentNodeEventToolStart  AgentNodeEvent = "tool_start"
	AgentNodeEventToolEnd    AgentNodeEvent = "tool_end"
	AgentNodeEventTransfer   AgentNodeEvent = "transfer"
	AgentNodeEventReset      AgentNodeEvent = "reset" // transfer complete, node goes idle
	AgentNodeEventRetry      AgentNodeEvent = "retry"
	AgentNodeEventWaitInput  AgentNodeEvent = "wait_input"
	AgentNodeEventWaitReview AgentNodeEvent = "wait_review"
	AgentNodeEventWaitAssign AgentNodeEvent = "wait_assign"
	AgentNodeEventBlock      AgentNodeEvent = "block"
	AgentNodeEventSucceed    AgentNodeEvent = "succeed"
	AgentNodeEventFail       AgentNodeEvent = "fail"
	AgentNodeEventSkip       AgentNodeEvent = "skip"
	AgentNodeEventCancel     AgentNodeEvent = "cancel"
	AgentNodeEventTimeout    AgentNodeEvent = "timeout"
)

/*
```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> Queued : Queue
    Idle --> Scheduled : Schedule
    Idle --> Running : Start
    Queued --> Scheduled : Schedule
    Queued --> Running : Start
    Queued --> Skipped : Skip
    Queued --> Cancelled : Cancel
    Scheduled --> Running : Start
    Scheduled --> Failed : Fail
    Scheduled --> Cancelled : Cancel
    Running --> Thinking : Think
    Running --> ToolRunning : ToolStart
    Running --> Transferring : Transfer
    Running --> WaitingInput : WaitInput
    Running --> WaitingReview : WaitReview
    Running --> WaitingAssign : WaitAssign
    Running --> Blocked : Block
    Running --> Retrying : Retry
    Running --> Success : Succeed
    Running --> Failed : Fail
    Running --> Cancelled : Cancel
    Running --> TimedOut : Timeout
    Thinking --> ToolRunning : ToolStart
    Thinking --> Transferring : Transfer
    Thinking --> Success : Succeed
    Thinking --> Failed : Fail
    Thinking --> Cancelled : Cancel
    Thinking --> TimedOut : Timeout
    Thinking --> Retrying : Retry
    ToolRunning --> Thinking : ToolEnd
    ToolRunning --> Failed : Fail
    ToolRunning --> Cancelled : Cancel
    ToolRunning --> TimedOut : Timeout
    ToolRunning --> Retrying : Retry
    Transferring --> Running : Start
    Transferring --> Idle : Reset
    Transferring --> Failed : Fail
    Transferring --> Cancelled : Cancel
    Retrying --> Running : Start
    Retrying --> Failed : Fail
    Retrying --> Cancelled : Cancel
    Retrying --> TimedOut : Timeout
    WaitingInput --> Running : Start
    WaitingInput --> Cancelled : Cancel
    WaitingInput --> TimedOut : Timeout
    WaitingReview --> Running : Start
    WaitingReview --> Failed : Fail
    WaitingReview --> Cancelled : Cancel
    WaitingReview --> Blocked : Block
    WaitingAssign --> Running : Start
    WaitingAssign --> Cancelled : Cancel
    WaitingAssign --> Blocked : Block
    Blocked --> Running : Start
    Blocked --> Cancelled : Cancel
    Blocked --> Failed : Fail
    Success --> Retrying : Retry
    Success --> Running : Start
    Failed --> Retrying : Retry
    Failed --> Running : Start
    Failed --> Skipped : Skip
    TimedOut --> Retrying : Retry
    TimedOut --> Running : Start
```
*/

// agentNodeTransitionRules defines all legal state transitions for AgentNodeStatus.
// Terminal states (Success/Failed/Skipped/Cancelled/TimedOut) have limited
// override transitions as defined by canOverrideTerminal.
var agentNodeTransitionRules = []shared.TransitionRule[AgentNodeStatus, AgentNodeEvent]{
	// From Idle — initial state; can transition to any non-terminal operational
	// state (graph tasks may be created with e.g. review_required/blocked status)
	// or to a terminal state directly.
	{From: AgentNodeStatusIdle, Event: AgentNodeEventQueue, To: AgentNodeStatusQueued},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventSchedule, To: AgentNodeStatusScheduled},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventThink, To: AgentNodeStatusThinking},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventWaitInput, To: AgentNodeStatusWaitingInput},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventWaitReview, To: AgentNodeStatusWaitingReview},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventWaitAssign, To: AgentNodeStatusWaitingAssign},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventBlock, To: AgentNodeStatusBlocked},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusIdle, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Queued
	{From: AgentNodeStatusQueued, Event: AgentNodeEventSchedule, To: AgentNodeStatusScheduled},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusQueued, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Scheduled
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusScheduled, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Running
	{From: AgentNodeStatusRunning, Event: AgentNodeEventThink, To: AgentNodeStatusThinking},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventToolStart, To: AgentNodeStatusToolRunning},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventTransfer, To: AgentNodeStatusTransferring},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventWaitInput, To: AgentNodeStatusWaitingInput},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventWaitReview, To: AgentNodeStatusWaitingReview},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventWaitAssign, To: AgentNodeStatusWaitingAssign},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventBlock, To: AgentNodeStatusBlocked},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusRunning, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Thinking
	{From: AgentNodeStatusThinking, Event: AgentNodeEventToolStart, To: AgentNodeStatusToolRunning},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventTransfer, To: AgentNodeStatusTransferring},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusThinking, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From ToolRunning
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventToolEnd, To: AgentNodeStatusThinking},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusToolRunning, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Transferring
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventReset, To: AgentNodeStatusIdle},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusTransferring, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Retrying
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusRetrying, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From WaitingInput
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventBlock, To: AgentNodeStatusBlocked},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusWaitingInput, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From WaitingReview
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventBlock, To: AgentNodeStatusBlocked},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusWaitingReview, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From WaitingAssign
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventBlock, To: AgentNodeStatusBlocked},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusWaitingAssign, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// From Blocked
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventSucceed, To: AgentNodeStatusSuccess},
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventFail, To: AgentNodeStatusFailed},
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventCancel, To: AgentNodeStatusCancelled},
	{From: AgentNodeStatusBlocked, Event: AgentNodeEventTimeout, To: AgentNodeStatusTimedOut},

	// Terminal overrides (canOverrideTerminal logic)
	// Success → Retrying/Running (re-execute after success, e.g., re-run node)
	{From: AgentNodeStatusSuccess, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusSuccess, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	// Failed → Retrying/Running/Skipped (retry, re-run, or mark as skipped)
	{From: AgentNodeStatusFailed, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusFailed, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	{From: AgentNodeStatusFailed, Event: AgentNodeEventSkip, To: AgentNodeStatusSkipped},
	// TimedOut → Retrying/Running (retry after timeout)
	{From: AgentNodeStatusTimedOut, Event: AgentNodeEventRetry, To: AgentNodeStatusRetrying},
	{From: AgentNodeStatusTimedOut, Event: AgentNodeEventStart, To: AgentNodeStatusRunning},
	// Skipped and Cancelled are fully terminal — no overrides
}

// agentNodeStateMachine is the singleton AgentNodeStatus state machine.
var agentNodeStateMachine = shared.NewGenericStateMachine(agentNodeTransitionRules)

// AgentNodeStateMachine returns the singleton AgentNodeStatus state machine.
// Stability:evolving
func AgentNodeStateMachine() shared.StateMachine[AgentNodeStatus, AgentNodeEvent] {
	return agentNodeStateMachine
}

// CanTransitionAgentNodeStatus reports whether a direct transition from one
// AgentNodeStatus to another is valid according to the state machine.
func CanTransitionAgentNodeStatus(from, to AgentNodeStatus) bool {
	return agentNodeStateMachine.CanTransition(from, to)
}

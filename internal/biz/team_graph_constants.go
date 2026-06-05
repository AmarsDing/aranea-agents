package biz

// Team mode constants.
const (
	TeamModeSequential  = "sequential"
	TeamModeParallel    = "parallel"
	TeamModeCoordinator = "coordinator"
	TeamModeCriticLoop  = "critic_loop"
	TeamModeSwarm       = "swarm"
	TeamModeAdaptive    = "adaptive"
)

// Runtime engine constants.
const (
	RuntimeEngineGraph  = "graph"
	RuntimeEngineNative = "native"
)

// Node type constants.
const (
	NodeTypeAgent    = "agent"
	NodeTypeLLM      = "llm"
	NodeTypeTool     = "tool"
	NodeTypeTools    = "tools"
	NodeTypeTask     = "task"
	NodeTypeReview   = "review"
	NodeTypeRouter   = "router"
	NodeTypeJoin     = "join"
	NodeTypeFunction = "function"
	NodeTypeStart    = "start"
	NodeTypeEnd      = "end"
	NodeTypeSubgraph = "subgraph"
)

// Edge kind constants.
const (
	EdgeKindFlow     = "flow"
	EdgeKindTransfer = "transfer"
	EdgeKindDispatch = "dispatch"
	EdgeKindApprove  = "approve"
	EdgeKindReject   = "reject"
	EdgeKindRetry    = "retry"
	EdgeKindFallback = "fallback"
	EdgeKindHandoff  = "handoff"
	EdgeKindDelegate = "delegate"
)

// Failure policy constants.
const (
	FailurePolicyRetryThenBlock = "retry_then_block"
	FailurePolicySkip           = "skip"
	FailurePolicyFailFast       = "fail_fast"
	FailurePolicySkipOnFailure  = "skip_on_failure"
	FailurePolicyContinue       = "continue"
	FailurePolicyAbort          = "abort"
	FailurePolicyOrchSkip       = "orchestration.skip"
	FailurePolicyAwaitReview    = "await_review"
	FailurePolicyHalt           = "halt"

	SkippedNodesKey = "_skipped_nodes"
	SkippedNodeKey  = "_skipped_node"
)

// Legacy failure policy constant aliases — kept for backward compatibility with existing references.
const (
	FailureDefaultRetryThenBlock = FailurePolicyRetryThenBlock
	FailureDefaultSkip           = FailurePolicySkip
	FailureDefaultFailFast       = FailurePolicyFailFast
	FailureOnFailureSkip         = FailurePolicySkipOnFailure
	SkipNodeFuncRef              = FailurePolicyOrchSkip
	SkippedNodesStateKey         = SkippedNodesKey
	SkippedNodeOutputKey         = SkippedNodeKey
	ParallelFailContinue         = FailurePolicyContinue
	ParallelFailAbort            = FailurePolicyAbort
)

// Execution engine constants.
const (
	ExecEngineBSP = "bsp"
	ExecEngineDAG = "dag"
)

// Compile template ID constants.
const (
	CompileTemplatePipeline       = "pipeline"
	CompileTemplateParallelReview = "parallel_review"
	CompileTemplateDispatch       = "dispatch"
	CompileTemplateReviewLoop     = "review_loop"
)

// Graph ID prefix.
const (
	GraphIDTeamPrefix = "team:"
)

// Conditional function references.
const (
	CriticLoopDecisionFunc = "critic_loop_decision"
)

// Role name constants.
const (
	RoleWorker      = "worker"
	RoleSynthesizer = "synthesizer"
	RoleCoordinator = "coordinator"
	RoleGenerator   = "generator"
	RoleCritic      = "critic"
)

// Assignment mode constants.
const (
	AssignmentModeStatic = "static"
)

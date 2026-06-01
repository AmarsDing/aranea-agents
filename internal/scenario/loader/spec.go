package loader

type IndustrySpec struct {
	IndustryKey   string       `yaml:"industry_key"`
	Defaults      AgentDefaults `yaml:"defaults"`
	Agents        []AgentSpec  `yaml:"agents"`
	Teams         []TeamSpec   `yaml:"teams"`
}

type AgentDefaults struct {
	Provider         string   `yaml:"provider"`
	FastModel        string   `yaml:"fast_model"`
	StrongModel      string   `yaml:"strong_model"`
	ToolsDeny        []string `yaml:"tools_deny"`
	SystemPromptMode string   `yaml:"system_prompt_mode"`
	ContextWindow    int      `yaml:"context_window"`
	CodeExecutor     string   `yaml:"code_executor"`
}

type AgentSpec struct {
	Key             string   `yaml:"key"`
	PositionKey     string   `yaml:"position_key"`
	Variant         string   `yaml:"variant"`
	DisplayName     string   `yaml:"display_name"`
	Description     string   `yaml:"description"`
	ModelTier       string   `yaml:"model_tier"`
	ToolsProfile    string   `yaml:"tools_profile"`
	ToolsAllow      []string `yaml:"tools_allow"`
	ToolsDeny       []string `yaml:"tools_deny"`
	Skills          []string `yaml:"skills"`
	ContextWindow   int      `yaml:"context_window"`
	SubagentsEnabled *bool   `yaml:"subagents_enabled"`
	ToolsParallel   *bool   `yaml:"tools_parallel"`
	CodeExecutor    string   `yaml:"code_executor"`
	MaxOutputTokens int      `yaml:"max_output_tokens"`
	SystemPromptMode string  `yaml:"system_prompt_mode"`
	RoleKey         string   `yaml:"role_key"`
	TeamRole        string   `yaml:"team_role"`
}

type TeamSpec struct {
	Key            string        `yaml:"key"`
	DisplayName    string        `yaml:"display_name"`
	Mode           string        `yaml:"mode"`
	Description    string        `yaml:"description"`
	MaxConcurrency int           `yaml:"max_concurrency"`
	TimeoutSeconds int           `yaml:"timeout_seconds"`
	LoopMaxIter    int           `yaml:"loop_max_iterations"`
	EnableCheckpoint bool        `yaml:"enable_checkpoint"`
	IntentAnchorKey string       `yaml:"intent_anchor_key"`
	SynthesizerKey  string       `yaml:"synthesizer_key"`
	CriticLoop     *CriticLoopSpec `yaml:"critic_loop"`
	Members        []TeamMemberSpec `yaml:"members"`
	Graph          *GraphSpec    `yaml:"graph"`
}

type TeamMemberSpec struct {
	AgentKey   string `yaml:"agent_key"`
	Role       string `yaml:"role"`
	Name       string `yaml:"name"`
	SortOrder  int    `yaml:"sort_order"`
	TaskPrompt string `yaml:"task_prompt"`
}

type CriticLoopSpec struct {
	MaxIterations  int     `yaml:"max_iterations"`
	ScoreThreshold float64 `yaml:"score_threshold"`
}

type GraphSpec struct {
	Layout string         `yaml:"layout"`
	Nodes  []GraphNodeSpec `yaml:"nodes"`
	Edges  []GraphEdgeSpec `yaml:"edges"`
}

type GraphNodeSpec struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Label   string `yaml:"label"`
	AgentKey string `yaml:"agent_key"`
	Role    string `yaml:"role"`
}

type GraphEdgeSpec struct {
	ID     string `yaml:"id"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

package biz

// Team is a row in legacy teams.
type Team struct {
	ID             string
	TeamKey        string
	DisplayName    string
	Status         string
	IsDefault      bool
	DefinitionJSON string
	ADKAppName     string
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

// TeamRun is a row in team_runs.
type TeamRun struct {
	ID            string `json:"id"`
	TeamID        string `json:"team_id"`
	SessionID     string `json:"session_id"`
	MessageID     string `json:"message_id"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	TopologyJSON  string `json:"topology_json"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// TeamRunStep is a row in team_run_steps.
type TeamRunStep struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	TeamID        string `json:"team_id"`
	AgentID       string `json:"agent_id"`
	AgentKey      string `json:"agent_key"`
	AgentName     string `json:"agent_name"`
	Role          string `json:"role"`
	SortOrder     int    `json:"sort_order"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
}

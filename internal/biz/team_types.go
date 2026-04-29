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
	ID            string
	TeamID        string
	SessionID     string
	MessageID     string
	Mode          string
	Status        string
	InputPreview  string
	OutputPreview string
	TokenIn       int
	TokenOut      int
	CostMicroUSD  int64
	DurationMS    int
	ErrorMessage  string
	TopologyJSON  string
	StartedAt     string
	FinishedAt    string
	CreatedAt     string
	UpdatedAt     string
}

// TeamRunStep is a row in team_run_steps.
type TeamRunStep struct {
	ID            string
	RunID         string
	TeamID        string
	AgentID       string
	AgentKey      string
	AgentName     string
	Role          string
	SortOrder     int
	Status        string
	InputPreview  string
	OutputPreview string
	TokenIn       int
	TokenOut      int
	CostMicroUSD  int64
	DurationMS    int
	ErrorMessage  string
	StartedAt     string
	FinishedAt    string
	CreatedAt     string
}

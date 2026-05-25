package modelcatalog

import "encoding/json"

// Catalog is the top-level models.dev api.json shape: map of provider id -> provider.
type Catalog map[string]Provider

type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Env    []string         `json:"env"`
	Npm    string           `json:"npm"`
	API    string           `json:"api,omitempty"`
	Doc    string           `json:"doc"`
	Models map[string]Model `json:"models"`
}

type Model struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Family           string       `json:"family,omitempty"`
	Attachment       bool         `json:"attachment"`
	Reasoning        bool         `json:"reasoning"`
	ToolCall         bool         `json:"tool_call"`
	StructuredOutput *bool        `json:"structured_output,omitempty"`
	Temperature      *bool        `json:"temperature,omitempty"`
	Knowledge        string       `json:"knowledge,omitempty"`
	ReleaseDate      string       `json:"release_date"`
	LastUpdated      string       `json:"last_updated"`
	OpenWeights      bool             `json:"open_weights"`
	Interleaved      json.RawMessage  `json:"interleaved,omitempty"`
	Status           string           `json:"status,omitempty"`
	Cost             *ModelCost   `json:"cost,omitempty"`
	Limit            ModelLimit   `json:"limit"`
	Modalities       Modalities   `json:"modalities"`
}

type ModelCost struct {
	Input        float64 `json:"input,omitempty"`
	Output       float64 `json:"output,omitempty"`
	Reasoning    float64 `json:"reasoning,omitempty"`
	CacheRead    float64 `json:"cache_read,omitempty"`
	CacheWrite   float64 `json:"cache_write,omitempty"`
	InputAudio   float64 `json:"input_audio,omitempty"`
	OutputAudio  float64 `json:"output_audio,omitempty"`
}

type ModelLimit struct {
	Context int64 `json:"context"`
	Input   int64 `json:"input,omitempty"`
	Output  int64 `json:"output"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type Meta struct {
	SyncedAt      string `json:"synced_at"`
	ETag          string `json:"etag,omitempty"`
	SHA256        string `json:"sha256"`
	SourceURL     string `json:"source_url"`
	ProviderCount int    `json:"provider_count"`
	ModelCount    int    `json:"model_count"`
	Bytes         int64  `json:"bytes"`
}

// Policy holds sync configuration persisted locally.
type Policy struct {
	SourceURL         string `json:"source_url"`
	SyncPolicy        string `json:"sync_policy"`
	SyncIntervalHours int    `json:"sync_interval_hours"`
	AutoApply         string `json:"auto_apply"`
}

func DefaultPolicy() Policy {
	return Policy{
		SourceURL:         "https://models.dev/api.json",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 24,
		AutoApply:         "metadata_and_pricing",
	}
}

type SyncStats struct {
	Providers          int `json:"providers"`
	Models             int `json:"models"`
	LogosSynced        int `json:"logos_synced,omitempty"`
	LogosFailed        int `json:"logos_failed,omitempty"`
	LogosRemoved       int `json:"logos_removed,omitempty"`
	DeprecatedDisabled int `json:"deprecated_disabled,omitempty"`
	LLMRowsUpdated     int `json:"llm_rows_updated,omitempty"`
	AgentsUpdated      int `json:"agents_updated,omitempty"`
}

type SyncLogEntry struct {
	ID          string     `json:"id"`
	StartedAt   string     `json:"started_at"`
	FinishedAt  string     `json:"finished_at"`
	Status      string     `json:"status"`
	Message     string     `json:"message,omitempty"`
	SourceURL   string     `json:"source_url"`
	ETag        string     `json:"etag,omitempty"`
	DryRun      bool       `json:"dry_run,omitempty"`
	Stats       SyncStats  `json:"stats"`
	Errors      []string   `json:"errors,omitempty"`
}

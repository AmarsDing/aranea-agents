package biz

type Industry struct {
	ID          string
	Key         string
	Name        string
	Icon        string
	Description string
	ScenarioKey string
	Enabled     bool
	SortOrder   int
	CreatedAt   string
	UpdatedAt   string
	DeletedAt   string
}

type IndustryListQuery struct {
	Enabled *bool
}

type IndustryListResult struct {
	Items []Industry
	Total int
}

package biz

type Position struct {
	ID                   string
	Key                  string
	Name                 string
	DepartmentKey        string
	Description          string
	ResponsibilitiesJSON string
	SkillsRequired       []string
	SeniorityLevel       string
	SortOrder            int
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
}

type PositionListQuery struct {
	DepartmentKey string
	IndustryKey   string
}

type PositionListResult struct {
	Items []Position
	Total int
}

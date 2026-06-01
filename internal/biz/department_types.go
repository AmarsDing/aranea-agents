package biz

type Department struct {
	ID                   string
	Key                  string
	Name                 string
	IndustryKey          string
	Description          string
	ResponsibilitiesJSON string
	SortOrder            int
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
}

type DepartmentListQuery struct {
	IndustryKey string
}

type DepartmentListResult struct {
	Items []Department
	Total int
}

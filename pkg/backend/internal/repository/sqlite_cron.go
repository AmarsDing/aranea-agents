package repository

import (
	"arenea/backend/internal/domain"
	opsql "arenea/backend/internal/operations/adapters/sqlite"
)

func (r *SQLiteRepository) operationsSQL() *opsql.Store {
	return opsql.NewStore(r.db)
}

func (r *SQLiteRepository) ListCronTaskRuns(query domain.CronTaskRunQuery) ([]domain.CronTaskRun, error) {
	return r.operationsSQL().ListCronTaskRuns(query)
}

func (r *SQLiteRepository) AddCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error) {
	return r.operationsSQL().AddCronTaskRun(run)
}

func (r *SQLiteRepository) UpdateCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error) {
	return r.operationsSQL().UpdateCronTaskRun(run)
}

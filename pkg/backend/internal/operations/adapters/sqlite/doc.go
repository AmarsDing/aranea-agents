// Package sqlite implements Operations-related persistence for the shared
// monolithic schema (e.g. cron_task_run, cron_task; migration #31).
// Store uses the same *sql.DB as repository.SQLiteRepository.
package sqlite

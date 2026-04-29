package sqlite

import "time"

// scanner 抽象 *sql.Row 与 *sql.Rows，与 repository/sqlite_helpers 同形。
type scanner interface {
	Scan(dest ...any) error
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

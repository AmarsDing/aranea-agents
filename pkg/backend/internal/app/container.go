package app

import (
	"database/sql"
	"log/slog"
)

// Container holds the singleton infrastructure dependencies passed to every
// Context module during construction.
//
// Skeleton state (P0): only the canonical fields are declared; concrete
// wiring (event bus, telemetry, clock) lands as those subsystems migrate.
type Container struct {
	DB     *sql.DB
	Logger *slog.Logger
}

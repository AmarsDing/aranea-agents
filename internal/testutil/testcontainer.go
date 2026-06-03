package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PostgresContainer wraps a testcontainers PostgreSQL container for integration tests.
// Usage:
//
//	ctx := context.Background()
//	pg, cleanup, err := testutil.StartPostgres(ctx, t)
//	defer cleanup()
//	dsn := pg.DSN()
func StartPostgres(ctx context.Context, t *testing.T) (*PostgresContainer, func(), error) {
	t.Helper()

	c, err := postgres.Run(ctx,
		"pgvector/pgvector:pg16",
		postgres.WithDatabase("aranea_test"),
		postgres.WithUsername("aranea"),
		postgres.WithPassword("aranea_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	cleanup := func() {
		if err := c.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	}

	host, err := c.Host(ctx)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to get container port: %w", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=aranea password=aranea_test dbname=aranea_test sslmode=disable", host, port.Port())

	return &PostgresContainer{
		Container: c,
		DSNValue:  dsn,
	}, cleanup, nil
}

// PostgresContainer holds the testcontainer and connection info.
type PostgresContainer struct {
	Container *postgres.PostgresContainer
	DSNValue  string
}

// DSN returns the PostgreSQL connection string for the test container.
func (p *PostgresContainer) DSN() string {
	return p.DSNValue
}

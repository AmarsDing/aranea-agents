//go:build !pgvector

package vector

import (
	"database/sql"
	"fmt"

	"aranea-agents/pkg/loggateway"
)

// NewPgVectorStore returns an error when pgvector build tag is not enabled.
func NewPgVectorStore(db *sql.DB, tableName string, dim int, lg loggateway.Logger) (VectorStore, error) {
	return nil, fmt.Errorf("pgvector store requires build tag 'pgvector': %w", sql.ErrConnDone)
}

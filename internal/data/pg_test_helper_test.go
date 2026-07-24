package data

import (
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newTestDataPG builds a *Data backed by an isolated Postgres test schema
// (see testhelper.SetupTestPG). Postgres is the only supported primary
// database; SQLite-based test helpers were removed.
func newTestDataPG(t *testing.T) *Data {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return d
}

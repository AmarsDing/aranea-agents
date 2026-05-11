package data

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/internal/data/sessionmemory"

	"entgo.io/ent/dialect"

	_ "github.com/glebarez/go-sqlite/compat"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewAdminRepo,
	NewAvatarRepo,
	NewMemoryRepo,
	NewAgentRepo,
	NewTeamRepo,
	NewAgentCategoryRepo,
	NewLlmProviderModelRepo,
	NewHookRepo,
	NewCronRepo,
	NewPluginRepo,
	NewMCPServerRepo,
	NewSkillRepo,
	NewSessionRepo,
	NewToolRepo,
	NewChannelRepo,
	NewChannelPeerSessionRepo,
	NewUsageRepo,
	NewMonitorRepo,
	NewSystemSettingRepo,
	NewSessionMemoryStore,
)

// Data: Ent/SQLite holds app CRUD; Postgres (optional) holds pgvector agent memory only.
// 复杂原生 SQL 走 *ent.Client 上的 QueryContext（见 sqlite_db.go），不另开 sql.DB。
type Data struct {
	entClient *ent.Client // SQLite — Ent schema（admin / avatar_assets / embedding 偏好等）
	pg        *sql.DB     // Postgres — agent_memory 向量列
	vectorDim int
}

// Ent returns the SQLite-backed Ent client.
func (d *Data) Ent() *ent.Client {
	if d == nil {
		return nil
	}
	return d.entClient
}

// Postgres returns the Postgres DB handle for vectors, or nil if not configured.
func (d *Data) Postgres() *sql.DB {
	if d == nil {
		return nil
	}
	return d.pg
}

// VectorDim returns configured embedding dimension for agent memory inserts / search.
func (d *Data) VectorDim() int {
	if d == nil || d.vectorDim <= 0 {
		return defaultVectorDim
	}
	return d.vectorDim
}

const defaultVectorDim = 1536

func vectorDimFromConf(c *conf.Data) int {
	dim := defaultVectorDim
	if c != nil && c.GetPostgres() != nil {
		if v := int(c.GetPostgres().GetVectorDim()); v > 0 {
			dim = v
		}
	}
	return dim
}

// entSQLiteDriverAndDSN resolves Ent's SQLite location: explicit data.sqlite, or legacy data.database.driver=sqlite3.
func entSQLiteDriverAndDSN(c *conf.Data) (driverName, dsn string, err error) {
	if c == nil {
		return "", "", fmt.Errorf("data config is nil")
	}
	if c.GetSqlite() != nil && c.GetSqlite().GetEnable() {
		src := strings.TrimSpace(c.GetSqlite().GetSource())
		if src == "" {
			return "", "", fmt.Errorf("data.sqlite.enabled but source is empty")
		}
		return dialect.SQLite, src, nil
	}
	if db := c.GetDatabase(); db != nil {
		drv := strings.TrimSpace(strings.ToLower(db.GetDriver()))
		src := strings.TrimSpace(db.GetSource())
		if drv == dialect.SQLite && src != "" {
			return dialect.SQLite, src, nil
		}
	}
	return "", "", fmt.Errorf(`local CRUD expects SQLite: set data.sqlite.enable=true with source, or data.database.driver=sqlite3`)
}

func postgresVectorDSN(c *conf.Data) string {
	if c == nil || c.GetPostgres() == nil {
		return ""
	}
	return strings.TrimSpace(c.GetPostgres().GetSource())
}

func migrateDev(ctx context.Context, client *ent.Client, label string) (*ent.Client, error) {
	client = client.Debug()
	if err := client.Schema.Create(ctx, migrate.WithDropIndex(true)); err != nil {
		return nil, fmt.Errorf("%s migrate: %w", label, err)
	}
	return client, nil
}

// NewData opens SQLite for Ent CRUD; optionally opens Postgres + ensures pgvector schema for agent memory.
func NewData(c *conf.Data) (*Data, func(), error) {
	driverName, dsn, err := entSQLiteDriverAndDSN(c)
	if err != nil {
		log.Fatalf("sqlite (ent): %v", err)
	}
	entClient, err := ent.Open(driverName, dsn)
	if err != nil {
		log.Fatalf("failed opening sqlite for ent: %v", err)
	}
	if driverName == dialect.SQLite {
		if _, pragmaErr := entClient.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); pragmaErr != nil {
			_ = entClient.Close()
			return nil, nil, fmt.Errorf("sqlite foreign_keys pragma: %w", pragmaErr)
		}
	}

	ctxEnt := context.Background()
	if strings.TrimSpace(os.Getenv("DEPLOY_ENV")) == "dev" {
		entClient, err = migrateDev(ctxEnt, entClient, "sqlite(ent)")
		if err != nil {
			_ = entClient.Close()
			return nil, nil, err
		}
	} else if err = entClient.Schema.Create(ctxEnt); err != nil {
		_ = entClient.Close()
		return nil, nil, fmt.Errorf("ent schema create (sqlite): %w", err)
	}

	if err = sessionmemory.EnsureSchema(context.Background(), entClient); err != nil {
		_ = entClient.Close()
		return nil, nil, fmt.Errorf("session memory schema: %w", err)
	}
	if err = ensureAgentRuntimePatches(context.Background(), entClient); err != nil {
		_ = entClient.Close()
		return nil, nil, fmt.Errorf("agent runtime patches: %w", err)
	}
	if err = ensureBuiltinPlatformTools(context.Background(), entClient); err != nil {
		_ = entClient.Close()
		return nil, nil, err
	}
	if err = ensureDefaultSystemSetting(context.Background(), entClient); err != nil {
		_ = entClient.Close()
		return nil, nil, err
	}

	pgDSN := postgresVectorDSN(c)
	var pg *sql.DB
	if pgDSN != "" {
		pg, err = sql.Open("postgres", pgDSN)
		if err != nil {
			_ = entClient.Close()
			log.Fatalf("failed opening postgres for vectors: %v", err)
		}
		pg.SetMaxOpenConns(8)
		pg.SetConnMaxLifetime(0)
		if err = pg.PingContext(context.Background()); err != nil {
			pg.Close()
			_ = entClient.Close()
			return nil, nil, fmt.Errorf("postgres ping: %w", err)
		}
	}

	vdim := vectorDimFromConf(c)
	st := &Data{entClient: entClient, pg: pg, vectorDim: vdim}

	if pg != nil {
		if err = pgvector.EnsureSchema(context.Background(), pg, vdim); err != nil {
			pg.Close()
			entClient.Close()
			return nil, nil, err
		}
	}

	if err = ensureInitialAdminFromConfig(context.Background(), entClient, c); err != nil {
		if pg != nil {
			pg.Close()
		}
		_ = entClient.Close()
		return nil, nil, err
	}
	if err = ensureDevBypassAdminIfEnabled(context.Background(), entClient); err != nil {
		if pg != nil {
			pg.Close()
		}
		_ = entClient.Close()
		return nil, nil, err
	}

	cleanup := func() {
		if st.pg != nil {
			st.pg.Close()
		}
		if st.entClient != nil {
			st.entClient.Close()
		}
	}
	return st, cleanup, nil
}

// NewSessionMemoryStore exposes SQLite session-chain reads (L0–L4, evolution) on the same DB as Ent.
func NewSessionMemoryStore(d *Data) *sessionmemory.Store {
	if d == nil {
		return nil
	}
	return sessionmemory.NewStore(d.Ent())
}

package data

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/internal/data/sessionmemory"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

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
	NewPluginRunRepo,
	NewHookDeliveryRepo,
	NewPluginCostGuardUsageRepo,
	NewMCPServerRepo,
	NewSkillRepo,
	NewSessionRepo,
	NewToolRepo,
	NewChannelRepo,
	NewChannelPeerSessionRepo,
	NewChannelInboundReceiptRepo,
	NewChannelTurnJobRepo,
	NewSessionRunRepo,
	NewSessionRunCheckpointRepo,
	NewSessionParticipantRepo,
	NewUsageRepo,
	NewMonitorRepo,
	NewSystemSettingRepo,
	NewSessionMemoryStore,
	NewEvolutionMetricsRepo,
	NewEvolutionSuggestionRepo,
	NewGraphRepo,
	NewGraphRunRepo,
	NewTaskRepo,
	ProvideTaskLinkRepo,
	NewArtifactRepo,
	NewKnowledgeRepoFromData,
	NewEvalRepoFromData,
	NewA2ARepoFromData,
	NewEcosystemRepo,
	NewEventStoreRepo,
	NewFlowLogRepo,
	NewWebhookRepo,
)

// Data: Ent/SQLite holds app CRUD; Postgres (optional) holds pgvector agent memory only.
// 复杂原生 SQL 走 *ent.Client 上的 QueryContext（见 sqlite_db.go），不另开 sql.DB。
type Data struct {
	entClient *ent.Client // SQLite — Ent schema（admin / avatar_assets / embedding 偏好等）
	rawDB     *sql.DB     // SQLite — 底层 *sql.DB，Ent 与 trpc 适配器共用同一连接池
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

// RawDB returns the underlying SQLite *sql.DB shared by Ent and trpc adapters (session / checkpoint).
func (d *Data) RawDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.rawDB
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
	// Do not enable ent.Debug() by default: it logs every SQL line synchronously to stdout.
	// With a small SQLite pool that can block other handlers (e.g. POST /v1/admins/login) on Windows.
	if entSQLDebugEnabled() {
		client = client.Debug()
	}
	if err := client.Schema.Create(ctx, migrate.WithDropIndex(true)); err != nil {
		return nil, fmt.Errorf("%s migrate: %w", label, err)
	}
	return client, nil
}

func entSQLDebugEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ARANEA_ENT_SQL_DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

// NewData opens SQLite for Ent CRUD; optionally opens Postgres + ensures pgvector schema for agent memory.
// The underlying *sql.DB is shared with trpc session/checkpoint adapters via RawDB().
func NewData(c *conf.Data) (*Data, func(), error) {
	var entClient *ent.Client
	var rawDB *sql.DB
	var pg *sql.DB
	var pgOpened bool

	cleanup := func() {
		if entClient != nil {
			_ = entClient.Close()
		}
		if pgOpened && pg != nil {
			pg.Close()
		}
		if rawDB != nil {
			rawDB.Close()
		}
	}

	if err := runStartupStep("initSQLite", func() error {
		var stepErr error
		entClient, rawDB, stepErr = initSQLite(c)
		return stepErr
	}); err != nil {
		return nil, nil, err
	}

	if err := runStartupStep("ensureSchemaDDL", func() error {
		return ensureSchemaDDL(rawDB, entClient)
	}); err != nil {
		cleanup()
		return nil, nil, err
	}

	if err := runStartupStep("initPostgres", func() error {
		var stepErr error
		pg, stepErr = initPostgres(c)
		pgOpened = true
		return stepErr
	}); err != nil {
		cleanup()
		return nil, nil, err
	}

	vdim := vectorDimFromConf(c)
	st := &Data{entClient: entClient, rawDB: rawDB, pg: pg, vectorDim: vdim}

	if err := runStartupStep("ensurePostgresSchemas", func() error {
		return ensurePostgresSchemas(pg, vdim)
	}); err != nil {
		cleanup()
		return nil, nil, err
	}

	if err := runStartupStep("seedInitialData", func() error {
		return seedInitialData(entClient, c)
	}); err != nil {
		cleanup()
		return nil, nil, err
	}

	return st, cleanup, nil
}

var startupLog = log.New(os.Stdout, "", log.LstdFlags)

func runStartupStep(name string, fn func() error) error {
	start := time.Now()
	err := fn()
	if err != nil {
		return err
	}
	startupLog.Printf("[startup] %s done in %s", name, time.Since(start).Round(time.Millisecond))
	return nil
}

// initSQLite opens the SQLite database, configures Ent, applies PRAGMAs,
// and runs migration.
func initSQLite(c *conf.Data) (*ent.Client, *sql.DB, error) {
	driverName, dsn, err := entSQLiteDriverAndDSN(c)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite (ent): %w", err)
	}
	if err := ensureSQLiteParentDir(dsn); err != nil {
		return nil, nil, err
	}

	rawDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed opening sqlite raw db: %w", err)
	}
	rawDB.SetMaxOpenConns(4)

	drv := entsql.OpenDB(driverName, rawDB)
	entClient := ent.NewClient(ent.Driver(drv))

	if driverName == dialect.SQLite {
		for _, pragma := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=10000",
		} {
			if _, pragmaErr := rawDB.ExecContext(context.Background(), pragma); pragmaErr != nil {
				rawDB.Close()
				return nil, nil, fmt.Errorf("sqlite %s: %w", pragma, pragmaErr)
			}
		}
	}

	ctxEnt := context.Background()
	if strings.TrimSpace(os.Getenv("DEPLOY_ENV")) == "dev" {
		entClient, err = migrateDev(ctxEnt, entClient, "sqlite(ent)")
		if err != nil {
			rawDB.Close()
			return nil, nil, err
		}
	} else if err = entClient.Schema.Create(ctxEnt); err != nil {
		rawDB.Close()
		return nil, nil, fmt.Errorf("ent schema create (sqlite): %w", err)
	}
	return entClient, rawDB, nil
}

// ensureSchemaDDL applies Ent and raw SQL schema patches (DDL only; no data migrations).
func ensureSchemaDDL(rawDB *sql.DB, entClient *ent.Client) error {
	if err := sessionmemory.EnsurePatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("session memory patches: %w", err)
	}
	if err := EnsureSessionMemorySchema(context.Background(), entClient); err != nil {
		return fmt.Errorf("session memory schema: %w", err)
	}
	if err := sessionmemory.EnsureMemoryRelationPatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("memory relation patches: %w", err)
	}
	if err := sessionmemory.EnsureMonitorSchemaPatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("monitor schema patches: %w", err)
	}
	if err := ensureAgentRuntimePatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("agent runtime patches: %w", err)
	}
	if err := ensureBuiltinPlatformTools(context.Background(), entClient); err != nil {
		return err
	}
	if err := ensureSystemSettingPatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("system setting patches: %w", err)
	}
	if err := ensurePricingRulePatches(context.Background(), entClient); err != nil {
		return fmt.Errorf("pricing rule patches: %w", err)
	}
	if err := ensureDefaultSystemSetting(context.Background(), entClient); err != nil {
		return err
	}
	if err := ensureDefaultCredentialEncryptionKey(context.Background(), entClient); err != nil {
		return fmt.Errorf("credential encryption key: %w", err)
	}
	ctxSchema := context.Background()
	if err := EnsureEvalSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("eval schema: %w", err)
	}
	if err := EnsureA2ASchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("a2a schema: %w", err)
	}
	if err := ensureA2ARemoteHealthPatches(ctxSchema, entClient); err != nil {
		return fmt.Errorf("a2a remote health patches: %w", err)
	}
	if err := ensureTeamRunSummaryPatches(ctxSchema, entClient); err != nil {
		return fmt.Errorf("team run summary patches: %w", err)
	}
	if err := ensureSessionRevisionPatches(ctxSchema, entClient); err != nil {
		return fmt.Errorf("session revision patches: %w", err)
	}
	if err := EnsurePluginRunSchema(ctxSchema, entClient); err != nil {
		return fmt.Errorf("plugin run schema: %w", err)
	}
	if err := EnsureHookDeliverySchema(ctxSchema, entClient); err != nil {
		return fmt.Errorf("hook delivery schema: %w", err)
	}
	if err := EnsureFlowLogSchema(ctxSchema, entClient); err != nil {
		return fmt.Errorf("flow log schema: %w", err)
	}
	if err := EnsureMessageFTSSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("message fts schema: %w", err)
	}
	if err := EnsureChannelInboundSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("channel inbound schema: %w", err)
	}
	if err := EnsureChannelTurnJobSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("channel turn job schema: %w", err)
	}
	if err := EnsureSessionRunSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("session run schema: %w", err)
	}
	if err := EnsureSessionParticipantSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("session participant schema: %w", err)
	}
	if err := ensureSessionRunCheckpointSchema(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("session run checkpoint schema: %w", err)
	}
	if err := ensureSessionRunColumnPatches(ctxSchema, rawDB); err != nil {
		return fmt.Errorf("session run column patches: %w", err)
	}
	if err := EnsureMonitorAlertSchema(ctxSchema, entClient); err != nil {
		return fmt.Errorf("monitor alert schema: %w", err)
	}
	if err := EnsureEcosystemSchema(ctxSchema, entClient); err != nil {
		return fmt.Errorf("ecosystem schema: %w", err)
	}
	return nil
}

// runPendingDataMigrations applies one-time data migrations (schema_migrations gate).
func runPendingDataMigrations(entClient *ent.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrated, skipped, err := RunLegacyTRPCMemoryMigration(ctx, sessionmemory.NewStore(entClient))
	if err != nil {
		return fmt.Errorf("legacy trpc memory backfill: %w", err)
	}
	if skipped {
		startupLog.Printf("[startup] legacy trpc memory backfill skipped (migration %d applied)", MigrationLegacyTRPCMemoryFacts)
	} else if migrated > 0 {
		startupLog.Printf("[startup] legacy trpc memory backfill migrated=%d", migrated)
	}
	return nil
}

// ensureAllSchemas applies DDL patches and pending data migrations (compat wrapper for tests).
func ensureAllSchemas(rawDB *sql.DB, entClient *ent.Client) error {
	if err := ensureSchemaDDL(rawDB, entClient); err != nil {
		return err
	}
	return runPendingDataMigrations(entClient)
}

// initPostgres opens the optional Postgres vector store connection.
func initPostgres(c *conf.Data) (*sql.DB, error) {
	pgDSN := postgresVectorDSN(c)
	if pgDSN == "" {
		return nil, nil
	}
	pg, err := sql.Open("postgres", pgDSN)
	if err != nil {
		return nil, fmt.Errorf("failed opening postgres for vectors: %w", err)
	}
	pg.SetMaxOpenConns(8)
	pg.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = pg.PingContext(ctx); err != nil {
		pg.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return pg, nil
}

// ensurePostgresSchemas applies vector and knowledge schema on Postgres if configured.
func ensurePostgresSchemas(pg *sql.DB, vdim int) error {
	if pg == nil {
		return nil
	}
	ctxPG := context.Background()
	if err := pgvector.EnsureSchema(ctxPG, pg, vdim); err != nil {
		return err
	}
	if err := EnsureKnowledgeSchema(ctxPG, pg, vdim); err != nil {
		return fmt.Errorf("knowledge schema: %w", err)
	}
	return nil
}

// seedInitialData seeds initial admin from config and optional dev bypass admin.
func seedInitialData(entClient *ent.Client, c *conf.Data) error {
	if err := ensureInitialAdminFromConfig(context.Background(), entClient, c); err != nil {
		return err
	}
	if err := ensureDevBypassAdminIfEnabled(context.Background(), entClient); err != nil {
		return err
	}
	if err := ensureChannelPlatformAvatars(context.Background(), entClient); err != nil {
		return err
	}
	return nil
}

// NewSessionMemoryStore exposes SQLite session-chain reads (L0–L4, evolution) on the same DB as Ent.
func NewSessionMemoryStore(d *Data) *sessionmemory.Store {
	if d == nil {
		return nil
	}
	return sessionmemory.NewStore(d.Ent())
}

func NewArtifactRepo() biz.ArtifactRepo {
	return artifactfs.NewFSArtifactRepo()
}

func NewKnowledgeRepoFromData(d *Data) biz.KnowledgeRepo {
	if d == nil || d.Postgres() == nil {
		return nil
	}
	return NewKnowledgeRepo(d.Postgres())
}

func NewEvalRepoFromData(d *Data) biz.EvalRepo {
	if d == nil || d.RawDB() == nil {
		return nil
	}
	return NewEvalRepo(d.RawDB())
}

func NewA2ARepoFromData(d *Data) biz.A2ARepo {
	if d == nil || d.RawDB() == nil {
		return nil
	}
	return NewA2ARepo(d.RawDB())
}

// NewCLIData wraps SQLite handles opened by OpenSQLiteEntClient for offline maintenance CLIs.
func NewCLIData(client *ent.Client, rawDB *sql.DB) *Data {
	return &Data{entClient: client, rawDB: rawDB}
}

// OpenSQLiteEntClient opens SQLite for offline CLI maintenance tools (e.g. memory-migrate).
// Do not use against a DSN while admin is running — use in-process NewData migrations instead.
func OpenSQLiteEntClient(dsn string) (*ent.Client, *sql.DB, func(), error) {
	dsn = normalizeSQLiteDSN(dsn)
	rawDB, err := sql.Open(dialect.SQLite, dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	rawDB.SetMaxOpenConns(4)
	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=10000",
	} {
		if _, err := rawDB.ExecContext(context.Background(), pragma); err != nil {
			rawDB.Close()
			return nil, nil, nil, fmt.Errorf("sqlite %s: %w", pragma, err)
		}
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, rawDB)))
	cleanup := func() {
		_ = client.Close()
		_ = rawDB.Close()
	}
	return client, rawDB, cleanup, nil
}

func normalizeSQLiteDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsn
	}
	if strings.HasPrefix(dsn, "file:") {
		if strings.Contains(dsn, "cache=") && strings.Contains(dsn, "_fk=") {
			return dsn
		}
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		if !strings.Contains(dsn, "cache=") {
			dsn += sep + "cache=shared"
			sep = "&"
		}
		if !strings.Contains(dsn, "_fk=") {
			dsn += sep + "_fk=1"
		}
		return dsn
	}
	return "file:" + dsn + "?cache=shared&_fk=1"
}

package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

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
	NewTaxonomyRepo,
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
	NewChannelRuntimeLeaseRepo,
	NewSessionRunRepo,
	NewSessionRunCheckpointRepo,
	NewSessionParticipantRepo,
	NewUsageRepo,
	NewMonitorAuditRepo,
	NewMonitorEventRepo,
	NewMonitorTraceRepo,
	NewMonitorAlertRepo,
	NewMonitorRunnerCompletionRepo,
	NewSystemSettingRepo,
	NewEvolutionMetricsRepo,
	NewEvolutionSuggestionRepo,
	NewGraphRepo,
	NewGraphRunRepo,
	NewTaskRepo,
	ProvideTaskLinkRepo,
	NewArtifactRepo,
	NewKnowledgeRepoFromData,
	NewKnowledgeSparseSearcherFromData,
	NewEvalRepoFromData,
	NewBackgroundJobRepo,
	NewA2ARepoFromData,
	NewEcosystemRepo,
	NewEcosystemPresetRepo,
	NewEventStoreRepo,
	NewFlowLogRepo,
	NewWebhookRepo,
	NewMemoryJobDeadLetterRepo,
	NewTeamGraphSessionRepo,
	NewAgentTemplateRepo,
	NewMemoryConsolidationWriterAdapter,
	NewMemoryFactIndexMaintainerAdapter,
	NewMemoryEpisodeDecayerAdapter,
	NewMemoryFactDecayerAdapter,
	NewMemoryEpisodeBackfillReaderAdapter,
	NewMemoryLegacyMigratorAdapter,
	NewObservationRepo,
	NewPatternRepo,
	NewProposalRepo,
	NewMemoryFactReader,
	NewToolResultBlobRepo,
	NewToolResultReplacementRepo,
	NewSeedVersionRepo,
	NewOrchestrationCacheRepo,
	NewCompiledTeamRepo,
	NewSkillProposalRepo,
	NewSkillInvocationStatsRepo,
	NewSkillHealthRepo,
	NewSessionMetricsRepo,
	NewSessionMetricsReader,
	NewSessionMetricsCache,
	NewSessionRuntimeRepo,
	NewSessionRuntimeReader,
	NewSpiritTransactor,
	NewTaskPlanRepo,
	NewOrchestrationRepo,
	NewAllocationPlanRepo,
	NewAgentPerformanceRepo,
	NewSelfCheckReportRepo,
	NewHealRecordRepo,
	NewSkillIntelligenceRepo,
	NewSkillDedupRepo,
	NewSkillEvolutionSuggestionRepo,
	NewFailurePatternRepo,
)

// Data: Ent/SQLite holds app CRUD; Postgres (optional) holds pgvector agent memory only.
// 复杂原生 SQL 走 *ent.Client 上的 QueryContext（见 sqlite_db.go），不另开 sql.DB。
type Data struct {
	entClient   *ent.Client
	readClient  *ent.Client
	rawDB       *sql.DB
	readDB      *sql.DB
	pg          *sql.DB
	rw          *ReadWriteClient
	rwDB        *ReadWriteDB
	vectorDim   int
	vectorStore vector.VectorStore
	readiness   *ReadinessGate
	lazySeeders map[string]*LazySeeder
	p1Cancel    context.CancelFunc
	p1Done      chan struct{}
	lg          loggateway.Logger
}

// Ent returns the SQLite-backed Ent client.
func (d *Data) Ent() *ent.Client {
	if d == nil {
		return nil
	}
	return d.entClient
}

// SetEntClientForTest sets up a Data instance for testing with the given client and DB.
func (d *Data) SetEntClientForTest(client *ent.Client, rawDB *sql.DB, lg loggateway.Logger) {
	d.entClient = client
	d.readClient = client
	d.rawDB = rawDB
	d.readDB = rawDB
	d.lg = lg
	d.rw = NewReadWriteClient(client, client)
	d.rwDB = NewReadWriteDB(rawDB, rawDB)
}

// RawDB returns the write *sql.DB handle.
//
// Deprecated: Use d.RWDB().WriteDB(ctx) for transaction-aware raw SQL writes,
// or d.RWDB().ReadDB(ctx) for reads. Direct RawDB() bypasses transaction awareness.
func (d *Data) RawDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.rawDB
}

// ReadDB returns the read-only *sql.DB handle.
//
// Deprecated: Use d.RWDB().ReadDB(ctx) for transaction-aware raw SQL reads.
func (d *Data) ReadDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.readDB
}

// ReadEnt returns the read-only Ent client.
//
// Deprecated: Use d.RW().Read(ctx) for transaction-aware Ent reads.
func (d *Data) ReadEnt() *ent.Client {
	if d == nil {
		return nil
	}
	return d.readClient
}

// ReadClient returns the appropriate Ent client for read operations.
//
// Deprecated: Use d.RW().Read(ctx) for transaction-aware Ent reads.
func (d *Data) ReadClient(ctx context.Context) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return d.ReadEnt()
}

// RW returns the ReadWriteClient for read-write separated Ent access.
// Returns nil if Data is nil (nil-safety).
func (d *Data) RW() *ReadWriteClient {
	if d == nil {
		return nil
	}
	return d.rw
}

// RWDB returns the ReadWriteDB for read-write separated raw SQL access.
// Returns nil if Data is nil (nil-safety).
func (d *Data) RWDB() *ReadWriteDB {
	if d == nil {
		return nil
	}
	return d.rwDB
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

// VectorStore returns the configured VectorStore implementation.
func (d *Data) VectorStore() vector.VectorStore {
	if d == nil {
		return nil
	}
	return d.vectorStore
}

func (d *Data) Readiness() *ReadinessGate {
	if d == nil {
		return nil
	}
	return d.readiness
}

func (d *Data) IsReady() bool {
	if d == nil || d.readiness == nil {
		return true
	}
	return d.readiness.IsReady()
}

func (d *Data) IsFailed() bool {
	if d == nil || d.readiness == nil {
		return false
	}
	return d.readiness.IsFailed()
}

func (d *Data) FailedReason() string {
	if d == nil || d.readiness == nil {
		return ""
	}
	return d.readiness.FailedReason()
}

func (d *Data) SeedLazy(ctx context.Context, name string) error {
	if d == nil || d.lazySeeders == nil {
		return nil
	}
	seeder, ok := d.lazySeeders[name]
	if !ok {
		return nil
	}
	return seeder.SeedIfNeeded(ctx)
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
func NewData(c *conf.Data, lg loggateway.Logger) (*Data, func(), error) {
	var entClient *ent.Client
	var rawDB *sql.DB
	var readClient *ent.Client
	var readDB *sql.DB
	var pg *sql.DB
	var pgOpened bool
	var st *Data

	cleanup := func() {
		if st != nil {
			if st.p1Cancel != nil {
				st.p1Cancel()
			}
			if st.p1Done != nil {
				select {
				case <-st.p1Done:
				case <-time.After(5 * time.Second):
					st.lg.Warn("P1 startup goroutine did not finish in time",
						loggateway.StepID("data.p1_shutdown"))
				}
			}
		}
		if entClient != nil {
			_ = entClient.Close()
		}
		if readClient != nil {
			_ = readClient.Close()
		}
		if rawDB != nil {
			rawDB.Close()
		}
		if readDB != nil {
			readDB.Close()
		}
		if pgOpened && pg != nil {
			pg.Close()
		}
	}

	if err := runStartupStep("initSQLite", func() error {
		var stepErr error
		entClient, rawDB, readClient, readDB, stepErr = initSQLite(c, lg)
		return stepErr
	}, lg); err != nil {
		return nil, nil, err
	}

	if err := runStartupStep("initPostgres", func() error {
		var stepErr error
		pg, stepErr = initPostgres(c, lg)
		pgOpened = true
		return stepErr
	}, lg); err != nil {
		cleanup()
		return nil, nil, err
	}

	if err := runStartupStep("seedAdminUsers", func() error {
		ctx := context.Background()
		if err := ensureInitialAdminFromConfig(ctx, entClient, c, lg); err != nil {
			return err
		}
		return ensureDevBypassAdminIfEnabled(ctx, entClient, lg)
	}, lg); err != nil {
		cleanup()
		return nil, nil, err
	}

	vdim := vectorDimFromConf(c)

	// Initialize VectorStore based on config and feature flag.
	var vs vector.VectorStore
	if pg != nil && conf.DAOVectorPgVector() {
		pgvs, vsErr := vector.NewPgVectorStore(pg, "vector_embeddings", vdim, lg)
		if vsErr != nil {
			lg.Warn("pgvector store init failed, falling back to SQLite",
				loggateway.StepID("data.vector_store"), loggateway.Err(vsErr))
		} else {
			vs = pgvs
		}
	}
	if vs == nil {
		sqliteVS, vsErr := vector.NewSQLiteVectorStore(rawDB, "vector_embeddings", lg)
		if vsErr != nil {
			cleanup()
			return nil, nil, fmt.Errorf("sqlite vector store init: %w", vsErr)
		}
		vs = sqliteVS
	}

	p1Ctx, p1Cancel := context.WithCancel(context.Background())
	p1Done := make(chan struct{})
	st = &Data{entClient: entClient, rawDB: rawDB, readClient: readClient, readDB: readDB, pg: pg, rw: NewReadWriteClient(entClient, readClient), rwDB: NewReadWriteDB(rawDB, readDB), vectorDim: vdim, vectorStore: vs, readiness: newReadinessGate(), p1Cancel: p1Cancel, p1Done: p1Done, lg: lg}

	safego.Go(appctx.Ctx(), "startup.p1", func() {
		defer close(p1Done)

		var p1Err error
		if err := runStartupStep("ensureSchemaDDL", func() error {
			return ensureSchemaDDL(rawDB, entClient, st.lg)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			p1Err = err
			st.lg.Error("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "ensureSchemaDDL"), loggateway.Err(err))
			st.readiness.MarkFailed("ensureSchemaDDL: " + err.Error())
			return
		}

		if err := runStartupStep("ensurePostgresSchemas", func() error {
			return ensurePostgresSchemas(pg, vdim, st.lg)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			p1Err = err
			st.lg.Error("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "ensurePostgresSchemas"), loggateway.Err(err))
			st.readiness.MarkFailed("ensurePostgresSchemas: " + err.Error())
			return
		}

		if err := runStartupStep("dataMigrations", func() error {
			return runPendingDataMigrations(st)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			p1Err = err
			st.lg.Error("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "dataMigrations"), loggateway.Err(err))
			st.readiness.MarkFailed("dataMigrations: " + err.Error())
			return
		}

		if err := runStartupStep("seedP1Data", func() error {
			return seedP1Data(entClient, c, st)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			p1Err = err
			st.lg.Error("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "seedP1Data"), loggateway.Err(err))
			st.readiness.MarkFailed("seedP1Data: " + err.Error())
			return
		}

		if p1Err == nil {
			st.readiness.MarkReady()
		}
		_ = p1Err
	})

	safego.Go(appctx.Ctx(), "startup.lazy_seeds", func() {
		if err := st.readiness.Wait(p1Ctx); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			st.lg.Warn("lazy seeds: readiness wait failed",
				loggateway.StepID("data.lazy"), loggateway.Err(err))
			return
		}
		select {
		case <-p1Ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		// #region debug-point data.lazy.trace
		st.lg.Info("lazy_seeds: loop start", loggateway.StepID("data.lazy.trace"), loggateway.Int("seeders", len(st.lazySeeders)))
		// #endregion debug-point
		for name := range st.lazySeeders {
			if p1Ctx.Err() != nil {
				return
			}
			// #region debug-point data.lazy.trace
			seedStart := time.Now()
			st.lg.Info("lazy_seeds: before SeedLazy", loggateway.StepID("data.lazy.trace"), loggateway.Str("seed", name))
			// #endregion debug-point
			if err := st.SeedLazy(p1Ctx, name); err != nil {
				if p1Ctx.Err() != nil {
					return
				}
				// #region debug-point data.lazy.trace
				st.lg.Info("lazy_seeds: SeedLazy returned error", loggateway.StepID("data.lazy.trace"), loggateway.Str("seed", name), loggateway.Duration(time.Since(seedStart).Milliseconds()), loggateway.Err(err))
				// #endregion debug-point
				st.lg.Warn("lazy seed failed",
					loggateway.StepID("data.lazy"), loggateway.Str("seed", name), loggateway.Err(err))
			}
			// #region debug-point data.lazy.trace
			st.lg.Info("lazy_seeds: after SeedLazy", loggateway.StepID("data.lazy.trace"), loggateway.Str("seed", name), loggateway.Duration(time.Since(seedStart).Milliseconds()))
			// #endregion debug-point
		}
		// #region debug-point data.lazy.trace
		st.lg.Info("lazy_seeds: loop end", loggateway.StepID("data.lazy.trace"))
		// #endregion debug-point
		st.lg.Info("lazy seeds completed", loggateway.StepID("data.lazy"))
	})

	// #region debug-point data.pool.trace
	// DEBUG ONLY: periodic SQLite pool stats dump to identify connection contention.
	safego.Go(appctx.Ctx(), "debug.pool_stats", func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p1Ctx.Done():
				return
			case <-t.C:
				if st == nil || st.rawDB == nil || st.readDB == nil {
					continue
				}
				rw := st.rawDB.Stats()
				rd := st.readDB.Stats()
				st.lg.Info("pool_stats: tick",
					loggateway.StepID("data.pool.trace"),
					loggateway.Int("raw_open", rw.OpenConnections),
					loggateway.Int("raw_in_use", rw.InUse),
					loggateway.Int("raw_idle", rw.Idle),
					loggateway.Int64("raw_wait_count", rw.WaitCount),
					loggateway.Duration(rw.WaitDuration.Milliseconds()),
					loggateway.Int("read_open", rd.OpenConnections),
					loggateway.Int("read_in_use", rd.InUse),
					loggateway.Int("read_idle", rd.Idle),
					loggateway.Int64("read_wait_count", rd.WaitCount),
					loggateway.Duration(rd.WaitDuration.Milliseconds()),
				)
			}
		}
	})
	// #endregion debug-point

	return st, cleanup, nil
}

func runStartupStep(name string, fn func() error, lg loggateway.Logger) error {
	start := time.Now()
	err := fn()
	if err != nil {
		lg.Error("startup step failed", loggateway.StepID("data.startup"), loggateway.Str("step", name), loggateway.Err(err))
		return err
	}
	lg.Info("startup step completed", loggateway.StepID("data.startup"), loggateway.Str("step", name), loggateway.Int("duration_ms", int(time.Since(start).Milliseconds())))
	return nil
}

// entLogAdapter bridges loggateway.Logger to Ent's func(...any) logger interface.
// Ent uses this for debug-mode SQL logging; without it the default falls back
// to log.Println which violates the project's "no log/slog" rule (Red Line #16).
func entLogAdapter(lg loggateway.Logger) func(...any) {
	return func(args ...any) {
		lg.Info(fmt.Sprint(args...), loggateway.StepID("ent.sql"))
	}
}

// initSQLite opens the SQLite database, configures Ent, applies PRAGMAs,
// and runs migration.
func initSQLite(c *conf.Data, lg loggateway.Logger) (*ent.Client, *sql.DB, *ent.Client, *sql.DB, error) {
	driverName, dsn, err := entSQLiteDriverAndDSN(c)
	if err != nil {
		lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("sqlite (ent): %w", err)
	}
	if err := ensureSQLiteParentDir(dsn); err != nil {
		lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "ensure_parent_dir"), loggateway.Err(err))
		return nil, nil, nil, nil, err
	}

	rawDB, err := sql.Open(driverName, dsn)
	if err != nil {
		lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "sql_open"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("failed opening sqlite raw db: %w", err)
	}
	rawDB.SetMaxOpenConns(1)
	rawDB.SetMaxIdleConns(1)
	rawDB.SetConnMaxIdleTime(5 * time.Minute)

	drv := entsql.OpenDB(driverName, rawDB)
	entClient := ent.NewClient(ent.Driver(drv), ent.Log(entLogAdapter(lg)))

	if driverName == dialect.SQLite {
		for _, pragma := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=30000",
			"PRAGMA synchronous=NORMAL",
		} {
			if _, pragmaErr := rawDB.ExecContext(context.Background(), pragma); pragmaErr != nil {
				rawDB.Close()
				lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "pragma"), loggateway.Str("pragma", pragma), loggateway.Err(pragmaErr))
				return nil, nil, nil, nil, fmt.Errorf("sqlite %s: %w", pragma, pragmaErr)
			}
		}
	}

	ctxEnt := context.Background()
	if strings.TrimSpace(os.Getenv("DEPLOY_ENV")) == "dev" {
		entClient, err = migrateDev(ctxEnt, entClient, "sqlite(ent)")
		if err != nil {
			rawDB.Close()
			lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "migrate_dev"), loggateway.Err(err))
			return nil, nil, nil, nil, err
		}
	} else if err = entClient.Schema.Create(ctxEnt); err != nil {
		rawDB.Close()
		lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "schema_create"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("ent schema create (sqlite): %w", err)
	}

	readDB, err := sql.Open(driverName, dsn)
	if err != nil {
		rawDB.Close()
		lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "read_db_open"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("failed opening sqlite read db: %w", err)
	}
	readDB.SetMaxOpenConns(2)
	readDB.SetMaxIdleConns(2)
	readDB.SetConnMaxIdleTime(5 * time.Minute)
	if driverName == dialect.SQLite {
		for _, pragma := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=30000",
			"PRAGMA synchronous=NORMAL",
		} {
			if _, pragmaErr := readDB.ExecContext(context.Background(), pragma); pragmaErr != nil {
				rawDB.Close()
				readDB.Close()
				lg.Error("init sqlite failed", loggateway.StepID("data.init_sqlite"), loggateway.Str("step", "read_db_pragma"), loggateway.Str("pragma", pragma), loggateway.Err(pragmaErr))
				return nil, nil, nil, nil, fmt.Errorf("sqlite read %s: %w", pragma, pragmaErr)
			}
		}
	}
	readDrv := entsql.OpenDB(driverName, readDB)
	readClient := ent.NewClient(ent.Driver(readDrv), ent.Log(entLogAdapter(lg)))

	return entClient, rawDB, readClient, readDB, nil
}

// ensureSchemaDDL applies Ent and raw SQL schema patches (DDL only; no data migrations).
func ensureSchemaDDL(rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return runDDLMigrations(rawDB, entClient, lg)
}

// runPendingDataMigrations applies one-time data migrations (schema_migrations gate).
func runPendingDataMigrations(d *Data) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entClient := d.Ent()
	migrated, skipped, err := RunLegacyTRPCMemoryMigration(ctx, d, d.lg)
	if err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.legacy_trpc_memory"), loggateway.Err(err))
		return fmt.Errorf("legacy trpc memory backfill: %w", err)
	}
	if skipped {
		d.lg.Info("legacy trpc memory backfill skipped",
			loggateway.StepID("data.startup"), loggateway.Int("migration", MigrationLegacyTRPCMemoryFacts))
	} else if migrated > 0 {
		d.lg.Info("legacy trpc memory backfill migrated",
			loggateway.StepID("data.startup"), loggateway.Int("migrated", migrated))
	}
	if err := RunTurnIndexToTurnIDMigration(ctx, entClient, d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.turn_index_to_turn_id"), loggateway.Err(err))
		return fmt.Errorf("turn_index migration: %w", err)
	}
	if err := RunSessionStatusIdleMigration(ctx, entClient, d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.session_status_idle"), loggateway.Err(err))
		return fmt.Errorf("session status migration: %w", err)
	}
	return nil
}

// ensureAllSchemas applies DDL patches and pending data migrations (compat wrapper for tests).
func ensureAllSchemas(rawDB *sql.DB, d *Data, lg loggateway.Logger) error {
	if err := ensureSchemaDDL(rawDB, d.Ent(), lg); err != nil {
		return err
	}
	return runPendingDataMigrations(d)
}

// initPostgres opens the optional Postgres vector store connection.
// On failure, logs a warning and returns nil (degrades to SQLite-only mode).
func initPostgres(c *conf.Data, lg loggateway.Logger) (*sql.DB, error) {
	pgDSN := postgresVectorDSN(c)
	if pgDSN == "" {
		return nil, nil
	}
	pg, err := sql.Open("postgres", pgDSN)
	if err != nil {
		lg.Warn("init postgres failed, degrading to SQLite-only mode", loggateway.StepID("data.init_postgres"), loggateway.Str("step", "sql_open"), loggateway.Err(err))
		return nil, nil
	}
	pg.SetMaxOpenConns(8)
	pg.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = pg.PingContext(ctx); err != nil {
		pg.Close()
		lg.Warn("init postgres ping failed, degrading to SQLite-only mode", loggateway.StepID("data.init_postgres"), loggateway.Str("step", "ping"), loggateway.Err(err))
		return nil, nil
	}
	return pg, nil
}

// ensurePostgresSchemas applies vector and knowledge schema on Postgres if configured.
func ensurePostgresSchemas(pg *sql.DB, vdim int, lg loggateway.Logger) error {
	if pg == nil {
		return nil
	}
	ctxPG := context.Background()
	if err := pgvector.EnsureSchema(ctxPG, pg, vdim); err != nil {
		lg.Error("postgres schema step failed", loggateway.StepID("data.schema.pgvector"), loggateway.Err(err))
		return err
	}
	if err := EnsureKnowledgeSchema(ctxPG, pg, vdim); err != nil {
		lg.Error("postgres schema step failed", loggateway.StepID("data.schema.knowledge"), loggateway.Err(err))
		return fmt.Errorf("knowledge schema: %w", err)
	}
	return nil
}

func seedP1Data(entClient *ent.Client, c *conf.Data, d *Data) error {
	lg := d.lg
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// P1 种子步骤全部幂等（ON CONFLICT UPDATE/DO NOTHING），
	// 采用 best-effort 模式：收集错误但不中断，下次重启时幂等重试。
	var seedErrs []error

	seedStep := func(stepID string, fn func() error) {
		if err := fn(); err != nil {
			lg.Warn("seed step failed", loggateway.StepID(stepID), loggateway.Err(err))
			seedErrs = append(seedErrs, fmt.Errorf("%s: %w", stepID, err))
		}
	}

	seedStep("data.seed.channel_avatars", func() error {
		return ensureChannelPlatformAvatars(ctx, entClient, lg)
	})
	seedStep("data.seed.agent_avatars", func() error {
		return ensureAgentAvatars(ctx, entClient, lg)
	})
	seedStep("data.seed.system_admin_agent", func() error {
		return SeedSystemAdminAgent(ctx, entClient, lg)
	})
	seedStep("data.seed.spirit_agent", func() error {
		return SeedSpiritAgent(ctx, entClient, lg)
	})
	seedStep("data.seed.memory_agent", func() error {
		return SeedMemoryAgent(ctx, entClient, lg)
	})
	seedStep("data.seed.skills_agent", func() error {
		return SeedSkillsAgent(ctx, entClient, lg)
	})
	seedStep("data.seed.cli_admin_tools", func() error {
		return SeedBuiltinCLIAdminTools(ctx, entClient, lg)
	})

	scenarioDir := biz.ScenarioDir()

	seedStep("data.seed.pack_builtin_templates", func() error {
		return SeedPackBuiltinTemplates(ctx, entClient, scenarioDir, lg)
	})
	seedStep("data.seed.pack_builtin_templates_v2", func() error {
		return SeedPackBuiltinTemplatesV2(ctx, entClient, scenarioDir, lg)
	})
	seedStep("data.seed.spirit_prompt_files", func() error {
		return SeedSpiritPromptFiles(ctx, entClient, scenarioDir, lg)
	})
	seedStep("data.seed.butler_prompt_files", func() error {
		return SeedButlerPromptFiles(ctx, entClient, scenarioDir, lg)
	})
	seedStep("data.seed.cron_tasks", func() error {
		return SeedCronTasks(ctx, entClient, lg)
	})

	d.lazySeeders = map[string]*LazySeeder{}

	if len(seedErrs) > 0 {
		lg.Warn("P1 seed completed with errors",
			loggateway.StepID("data.seed.p1"),
			loggateway.Int("error_count", len(seedErrs)))
		return seedErrs[0]
	}
	return nil
}

func NewArtifactRepo(d *Data) biz.ArtifactRepo {
	return artifactfs.NewFSArtifactRepo(d.lg)
}

func NewKnowledgeRepoFromData(d *Data) biz.KnowledgeRepo {
	return NewKnowledgeRepo(d, d.lg)
}

func NewKnowledgeSparseSearcherFromData(d *Data) biz.KnowledgeSparseSearcher {
	repo := NewKnowledgeRepo(d, d.lg)
	if repo == nil {
		return nil
	}
	kr, ok := repo.(*knowledgeRepo)
	if !ok {
		return nil
	}
	return kr
}

func NewEvalRepoFromData(d *Data) biz.EvalRepo {
	return NewEvalRepo(d, d.lg)
}

func NewA2ARepoFromData(d *Data, lg loggateway.Logger) biz.A2ARepo {
	return NewA2ARepo(d, lg)
}

// NewCLIData wraps SQLite handles opened by OpenSQLiteEntClient for offline maintenance CLIs.
func NewCLIData(client *ent.Client, rawDB *sql.DB, lg loggateway.Logger) *Data {
	var vs vector.VectorStore
	if rawDB != nil {
		if s, err := vector.NewSQLiteVectorStore(rawDB, "vector_embeddings", lg); err == nil {
			vs = s
		}
	}
	return &Data{entClient: client, readClient: client, rawDB: rawDB, readDB: rawDB, rw: NewReadWriteClient(client, client), rwDB: NewReadWriteDB(rawDB, rawDB), vectorStore: vs, lg: lg}
}

// OpenSQLiteEntClient opens SQLite for offline CLI maintenance tools (e.g. memory-migrate).
// Do not use against a DSN while admin is running — use in-process NewData migrations instead.
func OpenSQLiteEntClient(dsn string, lg loggateway.Logger) (*ent.Client, *sql.DB, func(), error) {
	dsn = normalizeSQLiteDSN(dsn)
	rawDB, err := sql.Open(dialect.SQLite, dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	rawDB.SetMaxOpenConns(1)
	rawDB.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := rawDB.ExecContext(context.Background(), pragma); err != nil {
			rawDB.Close()
			return nil, nil, nil, fmt.Errorf("sqlite %s: %w", pragma, err)
		}
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, rawDB)), ent.Log(entLogAdapter(lg)))
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

// generateCatalogID generates a random 24-hex-char ID for agent/team catalog entries.
func generateCatalogID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		// Fallback: use timestamp-based ID
		return fmt.Sprintf("%012x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

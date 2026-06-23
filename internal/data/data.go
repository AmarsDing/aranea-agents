package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/glebarez/go-sqlite/compat"
	_ "github.com/go-sql-driver/mysql"
	"github.com/lib/pq"

	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewAdminRepo,
	NewAdminReader,
	NewAdminWriter,
	NewAvatarRepo,
	NewMemoryRepo,
	NewAgentRepo,
	NewTeamRepo,
	NewOrganizationRepo,
	NewBorrowRequestRepo,
	NewLlmProviderModelRepo,
	NewHookRepo,
	NewCronRepo,
	NewPluginRepo,
	NewPluginRunRepo,
	NewHookDeliveryRepo,
	NewPluginCostGuardUsageRepo,
	NewMCPServerRepo,
	NewMCPServerUserCredentialRepo,
	NewSkillRepo,
	NewSessionRepo,
	NewToolRepo,
	NewChannelRepo,
	wire.Bind(new(biz.ChannelReader), new(*channelRepo)),
	wire.Bind(new(biz.ChannelWriter), new(*channelRepo)),
	wire.Bind(new(biz.ChannelCredentialRepo), new(*channelRepo)),
	wire.Bind(new(biz.ChannelDeliveryRepo), new(*channelRepo)),
	// Bind *Data to the biz-layer transaction provider interfaces so that
	// ProvideChannelUsecase / ProvideEvolutionUsecase can wrap multi-step
	// writes in a single transaction (red line #24).
	wire.Bind(new(biz.ChannelTxProvider), new(*Data)),
	wire.Bind(new(biz.EvolutionTxProvider), new(*Data)),
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
	NewWebhookReader,
	NewWebhookWriter,
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
	NewRuntimeProfileRepo,
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
	NewSkillMergeRepo,
	NewSkillEvolutionSuggestionRepo,
	NewFailurePatternRepo,
	NewUnifiedEvolutionRepo,
	NewPackSeeder,
	NewCircuitBreakerStateRepo,
	// AF-BE-04/17: Activity repo for Activity-First architecture
	NewActivityRepo,
	wire.Bind(new(biz.ActivityReader), new(biz.ActivityRepo)),
	wire.Bind(new(biz.ActivityWriter), new(biz.ActivityRepo)),
)

// Data: Ent/SQLite holds app CRUD; Postgres (optional) holds pgvector agent memory only.
// 复杂原生 SQL 走 *ent.Client 上的 QueryContext（见 sqlite_db.go），不另开 sql.DB。
type Data struct {
	entClient   *ent.Client
	readClient  *ent.Client
	rawDB       *sql.DB
	readDB      *sql.DB
	pg          *sql.DB
	pgRead      *sql.DB
	rw          *ReadWriteClient
	rwDB        *ReadWriteDB
	vectorDim   int
	vectorStore vector.VectorStore
	reranker    biz.Reranker
	readiness   *ReadinessGate
	lazySeeders map[string]*LazySeeder
	p1Cancel    context.CancelFunc
	p1Done      chan struct{}
	lg          loggateway.Logger
	// txTimeout is the hard deadline for ExecInTx transactions. Default 30s.
	// Set to 0 to disable the timeout (e.g., for long-running Postgres migrations).
	txTimeout time.Duration
	// dialect identifies the primary database dialect (sqlite or postgres).
	// Controls dialect-aware SQL generation in Repo implementations.
	dialect Dialect
	// pgDSN stores the Postgres connection string when driver=postgres.
	// Used by framework components (e.g. trpcsession postgres service) that
	// create their own connection pools from a DSN rather than sharing *sql.DB.
	pgDSN string
}

// Dialect returns the active primary database dialect.
func (d *Data) Dialect() Dialect {
	if d == nil {
		return DialectSQLite
	}
	return d.dialect
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
	d.txTimeout = 30 * time.Second
	d.dialect = DialectSQLite
}

// SetTxTimeout configures the hard deadline for ExecInTx transactions.
// A value of 0 disables the timeout (use with care — only for long-running
// Postgres migrations or batch operations where the caller manages deadlines).
func (d *Data) SetTxTimeout(dur time.Duration) {
	if d != nil {
		d.txTimeout = dur
	}
}

// TxTimeout returns the configured transaction timeout. Returns 30s if unset.
func (d *Data) TxTimeout() time.Duration {
	if d == nil || d.txTimeout <= 0 {
		return 30 * time.Second
	}
	return d.txTimeout
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

// Postgres returns the Postgres write DB handle (16 connections) for vectors,
// WAL, EventStore, and Checkpoint writes. Returns nil if not configured.
func (d *Data) Postgres() *sql.DB {
	if d == nil {
		return nil
	}
	return d.pg
}

// PostgresRead returns the Postgres read DB handle (32 connections) for vector
// similarity search and read-heavy queries. Returns nil if not configured.
// Falls back to the write pool if the read pool is not initialized.
func (d *Data) PostgresRead() *sql.DB {
	if d == nil {
		return nil
	}
	if d.pgRead != nil {
		return d.pgRead
	}
	return d.pg
}

// PostgresDSN returns the Postgres connection string when driver=postgres.
// Returns empty string when the primary database is SQLite.
// Used by framework components (e.g. trpcsession postgres service) that
// create their own connection pools from a DSN rather than sharing *sql.DB.
func (d *Data) PostgresDSN() string {
	if d == nil {
		return ""
	}
	return d.pgDSN
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

// Reranker returns the configured biz.Reranker for memory recall scoring.
func (d *Data) Reranker() biz.Reranker {
	if d == nil || d.reranker == nil {
		return biz.NewCrossEncoderReranker()
	}
	return d.reranker
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

// NewData opens Postgres for Ent CRUD + pgvector + WAL + EventStore.
// The underlying *sql.DB is shared with trpc session/checkpoint adapters via RawDB().
//
// SQLite is no longer supported as a primary database in production. It remains
// available only for:
//   - Test infrastructure (testhelper.SetupTestDB uses in-memory SQLite)
//   - Offline CLI maintenance tools (OpenSQLiteEntClient/NewCLIData)
func NewData(c *conf.Data, lg loggateway.Logger) (*Data, func(), error) {
	var entClient *ent.Client
	var rawDB *sql.DB
	var readClient *ent.Client
	var readDB *sql.DB
	var pg *sql.DB
	var pgRead *sql.DB
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
		if readClient != nil && readClient != entClient {
			_ = readClient.Close()
		}
		if rawDB != nil {
			rawDB.Close()
		}
		if readDB != nil && readDB != rawDB {
			readDB.Close()
		}
	}

	// Postgres is the only supported primary database.
	if err := runStartupStep("initPostgresEnt", func() error {
		var stepErr error
		entClient, rawDB, readClient, readDB, stepErr = initPostgresEnt(c, lg)
		return stepErr
	}, lg); err != nil {
		return nil, nil, err
	}
	// Postgres is primary: pgvector/WAL/EventStore share the same pool.
	pg = rawDB
	pgRead = readDB
	activeDialect := DialectPostgres

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

	// Initialize VectorStore — pgvector is the only supported backend in production.
	// SQLiteVectorStore remains available only for tests (see vector/sqlite.go).
	var vs vector.VectorStore
	if pg != nil && conf.DAOVectorPgVector() {
		pgvs, vsErr := vector.NewPgVectorStore(pg, "vector_embeddings", vdim, lg)
		if vsErr != nil {
			cleanup()
			return nil, nil, fmt.Errorf("pgvector store init: %w", vsErr)
		}
		vs = pgvs
	} else {
		cleanup()
		return nil, nil, fmt.Errorf("pgvector is required: set data.postgres.source and enable DAO_VECTOR_PGVECTOR")
	}

	p1Ctx, p1Cancel := context.WithCancel(context.Background())
	p1Done := make(chan struct{})
	pgDSNForData := postgresVectorDSN(c)
	st = &Data{entClient: entClient, rawDB: rawDB, readClient: readClient, readDB: readDB, pg: pg, pgRead: pgRead, rw: NewReadWriteClient(entClient, readClient), rwDB: NewReadWriteDB(rawDB, readDB), vectorDim: vdim, vectorStore: vs, reranker: newMemoryReranker(lg), readiness: newReadinessGate(), p1Cancel: p1Cancel, p1Done: p1Done, lg: lg, txTimeout: 30 * time.Second, dialect: activeDialect, pgDSN: pgDSNForData}

	safego.Go(appctx.Ctx(), "startup.p1", func() {
		defer close(p1Done)

		var p1Err error
		if err := runStartupStep("ensureSchemaDDL", func() error {
			return ensureSchemaDDL(rawDB, entClient, st.Dialect(), st.lg)
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
			// seedP1Data is best-effort: errors are already logged inside,
			// do NOT call MarkFailed — service should still become ready.
			st.lg.Warn("P1 seed completed with errors, service will still be ready",
				loggateway.StepID("data.p1"), loggateway.Str("step", "seedP1Data"), loggateway.Err(err))
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

// initPostgresEnt opens Postgres as the primary Ent database (driver=postgres).
// Returns write/read Ent clients and write/read *sql.DB handles.
// The write pool (MaxOpen=16) handles Ent writes + raw SQL writes; the read
// pool (MaxOpen=32) handles Ent reads + raw SQL reads. Both pools share the
// same DSN from data.postgres.source.
func initPostgresEnt(c *conf.Data, lg loggateway.Logger) (*ent.Client, *sql.DB, *ent.Client, *sql.DB, error) {
	pgDSN := postgresVectorDSN(c)
	if pgDSN == "" {
		return nil, nil, nil, nil, fmt.Errorf("data.postgres.source is required when driver=postgres")
	}

	// Write pool: 16 connections (Ent writes, raw SQL writes, DDL).
	writeDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		lg.Error("init postgres ent failed", loggateway.StepID("data.init_pg_ent"), loggateway.Str("step", "sql_open_write"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("open postgres write pool: %w", err)
	}
	writeDB.SetMaxOpenConns(16)
	writeDB.SetMaxIdleConns(4)
	writeDB.SetConnMaxLifetime(30 * time.Minute)
	writeDB.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = writeDB.PingContext(ctx); err != nil {
		writeDB.Close()
		lg.Error("init postgres ent failed", loggateway.StepID("data.init_pg_ent"), loggateway.Str("step", "ping_write"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("ping postgres write pool: %w", err)
	}

	// Read pool: 32 connections (Ent reads, raw SQL reads, vector search).
	readDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		writeDB.Close()
		lg.Error("init postgres ent failed", loggateway.StepID("data.init_pg_ent"), loggateway.Str("step", "sql_open_read"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("open postgres read pool: %w", err)
	}
	readDB.SetMaxOpenConns(32)
	readDB.SetMaxIdleConns(8)
	readDB.SetConnMaxLifetime(30 * time.Minute)
	readDB.SetConnMaxIdleTime(5 * time.Minute)

	// Ent write client.
	writeDrv := entsql.OpenDB(dialect.Postgres, writeDB)
	entClient := ent.NewClient(ent.Driver(writeDrv), ent.Log(entLogAdapter(lg)))

	ctxEnt := context.Background()
	if strings.TrimSpace(os.Getenv("DEPLOY_ENV")) == "dev" {
		entClient, err = migrateDev(ctxEnt, entClient, "postgres(ent)")
		if err != nil {
			writeDB.Close()
			readDB.Close()
			lg.Error("init postgres ent failed", loggateway.StepID("data.init_pg_ent"), loggateway.Str("step", "migrate_dev"), loggateway.Err(err))
			return nil, nil, nil, nil, err
		}
	} else if err = entClient.Schema.Create(ctxEnt); err != nil {
		writeDB.Close()
		readDB.Close()
		lg.Error("init postgres ent failed", loggateway.StepID("data.init_pg_ent"), loggateway.Str("step", "schema_create"), loggateway.Err(err))
		return nil, nil, nil, nil, fmt.Errorf("ent schema create (postgres): %w", err)
	}

	// Ent read client.
	readDrv := entsql.OpenDB(dialect.Postgres, readDB)
	readClient := ent.NewClient(ent.Driver(readDrv), ent.Log(entLogAdapter(lg)))

	lg.Info("postgres ent initialized",
		loggateway.StepID("data.init_pg_ent"),
		loggateway.Str("driver", "postgres"))
	return entClient, writeDB, readClient, readDB, nil
}

// ensureSchemaDDL applies Ent and raw SQL schema patches (DDL only; no data migrations).
// Uses the active dialect for idempotent error detection.
func ensureSchemaDDL(rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return runDDLMigrationsWithDialect(rawDB, entClient, d, lg)
}

// runPendingDataMigrations applies one-time data migrations (schema_migrations gate).
func runPendingDataMigrations(d *Data) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entClient := d.RW().Write(ctx)
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
	if err := RunTurnIndexToTurnIDMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.turn_index_to_turn_id"), loggateway.Err(err))
		return fmt.Errorf("turn_index migration: %w", err)
	}
	if err := RunSessionTurnNumberBackfillMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.session_turn_number"), loggateway.Err(err))
		return fmt.Errorf("session_turn_number backfill migration: %w", err)
	}
	if err := RunSessionTurnNumberRebackfillMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.session_turn_number_rebackfill"), loggateway.Err(err))
		return fmt.Errorf("session_turn_number rebackfill migration: %w", err)
	}
	if err := RunSessionStatusIdleMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.session_status_idle"), loggateway.Err(err))
		return fmt.Errorf("session status migration: %w", err)
	}
	if err := RunOrganizationRedesignMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.organization_redesign"), loggateway.Err(err))
		return fmt.Errorf("organization redesign migration: %w", err)
	}
	if err := RunActivityBackfillMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.activity_backfill"), loggateway.Err(err))
		return fmt.Errorf("activity backfill migration: %w", err)
	}
	if err := RunAvatarImageRepairMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.avatar_image_repair"), loggateway.Err(err))
		return fmt.Errorf("avatar image repair migration: %w", err)
	}
	return nil
}

// ensureAllSchemas applies DDL patches and pending data migrations (compat wrapper for tests).
func ensureAllSchemas(rawDB *sql.DB, d *Data, lg loggateway.Logger) error {
	if err := ensureSchemaDDL(rawDB, d.RW().Write(context.Background()), d.Dialect(), lg); err != nil {
		return err
	}
	return runPendingDataMigrations(d)
}

// ensurePostgresSchemas applies vector and knowledge schema on Postgres if configured.
// Also runs Phase 1 migration (event_wal/event_store/session_run_checkpoints tables
// + invariant constraints) for Postgres-native WAL/EventStore/Checkpoint support.
func ensurePostgresSchemas(pg *sql.DB, vdim int, lg loggateway.Logger) error {
	if pg == nil {
		return nil
	}
	ctxPG := context.Background()
	if vector.IsPgvector() {
		if err := vector.EnsureSchema(ctxPG, pg, vdim); err != nil {
			lg.Error("postgres schema step failed", loggateway.StepID("data.schema.pgvector"), loggateway.Err(err))
			return err
		}
	} else {
		lg.Info("pgvector build tag not set, skipping vector schema on Postgres", loggateway.StepID("data.schema.pgvector"))
	}
	if err := EnsureKnowledgeSchema(ctxPG, pg, vdim); err != nil {
		lg.Error("postgres schema step failed", loggateway.StepID("data.schema.knowledge"), loggateway.Err(err))
		return fmt.Errorf("knowledge schema: %w", err)
	}
	if err := ensurePostgresPhase1Schema(ctxPG, pg, lg); err != nil {
		lg.Error("postgres phase1 schema step failed", loggateway.StepID("data.schema.postgres_phase1"), loggateway.Err(err))
		return fmt.Errorf("postgres phase1 schema: %w", err)
	}
	return nil
}

// ensurePostgresPhase1Schema runs the Phase 1 migration SQL on Postgres.
// Idempotent — uses IF NOT EXISTS / ON CONFLICT DO NOTHING / DO $$ blocks.
// The migration creates event_wal, event_store, session_run_checkpoints tables
// (Postgres-native types: TIMESTAMPTZ, partial unique indexes, FK constraints)
// and adds invariant constraints (INV-UNIQ-01/02, INV-REF-01/02/03).
//
// The entire SQL is sent as a single ExecContext call because Postgres DO $$
// blocks contain semicolons inside the $$ delimiters, which would break the
// generic splitDDLStatements splitter. The lib/pq driver supports
// multi-statement execution in a single call.
func ensurePostgresPhase1Schema(ctx context.Context, pg *sql.DB, lg loggateway.Logger) error {
	sqlBytes, err := fs.ReadFile(migrationSQLFS, "sql/migrations/20260617_postgres_phase1.sql")
	if err != nil {
		return fmt.Errorf("read postgres phase1 SQL: %w", err)
	}
	ddl := strings.TrimPrefix(string(sqlBytes), "\ufeff")
	if _, err := pg.ExecContext(ctx, ddl); err != nil {
		// Postgres idempotency: tolerate "already exists" errors at the
		// statement level. Since we send all statements in one call, a
		// duplicate-object error in any statement aborts the whole batch.
		// The SQL uses IF NOT EXISTS / DO $$ ... IF NOT EXISTS ... checks
		// to be idempotent, so this error path should only trigger on
		// unexpected failures.
		if isPostgresAlreadyExistsErr(err) {
			lg.Debug("postgres phase1 step skipped (already exists)",
				loggateway.StepID("data.schema.postgres_phase1"),
				loggateway.Err(err))
			return nil
		}
		return fmt.Errorf("execute postgres phase1 SQL: %w", err)
	}
	lg.Info("postgres phase1 schema applied",
		loggateway.StepID("data.schema.postgres_phase1"))
	return nil
}

// isPostgresAlreadyExistsErr reports whether err is a Postgres "already exists"
// error (e.g., duplicate table/index/constraint). Used for idempotent migrations.
func isPostgresAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		// 42P07 = duplicate_table, 42710 = duplicate_object, 42701 = duplicate_column
		switch pgErr.Code {
		case "42P07", "42710", "42701":
			return true
		}
	}
	return false
}

func seedP1Data(entClient *ent.Client, c *conf.Data, d *Data) error {
	lg := d.lg

	// P1 种子步骤全部幂等（ON CONFLICT UPDATE/DO NOTHING），
	// 采用 best-effort 模式：收集错误但不中断，下次重启时幂等重试。
	// 每个步骤使用独立的 30 秒 context，避免前面的步骤消耗共享 context 时间。
	var seedErrs []error

	seedStep := func(stepID string, fn func(ctx context.Context) error) {
		stepCtx, stepCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stepCancel()
		if err := fn(stepCtx); err != nil {
			lg.Warn("seed step failed", loggateway.StepID(stepID), loggateway.Err(err))
			seedErrs = append(seedErrs, fmt.Errorf("%s: %w", stepID, err))
		}
	}

	seedStep("data.seed.channel_avatars", func(ctx context.Context) error {
		return ensureChannelPlatformAvatars(ctx, entClient, lg)
	})
	seedStep("data.seed.agent_avatars", func(ctx context.Context) error {
		return ensureAgentAvatars(ctx, entClient, lg)
	})
	seedStep("data.seed.system_admin_agent", func(ctx context.Context) error {
		return SeedSystemAdminAgent(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.spirit_agent", func(ctx context.Context) error {
		return SeedSpiritAgent(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.memory_agent", func(ctx context.Context) error {
		return SeedMemoryAgent(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.skills_agent", func(ctx context.Context) error {
		return SeedSkillsAgent(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.cli_admin_tools", func(ctx context.Context) error {
		return SeedBuiltinCLIAdminTools(ctx, entClient, d.Dialect(), lg)
	})

	scenarioDir := biz.ScenarioDir()

	seedStep("data.seed.pack_builtin_templates", func(ctx context.Context) error {
		return SeedPackBuiltinTemplates(ctx, entClient, d.Dialect(), scenarioDir, lg)
	})
	seedStep("data.seed.pack_builtin_templates_v2", func(ctx context.Context) error {
		return SeedPackBuiltinTemplatesV2(ctx, entClient, d.Dialect(), scenarioDir, lg)
	})
	seedStep("data.seed.spirit_prompt_files", func(ctx context.Context) error {
		return SeedSpiritPromptFiles(ctx, entClient, d.Dialect(), scenarioDir, lg)
	})
	seedStep("data.seed.butler_prompt_files", func(ctx context.Context) error {
		return SeedButlerPromptFiles(ctx, entClient, d.Dialect(), scenarioDir, lg)
	})
	seedStep("data.seed.cron_tasks", func(ctx context.Context) error {
		return SeedCronTasks(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.dept_lead_agents", func(ctx context.Context) error {
		return SeedDeptLeadAgents(ctx, entClient, d.Dialect(), lg)
	})
	seedStep("data.seed.dept_lead_prompt_files", func(ctx context.Context) error {
		return SeedDeptLeadPromptFiles(ctx, entClient, d.Dialect(), scenarioDir, lg)
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
	return &Data{entClient: client, readClient: client, rawDB: rawDB, readDB: rawDB, rw: NewReadWriteClient(client, client), rwDB: NewReadWriteDB(rawDB, rawDB), vectorStore: vs, reranker: newMemoryReranker(lg), lg: lg, dialect: DialectSQLite}
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

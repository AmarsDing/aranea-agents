package data

import (
	"context"
	"database/sql"
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
	"aranea-agents/internal/data/sessionmemory"
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
	NewChannelRuntimeLeaseRepo,
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
	NewKnowledgeSparseSearcherFromData,
	NewEvalRepoFromData,
	NewBackgroundJobRepo,
	NewA2ARepoFromData,
	NewEcosystemRepo,
	NewEventStoreRepo,
	NewFlowLogRepo,
	NewWebhookRepo,
	NewMemoryJobDeadLetterRepo,
	NewTeamGraphSessionRepo,
	NewIndustryRepo,
	NewDepartmentRepo,
	NewPositionRepo,
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
)

// Data: Ent/SQLite holds app CRUD; Postgres (optional) holds pgvector agent memory only.
// 复杂原生 SQL 走 *ent.Client 上的 QueryContext（见 sqlite_db.go），不另开 sql.DB。
type Data struct {
	entClient   *ent.Client
	readClient  *ent.Client
	rawDB       *sql.DB
	readDB      *sql.DB
	pg          *sql.DB
	vectorDim   int
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

// RawDB returns the underlying SQLite *sql.DB shared by Ent and trpc adapters (session / checkpoint).
func (d *Data) RawDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.rawDB
}

func (d *Data) ReadDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.readDB
}

func (d *Data) ReadEnt() *ent.Client {
	if d == nil {
		return nil
	}
	return d.readClient
}

func (d *Data) ReadClient(ctx context.Context) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return d.ReadEnt()
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
	p1Ctx, p1Cancel := context.WithCancel(context.Background())
	p1Done := make(chan struct{})
	st = &Data{entClient: entClient, rawDB: rawDB, readClient: readClient, readDB: readDB, pg: pg, vectorDim: vdim, readiness: newReadinessGate(), p1Cancel: p1Cancel, p1Done: p1Done, lg: lg}

	safego.Go(context.Background(), "startup.p1", func() {
		defer close(p1Done)
		defer st.readiness.MarkReady()

		if err := runStartupStep("ensureSchemaDDL", func() error {
			return ensureSchemaDDL(rawDB, entClient, st.lg)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			st.lg.Warn("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "ensureSchemaDDL"), loggateway.Err(err))
			return
		}

		if err := runStartupStep("ensurePostgresSchemas", func() error {
			return ensurePostgresSchemas(pg, vdim, st.lg)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			st.lg.Warn("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "ensurePostgresSchemas"), loggateway.Err(err))
			return
		}

		if err := runStartupStep("dataMigrations", func() error {
			return runPendingDataMigrations(entClient, st.lg)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			st.lg.Warn("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "dataMigrations"), loggateway.Err(err))
			return
		}

		if err := runStartupStep("seedP1Data", func() error {
			return seedP1Data(entClient, c, st)
		}, st.lg); err != nil {
			if p1Ctx.Err() != nil {
				return
			}
			st.lg.Warn("P1 startup step failed",
				loggateway.StepID("data.p1"), loggateway.Str("step", "seedP1Data"), loggateway.Err(err))
			return
		}
	})

	safego.Go(context.Background(), "startup.lazy_seeds", func() {
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
		for name := range st.lazySeeders {
			if p1Ctx.Err() != nil {
				return
			}
			if err := st.SeedLazy(p1Ctx, name); err != nil {
				if p1Ctx.Err() != nil {
					return
				}
				st.lg.Warn("lazy seed failed",
					loggateway.StepID("data.lazy"), loggateway.Str("seed", name), loggateway.Err(err))
			}
		}
		st.lg.Info("lazy seeds completed",
			loggateway.StepID("data.lazy"))
	})

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
	entClient := ent.NewClient(ent.Driver(drv))

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
	readClient := ent.NewClient(ent.Driver(readDrv))

	return entClient, rawDB, readClient, readDB, nil
}

// ensureSchemaDDL applies Ent and raw SQL schema patches (DDL only; no data migrations).
func ensureSchemaDDL(rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	if err := sessionmemory.EnsurePatches(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_memory_patches"), loggateway.Err(err))
		return fmt.Errorf("session memory patches: %w", err)
	}
	// Must run before EnsureSessionMemorySchema: the SQL DDL contains
	// CREATE INDEX on index_status, but CREATE TABLE IF NOT EXISTS skips
	// adding new columns to an existing table. Without this patch first,
	// the index creation fails with "no such column: index_status".
	if err := ensureMemoryFactsIndexStatusPatches(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.memory_facts_index_status"), loggateway.Err(err))
		return fmt.Errorf("memory facts index status patches: %w", err)
	}
	if err := ensureMessagesTurnNumberPatch(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.messages_turn_number"), loggateway.Err(err))
		return fmt.Errorf("messages turn_number patch: %w", err)
	}
	if err := EnsureSessionMemorySchema(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_memory"), loggateway.Err(err))
		return fmt.Errorf("session memory schema: %w", err)
	}
	if err := sessionmemory.EnsureMemoryRelationPatches(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.memory_relation"), loggateway.Err(err))
		return fmt.Errorf("memory relation patches: %w", err)
	}
	if err := sessionmemory.EnsureMonitorSchemaPatches(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.monitor"), loggateway.Err(err))
		return fmt.Errorf("monitor schema patches: %w", err)
	}
	if err := ensureAgentRuntimePatches(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.agent_runtime"), loggateway.Err(err))
		return fmt.Errorf("agent runtime patches: %w", err)
	}
	if err := ensureEntityReinforcementsSchema(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.entity_reinforcements"), loggateway.Err(err))
		return fmt.Errorf("entity reinforcements schema: %w", err)
	}
	if err := ensureCascadeSagaPatches(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.cascade_saga"), loggateway.Err(err))
		return fmt.Errorf("cascade saga patches: %w", err)
	}
	if err := ensureBuiltinPlatformTools(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.builtin_platform_tools"), loggateway.Err(err))
		return err
	}
	if err := ensureSystemSettingPatches(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.system_setting"), loggateway.Err(err))
		return fmt.Errorf("system setting patches: %w", err)
	}
	if err := ensurePricingRulePatches(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.pricing_rule"), loggateway.Err(err))
		return fmt.Errorf("pricing rule patches: %w", err)
	}
	if err := ensureLlmProviderModelCapabilityPatches(context.Background(), entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.llm_provider_model_cap"), loggateway.Err(err))
		return fmt.Errorf("llm provider model capability patches: %w", err)
	}
	if err := ensureDefaultSystemSetting(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.default_system_setting"), loggateway.Err(err))
		return err
	}
	if err := ensureDefaultCredentialEncryptionKey(context.Background(), entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.credential_enc_key"), loggateway.Err(err))
		return fmt.Errorf("credential encryption key: %w", err)
	}
	ctxSchema := context.Background()
	if err := EnsureEvalSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.eval"), loggateway.Err(err))
		return fmt.Errorf("eval schema: %w", err)
	}
	if err := EnsureA2ASchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.a2a"), loggateway.Err(err))
		return fmt.Errorf("a2a schema: %w", err)
	}
	if err := ensureA2ARemoteHealthPatches(ctxSchema, entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.a2a_remote_health"), loggateway.Err(err))
		return fmt.Errorf("a2a remote health patches: %w", err)
	}
	if err := ensureTeamRunSummaryPatches(ctxSchema, entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.team_run_summary"), loggateway.Err(err))
		return fmt.Errorf("team run summary patches: %w", err)
	}
	if err := ensureSessionRevisionPatches(ctxSchema, entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_revision"), loggateway.Err(err))
		return fmt.Errorf("session revision patches: %w", err)
	}
	if err := EnsurePluginRunSchema(ctxSchema, entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.plugin_run"), loggateway.Err(err))
		return fmt.Errorf("plugin run schema: %w", err)
	}
	if err := EnsureHookDeliverySchema(ctxSchema, entClient, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.hook_delivery"), loggateway.Err(err))
		return fmt.Errorf("hook delivery schema: %w", err)
	}
	if err := EnsureFlowLogSchema(ctxSchema, entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.flow_log"), loggateway.Err(err))
		return fmt.Errorf("flow log schema: %w", err)
	}
	if err := EnsureMessageFTSSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.message_fts"), loggateway.Err(err))
		return fmt.Errorf("message fts schema: %w", err)
	}
	if err := EnsureChannelInboundSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.channel_inbound"), loggateway.Err(err))
		return fmt.Errorf("channel inbound schema: %w", err)
	}
	if err := EnsureChannelTurnJobSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.channel_turn_job"), loggateway.Err(err))
		return fmt.Errorf("channel turn job schema: %w", err)
	}
	if err := EnsureChannelRuntimeLeaseSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.channel_runtime_lease"), loggateway.Err(err))
		return fmt.Errorf("channel runtime lease schema: %w", err)
	}
	if err := EnsureSessionRunSchema(ctxSchema, rawDB, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_run"), loggateway.Err(err))
		return fmt.Errorf("session run schema: %w", err)
	}
	if err := EnsureSessionParticipantSchema(ctxSchema, rawDB, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_participant"), loggateway.Err(err))
		return fmt.Errorf("session participant schema: %w", err)
	}
	if err := ensureSessionRunCheckpointSchema(ctxSchema, rawDB, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_run_checkpoint"), loggateway.Err(err))
		return fmt.Errorf("session run checkpoint schema: %w", err)
	}
	if err := ensureSessionRunColumnPatches(ctxSchema, rawDB, lg); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.session_run_col_patches"), loggateway.Err(err))
		return fmt.Errorf("session run column patches: %w", err)
	}
	if err := EnsureMonitorAlertSchema(ctxSchema, entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.monitor_alert"), loggateway.Err(err))
		return fmt.Errorf("monitor alert schema: %w", err)
	}
	if err := EnsureEcosystemSchema(ctxSchema, entClient); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.ecosystem"), loggateway.Err(err))
		return fmt.Errorf("ecosystem schema: %w", err)
	}
	if err := EnsureTeamGraphSessionSchema(ctxSchema, rawDB); err != nil {
		lg.Error("schema step failed", loggateway.StepID("data.schema.team_graph_session"), loggateway.Err(err))
		return fmt.Errorf("team graph session schema: %w", err)
	}
	return nil
}

// runPendingDataMigrations applies one-time data migrations (schema_migrations gate).
func runPendingDataMigrations(entClient *ent.Client, lg loggateway.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrated, skipped, err := RunLegacyTRPCMemoryMigration(ctx, sessionmemory.NewStore(entClient, lg), lg)
	if err != nil {
		lg.Error("migration step failed", loggateway.StepID("data.migration.legacy_trpc_memory"), loggateway.Err(err))
		return fmt.Errorf("legacy trpc memory backfill: %w", err)
	}
	if skipped {
		lg.Info("legacy trpc memory backfill skipped",
			loggateway.StepID("data.startup"), loggateway.Int("migration", MigrationLegacyTRPCMemoryFacts))
	} else if migrated > 0 {
		lg.Info("legacy trpc memory backfill migrated",
			loggateway.StepID("data.startup"), loggateway.Int("migrated", migrated))
	}
	if err := RunTurnIndexToTurnIDMigration(ctx, entClient, lg); err != nil {
		lg.Error("migration step failed", loggateway.StepID("data.migration.turn_index_to_turn_id"), loggateway.Err(err))
		return fmt.Errorf("turn_index migration: %w", err)
	}
	if err := RunSessionStatusIdleMigration(ctx, entClient, lg); err != nil {
		lg.Error("migration step failed", loggateway.StepID("data.migration.session_status_idle"), loggateway.Err(err))
		return fmt.Errorf("session status migration: %w", err)
	}
	return nil
}

// ensureAllSchemas applies DDL patches and pending data migrations (compat wrapper for tests).
func ensureAllSchemas(rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	if err := ensureSchemaDDL(rawDB, entClient, lg); err != nil {
		return err
	}
	return runPendingDataMigrations(entClient, lg)
}

// initPostgres opens the optional Postgres vector store connection.
func initPostgres(c *conf.Data, lg loggateway.Logger) (*sql.DB, error) {
	pgDSN := postgresVectorDSN(c)
	if pgDSN == "" {
		return nil, nil
	}
	pg, err := sql.Open("postgres", pgDSN)
	if err != nil {
		lg.Error("init postgres failed", loggateway.StepID("data.init_postgres"), loggateway.Str("step", "sql_open"), loggateway.Err(err))
		return nil, fmt.Errorf("failed opening postgres for vectors: %w", err)
	}
	pg.SetMaxOpenConns(8)
	pg.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = pg.PingContext(ctx); err != nil {
		pg.Close()
		lg.Error("init postgres failed", loggateway.StepID("data.init_postgres"), loggateway.Str("step", "ping"), loggateway.Err(err))
		return nil, fmt.Errorf("postgres ping: %w", err)
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

// seedInitialData seeds initial admin from config and optional dev bypass admin.
func seedInitialData(entClient *ent.Client, c *conf.Data, lg loggateway.Logger) error {
	if err := ensureInitialAdminFromConfig(context.Background(), entClient, c, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.initial_admin"), loggateway.Err(err))
		return err
	}
	if err := ensureDevBypassAdminIfEnabled(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.dev_admin"), loggateway.Err(err))
		return err
	}
	if err := ensureChannelPlatformAvatars(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.channel_avatars"), loggateway.Err(err))
		return err
	}
	if err := ensureAgentAvatars(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.agent_avatars"), loggateway.Err(err))
		return err
	}
	if err := SeedSystemAdminAgent(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.system_admin_agent"), loggateway.Err(err))
		return err
	}
	if err := SeedSpiritAgent(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_agent"), loggateway.Err(err))
		return err
	}
	if err := SeedBuiltinCLIAdminTools(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.cli_admin_tools"), loggateway.Err(err))
		return err
	}
	if err := SeedBuiltinIndustries(context.Background(), entClient); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.builtin_industries"), loggateway.Err(err))
		return err
	}
	scenarioDir := biz.ScenarioDir()
	if err := SeedBuiltinAgentCategories(context.Background(), entClient, scenarioDir, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.agent_categories"), loggateway.Err(err))
		return err
	}
	if err := SeedAgentTemplates(context.Background(), entClient, scenarioDir, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.agent_templates"), loggateway.Err(err))
		return err
	}
	return nil
}

func seedP1Data(entClient *ent.Client, c *conf.Data, d *Data) error {
	lg := d.lg
	if err := ensureChannelPlatformAvatars(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.channel_avatars"), loggateway.Err(err))
		return err
	}
	if err := ensureAgentAvatars(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.agent_avatars"), loggateway.Err(err))
		return err
	}
	if err := SeedSystemAdminAgent(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.system_admin_agent"), loggateway.Err(err))
		return err
	}
	if err := SeedSpiritAgent(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_agent"), loggateway.Err(err))
		return err
	}
	if err := SeedBuiltinCLIAdminTools(context.Background(), entClient, lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.cli_admin_tools"), loggateway.Err(err))
		return err
	}
	if err := SeedBuiltinIndustries(context.Background(), entClient); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.builtin_industries"), loggateway.Err(err))
		return err
	}

	scenarioDir := biz.ScenarioDir()
	d.lazySeeders = map[string]*LazySeeder{
		"agent_categories": NewLazySeeder(entClient, func(ctx context.Context, client *ent.Client) error {
			return SeedBuiltinAgentCategories(ctx, client, scenarioDir, lg)
		}, lg),
		"agent_templates": NewLazySeeder(entClient, func(ctx context.Context, client *ent.Client) error {
			return SeedAgentTemplates(ctx, client, scenarioDir, lg)
		}, lg),
	}

	return nil
}

// NewSessionMemoryStore exposes SQLite session-chain reads (L0–L4, evolution) on the same DB as Ent.
func NewSessionMemoryStore(d *Data) *sessionmemory.Store {
	if d == nil {
		return nil
	}
	return sessionmemory.NewStore(d.Ent(), d.lg)
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
	return repo.(*knowledgeRepo)
}

func NewEvalRepoFromData(d *Data) biz.EvalRepo {
	return NewEvalRepo(d, d.lg)
}

func NewA2ARepoFromData(d *Data, lg loggateway.Logger) biz.A2ARepo {
	return NewA2ARepo(d, lg)
}

// NewCLIData wraps SQLite handles opened by OpenSQLiteEntClient for offline maintenance CLIs.
func NewCLIData(client *ent.Client, rawDB *sql.DB, lg loggateway.Logger) *Data {
	return &Data{entClient: client, rawDB: rawDB, lg: lg}
}

// OpenSQLiteEntClient opens SQLite for offline CLI maintenance tools (e.g. memory-migrate).
// Do not use against a DSN while admin is running — use in-process NewData migrations instead.
func OpenSQLiteEntClient(dsn string) (*ent.Client, *sql.DB, func(), error) {
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

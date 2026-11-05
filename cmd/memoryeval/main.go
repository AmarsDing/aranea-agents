package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/decide"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"
)

// memoryeval is the standalone Agent Memory Challenge evaluation entrypoint.
// It exposes only the platform Add/Search contract over HTTP and reuses the
// existing L3 memory store; the main admin server is not involved.
//
// Environment:
//
//	EVAL_HTTP_ADDR        listen address (default ":8910")
//	EVAL_PG_SOURCE        Postgres DSN (pgvector-enabled); fallback DATABASE_URL
//	EVAL_VECTOR_DIM       embedding dimension (default 0 = server default 1536)
//	EVAL_MEMORY_TOKEN     Memory System Key for Bearer/X-Api-Key auth (empty = no auth, sandbox only)
//	EMBEDDING_PROVIDER    openai | ollama | gemini | huggingface (default openai)
//	EMBEDDING_BASE_URL    OpenAI-compatible embedding endpoint (empty = keyword-only recall)
//	EMBEDDING_API_KEY     embedding API key
//	EMBEDDING_MODEL       embedding model (default text-embedding-3-small)
//	EMBEDDING_DIM         embedding dimension (default 1536)
//	EVAL_SEARCH_AUDIT_PATH optional Search audit log path (query/user/count/size/latency)
//	EVAL_MEDIA_VLM_BASE_URL OpenAI-compatible vision endpoint; with EVAL_MEDIA_VLM_API_KEY
//	                        enables image captioning (default off = caption-only ack route)
//	EVAL_MEDIA_VLM_API_KEY  vision endpoint API key
//	EVAL_MEDIA_VLM_MODEL    caption model (default gpt-4o-mini, Cycle 2 academic-track rule)
//	EVAL_MEDIA_VLM_TIMEOUT_SEC / EVAL_MEDIA_VLM_MAX_CONCURRENCY /
//	EVAL_MEDIA_VLM_MAX_IMAGES / EVAL_MEDIA_VLM_BUDGET_SEC / EVAL_MEDIA_VLM_CACHE_ENTRIES
//	EVAL_SWEEP_INTERVAL_SEC  stale-index sweep interval seconds (default 30)
//	EVAL_SWEEP_BATCH         rows picked per sweep (default 50; batch-embedded 10/request)
//	EVAL_SWEEP_MAX_ATTEMPTS  per-row embed attempts before aging out (default 8)
//	ARANEA_MEM_SHADOW     on = T8 shadow audit chain (JEV Phase 0): double-record
//	                      VIR route/correction decisions + full state into the
//	                      M80 decision layer (default off, byte-identical behavior)
func main() {
	addr := envOr("EVAL_HTTP_ADDR", ":8910")
	token := os.Getenv("EVAL_MEMORY_TOKEN")
	pgSource := os.Getenv("EVAL_PG_SOURCE")
	if pgSource == "" {
		pgSource = os.Getenv("DATABASE_URL")
	}
	vectorDim, _ := strconv.Atoi(envOr("EVAL_VECTOR_DIM", "0"))

	pipe := logpipeline.NewPipeline(1024)
	pipe.AddSink(logpipeline.NewStdoutSink("info"))
	lg := loggateway.New(loggateway.LoggingConfig{Level: "info", Stdout: true}, pipe)
	defer func() { _ = pipe.Close() }()

	if pgSource == "" {
		lg.Error("EVAL_PG_SOURCE (or DATABASE_URL) is required", loggateway.StepID("memoryeval.config"))
		os.Exit(1)
	}
	if token == "" {
		lg.Warn("EVAL_MEMORY_TOKEN not set; Add/Search endpoints are unauthenticated (sandbox use only)",
			loggateway.StepID("memoryeval.config"))
	}

	d, cleanup, err := data.NewData(&conf.Data{
		Driver:   "postgres",
		Postgres: &conf.Data_Postgres{Source: pgSource, VectorDim: int32(vectorDim)},
	}, lg, knowledge.NewMemoryReranker(lg))
	if err != nil {
		lg.Error("init data failed", loggateway.StepID("memoryeval.startup"), loggateway.Err(err))
		os.Exit(1)
	}
	defer cleanup()

	var emb biz.EmbeddingService
	if baseURL, apiKey := os.Getenv("EMBEDDING_BASE_URL"), os.Getenv("EMBEDDING_API_KEY"); baseURL != "" && apiKey != "" {
		dim, _ := strconv.Atoi(envOr("EMBEDDING_DIM", "1536"))
		emb = singleEmbedder{e: knowledge.NewMultiProviderEmbedder(
			envOr("EMBEDDING_PROVIDER", "openai"), baseURL, apiKey,
			envOr("EMBEDDING_MODEL", "text-embedding-3-small"), dim, lg)}
		lg.Info("embedding enabled", loggateway.StepID("memoryeval.startup"))
	} else {
		lg.Warn("EMBEDDING_BASE_URL/EMBEDDING_API_KEY not set; recall degrades to keyword-only",
			loggateway.StepID("memoryeval.startup"))
	}

	evalStore := data.NewEvalMemoryStore(d, emb, lg)
	// T8 shadow 审计链（JEV Phase 0）：ARANEA_MEM_SHADOW=on 时装配 M80
	// outbox collector 并把 decide.Recorder 注入 eval store，双记 VIR
	// 路由/改口门决策与完整 state（kill_criteria 金标数据源）。缺省 off：
	// 不建 collector、不注入，零行为差异。shadow 记录依赖 VIR 向量路由，
	// emb 缺席时开关打开也只落零记录（store 内 router-nil 早退）。
	if os.Getenv("ARANEA_MEM_SHADOW") == "on" {
		repo := data.NewDecisionRepoFromData(d)
		lc := decision.NewOutboxCollector(repo, lg)
		lc.Start(context.Background())
		defer lc.Stop()
		if emb == nil {
			lg.Warn("ARANEA_MEM_SHADOW=on but no embedder; shadow records will be empty",
				loggateway.StepID("memoryeval.shadow"))
		}
		if sw, ok := evalStore.(decide.ShadowRecorderWired); ok {
			sw.SetShadowRecorder(decide.NewRecorder(lc, lg))
			lg.Info("mem shadow chain armed", loggateway.StepID("memoryeval.shadow"))
		} else {
			lg.Warn("eval store does not implement ShadowRecorderWired; shadow disabled",
				loggateway.StepID("memoryeval.shadow"))
		}
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           newEvalServer(evalStore, token, lg).routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// C3-C: embedding 索引补偿——Add 的 fire-and-forget 索引失败后事实永久停在
	// stale，sweeper 周期性重试到期批次，恢复向量召回；emb 为空时无向量可写不启动。
	// 2026-11-01 起旋钮经环境变量可调（AMC 种子洪峰 111 万积压需提速）：
	// EVAL_SWEEP_INTERVAL_SEC / EVAL_SWEEP_BATCH / EVAL_SWEEP_MAX_ATTEMPTS。
	var sweeperCancel context.CancelFunc
	if emb != nil {
		sweepIntervalSec, _ := strconv.Atoi(envOr("EVAL_SWEEP_INTERVAL_SEC", "30"))
		sweepBatch, _ := strconv.Atoi(envOr("EVAL_SWEEP_BATCH", "50"))
		sweepMaxAttempts, _ := strconv.Atoi(envOr("EVAL_SWEEP_MAX_ATTEMPTS", "8"))
		var sweepCtx context.Context
		sweepCtx, sweeperCancel = context.WithCancel(context.Background())
		sweeper := data.NewMemoryFactIndexSweeper(d,
			data.NewMemoryFactIndexSync(biz.NewMemoryUsecase(data.NewMemoryRepo(d), emb), d, lg),
			lg, time.Duration(sweepIntervalSec)*time.Second, sweepBatch, sweepMaxAttempts)
		safego.Go(sweepCtx, "memoryeval.index_sweeper", func() { _ = sweeper.Run(sweepCtx) })
		lg.Info("fact index sweeper armed", loggateway.StepID("memoryeval.startup"),
			loggateway.Int("interval_sec", sweepIntervalSec), loggateway.Int("batch", sweepBatch),
			loggateway.Int("max_attempts", sweepMaxAttempts))
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("memoryeval listening", loggateway.StepID("memoryeval.startup"), loggateway.Str("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		lg.Info("shutdown signal received", loggateway.StepID("memoryeval.shutdown"), loggateway.Str("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			lg.Error("http shutdown failed", loggateway.StepID("memoryeval.shutdown"), loggateway.Err(err))
			os.Exit(1)
		}
		if sweeperCancel != nil {
			sweeperCancel()
		}
	case err := <-errCh:
		if err != nil {
			lg.Error("http server failed", loggateway.StepID("memoryeval.shutdown"), loggateway.Err(err))
			os.Exit(1)
		}
	}
	lg.Info("memoryeval stopped", loggateway.StepID("memoryeval.shutdown"))
}

// singleEmbedder adapts the knowledge embedder onto biz.EmbeddingService AND
// biz.EmbeddingBatchService (AMC P3 评审 #4/#7：缺少批路径会让 VIR 锚点建
// 构与 per-message 原子嵌入退化为逐条串行往返——生产唯一服役 binary 上的
// 死路径，且违反 Add 热路径延迟契约)。
type singleEmbedder struct {
	e *knowledge.MultiProviderEmbedder
}

func (a singleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return a.e.EmbedSingle(ctx, text)
}

func (a singleEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return a.e.Embed(ctx, texts)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

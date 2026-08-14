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
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
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

	srv := &http.Server{
		Addr:              addr,
		Handler:           newEvalServer(data.NewEvalMemoryStore(d, emb, lg), token, lg).routes(),
		ReadHeaderTimeout: 10 * time.Second,
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
	case err := <-errCh:
		if err != nil {
			lg.Error("http server failed", loggateway.StepID("memoryeval.shutdown"), loggateway.Err(err))
			os.Exit(1)
		}
	}
	lg.Info("memoryeval stopped", loggateway.StepID("memoryeval.shutdown"))
}

// singleEmbedder adapts knowledge.QueryEmbedder onto biz.EmbeddingService.
type singleEmbedder struct{ e knowledge.QueryEmbedder }

func (a singleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return a.e.EmbedSingle(ctx, text)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

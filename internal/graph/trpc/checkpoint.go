package graph

import (
	"context"
	"database/sql"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphpg "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/postgres"
)

// CheckpointSaver is a wrapper around the framework's Postgres checkpoint saver.
// After A6, Postgres is the only supported backend for graph checkpoints.
type CheckpointSaver struct {
	saver trpcgraph.CheckpointSaver
	db    *sql.DB
	lg    loggateway.Logger
}

// NewCheckpointSaver creates a Postgres-backed checkpoint saver.
// pgDSN must be non-empty; production deployments require Postgres.
// The db must be a Postgres *sql.DB handle.
func NewCheckpointSaver(db *sql.DB, pgDSN string, lg loggateway.Logger) (*CheckpointSaver, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "graph checkpoint: db is nil")
	}
	if pgDSN == "" {
		return nil, apierror.BadRequest(apierror.DomainGraph, "graph checkpoint: pgDSN is empty (Postgres required after A6)")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	saver, err := trpcgraphpg.NewSaver(db)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, "graph checkpoint postgres init").WithCause(err)
	}
	return &CheckpointSaver{saver: saver, db: db, lg: lg}, nil
}

// Saver returns the underlying framework checkpoint saver.
func (s *CheckpointSaver) Saver() trpcgraph.CheckpointSaver {
	return s.saver
}

func (s *CheckpointSaver) Get(ctx context.Context, config map[string]any) (*trpcgraph.Checkpoint, error) {
	cp, err := s.saver.Get(ctx, config)
	if err != nil {
		s.lg.Warn("graph checkpoint get failed",
			loggateway.StepID("graph.checkpoint.get_fail"),
			loggateway.Err(err))
	}
	return cp, err
}

func (s *CheckpointSaver) GetTuple(ctx context.Context, config map[string]any) (*trpcgraph.CheckpointTuple, error) {
	return s.saver.GetTuple(ctx, config)
}

func (s *CheckpointSaver) List(ctx context.Context, config map[string]any, filter *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error) {
	return s.saver.List(ctx, config, filter)
}

func (s *CheckpointSaver) Put(ctx context.Context, req trpcgraph.PutRequest) (map[string]any, error) {
	result, err := s.saver.Put(ctx, req)
	if err != nil {
		s.lg.Warn("graph checkpoint put failed",
			loggateway.StepID("graph.checkpoint.put_fail"),
			loggateway.Err(err))
	} else {
		s.lg.Info("graph checkpoint saved",
			loggateway.StepID("graph.checkpoint.put"))
	}
	return result, err
}

func (s *CheckpointSaver) PutWrites(ctx context.Context, req trpcgraph.PutWritesRequest) error {
	return s.saver.PutWrites(ctx, req)
}

func (s *CheckpointSaver) PutFull(ctx context.Context, req trpcgraph.PutFullRequest) (map[string]any, error) {
	return s.saver.PutFull(ctx, req)
}

func (s *CheckpointSaver) DeleteLineage(ctx context.Context, lineageID string) error {
	return s.saver.DeleteLineage(ctx, lineageID)
}

func (s *CheckpointSaver) Close() error {
	return nil
}

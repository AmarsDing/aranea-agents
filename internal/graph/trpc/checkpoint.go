package graph

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphpg "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/postgres"
	trpcgraphsqlite "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/sqlite"
)

// CheckpointSaver is a dialect-aware wrapper around the framework's
// checkpoint saver. When pgDSN is non-empty, it uses the Postgres saver;
// otherwise it falls back to the SQLite saver.
type CheckpointSaver struct {
	saver trpcgraph.CheckpointSaver
	db    *sql.DB
	lg    loggateway.Logger
}

// NewCheckpointSaver creates a dialect-aware checkpoint saver.
// When pgDSN is non-empty, a Postgres-backed saver is created; otherwise SQLite.
// The db must be non-nil and must match the dialect (Postgres *sql.DB for pgDSN,
// SQLite *sql.DB otherwise).
func NewCheckpointSaver(db *sql.DB, pgDSN string, lg loggateway.Logger) (*CheckpointSaver, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "graph checkpoint: db is nil")
	}
	var saver trpcgraph.CheckpointSaver
	var err error
	if pgDSN != "" {
		saver, err = trpcgraphpg.NewSaver(db)
		if err != nil {
			return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph checkpoint postgres init: %v", err))
		}
	} else {
		saver, err = trpcgraphsqlite.NewSaver(db)
		if err != nil {
			return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph checkpoint sqlite init: %v", err))
		}
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

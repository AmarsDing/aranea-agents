package graph

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphsqlite "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/sqlite"
)

type SQLiteCheckpointSaver struct {
	saver *trpcgraphsqlite.Saver
	db    *sql.DB
	lg    loggateway.Logger
}

func NewSQLiteCheckpointSaver(db *sql.DB, lg loggateway.Logger) (*SQLiteCheckpointSaver, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "graph checkpoint: db is nil")
	}
	saver, err := trpcgraphsqlite.NewSaver(db)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph checkpoint sqlite init: %v", err))
	}
	return &SQLiteCheckpointSaver{saver: saver, db: db, lg: lg}, nil
}

func (s *SQLiteCheckpointSaver) Saver() trpcgraph.CheckpointSaver {
	return s.saver
}

func (s *SQLiteCheckpointSaver) Get(ctx context.Context, config map[string]any) (*trpcgraph.Checkpoint, error) {
	cp, err := s.saver.Get(ctx, config)
	if err != nil {
		s.lg.Warn("graph checkpoint get failed",
			loggateway.StepID("graph.checkpoint.get_fail"),
			loggateway.Err(err))
	}
	return cp, err
}

func (s *SQLiteCheckpointSaver) GetTuple(ctx context.Context, config map[string]any) (*trpcgraph.CheckpointTuple, error) {
	return s.saver.GetTuple(ctx, config)
}

func (s *SQLiteCheckpointSaver) List(ctx context.Context, config map[string]any, filter *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error) {
	return s.saver.List(ctx, config, filter)
}

func (s *SQLiteCheckpointSaver) Put(ctx context.Context, req trpcgraph.PutRequest) (map[string]any, error) {
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

func (s *SQLiteCheckpointSaver) PutWrites(ctx context.Context, req trpcgraph.PutWritesRequest) error {
	return s.saver.PutWrites(ctx, req)
}

func (s *SQLiteCheckpointSaver) PutFull(ctx context.Context, req trpcgraph.PutFullRequest) (map[string]any, error) {
	return s.saver.PutFull(ctx, req)
}

func (s *SQLiteCheckpointSaver) DeleteLineage(ctx context.Context, lineageID string) error {
	return s.saver.DeleteLineage(ctx, lineageID)
}

func (s *SQLiteCheckpointSaver) Close() error {
	return nil
}

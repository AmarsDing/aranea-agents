package graph

import (
	"context"
	"database/sql"
	"fmt"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphsqlite "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/sqlite"
)

type SQLiteCheckpointSaver struct {
	saver *trpcgraphsqlite.Saver
	db    *sql.DB
}

func NewSQLiteCheckpointSaver(db *sql.DB) (*SQLiteCheckpointSaver, error) {
	if db == nil {
		return nil, fmt.Errorf("graph checkpoint: db is nil")
	}
	saver, err := trpcgraphsqlite.NewSaver(db)
	if err != nil {
		return nil, fmt.Errorf("graph checkpoint sqlite init: %w", err)
	}
	return &SQLiteCheckpointSaver{saver: saver, db: db}, nil
}

func (s *SQLiteCheckpointSaver) Saver() trpcgraph.CheckpointSaver {
	return s.saver
}

func (s *SQLiteCheckpointSaver) Get(ctx context.Context, config map[string]any) (*trpcgraph.Checkpoint, error) {
	return s.saver.Get(ctx, config)
}

func (s *SQLiteCheckpointSaver) GetTuple(ctx context.Context, config map[string]any) (*trpcgraph.CheckpointTuple, error) {
	return s.saver.GetTuple(ctx, config)
}

func (s *SQLiteCheckpointSaver) List(ctx context.Context, config map[string]any, filter *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error) {
	return s.saver.List(ctx, config, filter)
}

func (s *SQLiteCheckpointSaver) Put(ctx context.Context, req trpcgraph.PutRequest) (map[string]any, error) {
	return s.saver.Put(ctx, req)
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

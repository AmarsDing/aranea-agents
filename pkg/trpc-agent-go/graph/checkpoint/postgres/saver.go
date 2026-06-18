//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.

// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package postgres provides Postgres-based checkpoint storage implementation
// for graph execution state persistence and recovery.
//
// This saver stores the entire checkpoint and metadata as JSON blobs in
// Postgres. It is suitable for production usage when paired with a
// persistent Postgres instance, and is the recommended saver for crash
// recovery (P1-8) because Postgres provides durable WAL-backed storage.
//
// The caller is responsible for registering the Postgres driver
// (e.g. via `import _ "github.com/lib/pq"`) before constructing a Saver.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/graph"
)

const (
	// Postgres table names and SQL statements.
	pgTableCheckpoints = "graph_checkpoints"
	pgTableWrites      = "graph_checkpoint_writes"

	pgCreateCheckpoints = "CREATE TABLE IF NOT EXISTS graph_checkpoints (" +
		"lineage_id TEXT NOT NULL, " +
		"checkpoint_ns TEXT NOT NULL, " +
		"checkpoint_id TEXT NOT NULL, " +
		"parent_checkpoint_id TEXT, " +
		"ts BIGINT NOT NULL, " +
		"checkpoint_json TEXT NOT NULL, " +
		"metadata_json TEXT NOT NULL, " +
		"PRIMARY KEY (lineage_id, checkpoint_ns, checkpoint_id)" +
		")"

	pgCreateWrites = "CREATE TABLE IF NOT EXISTS graph_checkpoint_writes (" +
		"lineage_id TEXT NOT NULL, " +
		"checkpoint_ns TEXT NOT NULL, " +
		"checkpoint_id TEXT NOT NULL, " +
		"task_id TEXT NOT NULL, " +
		"idx INTEGER NOT NULL, " +
		"channel TEXT NOT NULL, " +
		"value_json TEXT NOT NULL, " +
		"task_path TEXT, " +
		"seq BIGINT NOT NULL, " +
		"PRIMARY KEY (lineage_id, checkpoint_ns, checkpoint_id, task_id, idx)" +
		")"

	// Postgres uses ON CONFLICT ... DO UPDATE instead of INSERT OR REPLACE.
	pgInsertCheckpoint = "INSERT INTO graph_checkpoints (" +
		"lineage_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id, ts, " +
		"checkpoint_json, metadata_json) VALUES ($1, $2, $3, $4, $5, $6, $7) " +
		"ON CONFLICT (lineage_id, checkpoint_ns, checkpoint_id) DO UPDATE SET " +
		"parent_checkpoint_id = EXCLUDED.parent_checkpoint_id, " +
		"ts = EXCLUDED.ts, " +
		"checkpoint_json = EXCLUDED.checkpoint_json, " +
		"metadata_json = EXCLUDED.metadata_json"

	pgSelectLatest = "SELECT checkpoint_json, metadata_json, parent_checkpoint_id, checkpoint_ns, checkpoint_id " +
		"FROM graph_checkpoints WHERE lineage_id = $1 AND checkpoint_ns = $2 " +
		"ORDER BY ts DESC LIMIT 1"

	pgSelectByID = "SELECT checkpoint_json, metadata_json, parent_checkpoint_id, checkpoint_ns, checkpoint_id " +
		"FROM graph_checkpoints WHERE lineage_id = $1 AND checkpoint_ns = $2 AND checkpoint_id = $3 LIMIT 1"

	pgInsertWrite = "INSERT INTO graph_checkpoint_writes (" +
		"lineage_id, checkpoint_ns, checkpoint_id, task_id, idx, channel, value_json, task_path, seq) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) " +
		"ON CONFLICT (lineage_id, checkpoint_ns, checkpoint_id, task_id, idx) DO UPDATE SET " +
		"channel = EXCLUDED.channel, " +
		"value_json = EXCLUDED.value_json, " +
		"task_path = EXCLUDED.task_path, " +
		"seq = EXCLUDED.seq"

	pgSelectWrites = "SELECT task_id, idx, channel, value_json, task_path, seq FROM graph_checkpoint_writes " +
		"WHERE lineage_id = $1 AND checkpoint_ns = $2 AND checkpoint_id = $3 ORDER BY seq"

	pgDeleteLineageCkpts  = "DELETE FROM graph_checkpoints WHERE lineage_id = $1"
	pgDeleteLineageWrites = "DELETE FROM graph_checkpoint_writes WHERE lineage_id = $1"

	pgSelectNamespaceByID = "SELECT checkpoint_ns FROM graph_checkpoints " +
		"WHERE lineage_id = $1 AND checkpoint_id = $2 LIMIT 1"
)

// Saver is a Postgres-backed implementation of graph.CheckpointSaver.
// It expects an initialized *sql.DB connected to a Postgres instance.
// The caller must register the Postgres driver before construction.
// The constructor creates the required schema if needed.
type Saver struct {
	db *sql.DB
}

// NewSaver creates a new saver using the provided DB.
// The DB must use a Postgres driver. The constructor creates tables if needed.
func NewSaver(db *sql.DB) (*Saver, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if _, err := db.Exec(pgCreateCheckpoints); err != nil {
		return nil, fmt.Errorf("create checkpoints table: %w", err)
	}
	if _, err := db.Exec(pgCreateWrites); err != nil {
		return nil, fmt.Errorf("create writes table: %w", err)
	}
	return &Saver{db: db}, nil
}

// Get returns the checkpoint for the given config.
func (s *Saver) Get(ctx context.Context, config map[string]any) (*graph.Checkpoint, error) {
	t, err := s.GetTuple(ctx, config)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	return t.Checkpoint, nil
}

// GetTuple returns the checkpoint tuple for the given config.
func (s *Saver) GetTuple(ctx context.Context, config map[string]any) (*graph.CheckpointTuple, error) {
	lineageID := graph.GetLineageID(config)
	checkpointNS := graph.GetNamespace(config)
	checkpointID := graph.GetCheckpointID(config)
	if lineageID == "" {
		return nil, errors.New("lineage_id is required")
	}

	row, err := s.queryCheckpointData(ctx, lineageID, checkpointNS, checkpointID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	nsForTuple := checkpointNS
	if checkpointNS == "" {
		nsForTuple = row.namespace
	}

	return s.buildTuple(ctx, lineageID, nsForTuple, row.checkpointID, row.parentID,
		row.checkpointJSON, row.metadataJSON)
}

type checkpointRow struct {
	checkpointJSON []byte
	metadataJSON   []byte
	parentID       string
	checkpointID   string
	namespace      string
}

func (s *Saver) queryCheckpointData(ctx context.Context, lineageID, checkpointNS,
	checkpointID string) (*checkpointRow, error) {

	var r checkpointRow
	var err error
	if checkpointID == "" {
		err = s.db.QueryRowContext(ctx, pgSelectLatest, lineageID, checkpointNS).
			Scan(&r.checkpointJSON, &r.metadataJSON, &r.parentID, &r.namespace, &r.checkpointID)
	} else {
		err = s.db.QueryRowContext(ctx, pgSelectByID, lineageID, checkpointNS, checkpointID).
			Scan(&r.checkpointJSON, &r.metadataJSON, &r.parentID, &r.namespace, &r.checkpointID)
	}
	if err != nil {
		return nil, fmt.Errorf("select checkpoint failed: %w", err)
	}
	return &r, nil
}

func (s *Saver) buildTuple(ctx context.Context, lineageID, checkpointNS, checkpointID,
	parentID string, checkpointJSON, metadataJSON []byte) (*graph.CheckpointTuple, error) {

	var ckpt graph.Checkpoint
	if err := json.Unmarshal(checkpointJSON, &ckpt); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	var meta graph.CheckpointMetadata
	if err := json.Unmarshal(metadataJSON, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	cfg := graph.CreateCheckpointConfig(lineageID, checkpointID, checkpointNS)
	writes, err := s.loadWrites(ctx, lineageID, checkpointNS, checkpointID)
	if err != nil {
		return nil, err
	}

	var parentCfg map[string]any
	if parentID != "" {
		parentNS, err := s.findCheckpointNamespace(ctx, lineageID, parentID)
		if err != nil {
			return nil, err
		}
		parentCfg = graph.CreateCheckpointConfig(lineageID, parentID, parentNS)
	}
	return &graph.CheckpointTuple{
		Config:        cfg,
		Checkpoint:    &ckpt,
		Metadata:      &meta,
		ParentConfig:  parentCfg,
		PendingWrites: writes,
	}, nil
}

func (s *Saver) findCheckpointNamespace(ctx context.Context, lineageID, checkpointID string) (string, error) {
	if checkpointID == "" || lineageID == "" {
		return "", nil
	}
	var ns string
	if err := s.db.QueryRowContext(ctx, pgSelectNamespaceByID, lineageID, checkpointID).Scan(&ns); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("lookup parent namespace: %w", err)
	}
	return ns, nil
}

// List returns checkpoints for the lineage/namespace, with optional filters.
func (s *Saver) List(
	ctx context.Context,
	config map[string]any,
	filter *graph.CheckpointFilter,
) ([]*graph.CheckpointTuple, error) {
	lineageID := graph.GetLineageID(config)
	checkpointNS := graph.GetNamespace(config)
	if lineageID == "" {
		return nil, errors.New("lineage_id is required")
	}

	beforeTs, err := s.getBeforeTimestamp(ctx, lineageID, checkpointNS, filter)
	if err != nil {
		return nil, err
	}

	rows, err := s.executeListQuery(ctx, lineageID, checkpointNS, beforeTs, filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.processListResults(ctx, rows, lineageID, checkpointNS, filter)
}

func (s *Saver) getBeforeTimestamp(ctx context.Context, lineageID, checkpointNS string,
	filter *graph.CheckpointFilter) (*int64, error) {

	if filter == nil || filter.Before == nil {
		return nil, nil
	}
	beforeID := graph.GetCheckpointID(filter.Before)
	if beforeID == "" {
		return nil, nil
	}

	var ts int64
	var err error
	if checkpointNS == "" {
		err = s.db.QueryRowContext(ctx,
			"SELECT ts FROM graph_checkpoints WHERE lineage_id=$1 AND checkpoint_id=$2 ORDER BY ts DESC LIMIT 1",
			lineageID, beforeID,
		).Scan(&ts)
	} else {
		err = s.db.QueryRowContext(ctx,
			"SELECT ts FROM graph_checkpoints WHERE lineage_id=$1 AND checkpoint_ns=$2 AND checkpoint_id=$3 LIMIT 1",
			lineageID, checkpointNS, beforeID,
		).Scan(&ts)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get before timestamp: %w", err)
	}
	return &ts, nil
}

func (s *Saver) executeListQuery(ctx context.Context, lineageID, checkpointNS string,
	beforeTs *int64, filter *graph.CheckpointFilter) (*sql.Rows, error) {

	var q string
	var args []any
	argIdx := 1

	if checkpointNS == "" {
		q = "SELECT checkpoint_id, checkpoint_ns, ts FROM graph_checkpoints WHERE lineage_id=$1"
		args = []any{lineageID}
		argIdx = 2
	} else {
		q = "SELECT checkpoint_id, ts FROM graph_checkpoints WHERE lineage_id=$1 AND checkpoint_ns=$2"
		args = []any{lineageID, checkpointNS}
		argIdx = 3
	}

	if beforeTs != nil {
		q += fmt.Sprintf(" AND ts < $%d", argIdx)
		args = append(args, *beforeTs)
		argIdx++
	}

	q += " ORDER BY ts DESC"

	if filter != nil && filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("select checkpoints: %w", err)
	}
	return rows, nil
}

func (s *Saver) processListResults(ctx context.Context, rows *sql.Rows,
	lineageID, checkpointNS string, filter *graph.CheckpointFilter) ([]*graph.CheckpointTuple, error) {

	var tuples []*graph.CheckpointTuple
	for rows.Next() {
		tuple, err := s.processSingleRow(ctx, rows, lineageID, checkpointNS)
		if err != nil {
			return nil, err
		}
		if tuple == nil {
			continue
		}
		if !s.matchesMetadataFilter(tuple, filter) {
			continue
		}
		tuples = append(tuples, tuple)
		if filter != nil && filter.Limit > 0 && len(tuples) >= filter.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter checkpoints: %w", err)
	}
	return tuples, nil
}

func (s *Saver) processSingleRow(ctx context.Context, rows *sql.Rows,
	lineageID, checkpointNS string) (*graph.CheckpointTuple, error) {

	var checkpointID string
	var ts int64
	if checkpointNS == "" {
		var ns string
		if err := rows.Scan(&checkpointID, &ns, &ts); err != nil {
			return nil, fmt.Errorf("scan checkpoint (cross-ns): %w", err)
		}
		cfg := graph.CreateCheckpointConfig(lineageID, checkpointID, ns)
		return s.GetTuple(ctx, cfg)
	}
	if err := rows.Scan(&checkpointID, &ts); err != nil {
		return nil, fmt.Errorf("scan checkpoint: %w", err)
	}
	cfg := graph.CreateCheckpointConfig(lineageID, checkpointID, checkpointNS)
	return s.GetTuple(ctx, cfg)
}

func (s *Saver) matchesMetadataFilter(tuple *graph.CheckpointTuple, filter *graph.CheckpointFilter) bool {
	if filter == nil || len(filter.Metadata) == 0 {
		return true
	}
	if tuple.Metadata == nil || tuple.Metadata.Extra == nil {
		return false
	}
	for key, value := range filter.Metadata {
		if tuple.Metadata.Extra[key] != value {
			return false
		}
	}
	return true
}

// Put stores the checkpoint and returns the updated config with checkpoint ID.
func (s *Saver) Put(ctx context.Context, req graph.PutRequest) (map[string]any, error) {
	if req.Checkpoint == nil {
		return nil, errors.New("checkpoint cannot be nil")
	}
	lineageID := graph.GetLineageID(req.Config)
	checkpointNS := graph.GetNamespace(req.Config)
	if lineageID == "" {
		return nil, errors.New("lineage_id is required")
	}
	parentID := req.Checkpoint.ParentCheckpointID
	checkpointJSON, err := json.Marshal(req.Checkpoint)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}
	if req.Metadata == nil {
		req.Metadata = &graph.CheckpointMetadata{Source: graph.CheckpointSourceUpdate, Step: 0}
	}
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	ts := req.Checkpoint.Timestamp.UnixNano()
	if ts == 0 {
		ts = time.Now().UTC().UnixNano()
	}
	_, err = s.db.ExecContext(
		ctx, pgInsertCheckpoint,
		lineageID, checkpointNS, req.Checkpoint.ID, parentID, ts, checkpointJSON, metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint: %w", err)
	}
	return graph.CreateCheckpointConfig(lineageID, req.Checkpoint.ID, checkpointNS), nil
}

// PutWrites stores write entries for a checkpoint.
func (s *Saver) PutWrites(ctx context.Context, req graph.PutWritesRequest) error {
	lineageID := graph.GetLineageID(req.Config)
	checkpointNS := graph.GetNamespace(req.Config)
	checkpointID := graph.GetCheckpointID(req.Config)
	if lineageID == "" || checkpointID == "" {
		return errors.New("lineage_id and checkpoint_id are required")
	}
	for idx, w := range req.Writes {
		valueJSON, err := json.Marshal(w.Value)
		if err != nil {
			return fmt.Errorf("marshal write: %w", err)
		}
		seq := w.Sequence
		if seq == 0 {
			seq = int64(idx)
		}
		_, err = s.db.ExecContext(
			ctx, pgInsertWrite,
			lineageID, checkpointNS, checkpointID, req.TaskID, idx, w.Channel, valueJSON, req.TaskPath, seq,
		)
		if err != nil {
			return fmt.Errorf("insert write: %w", err)
		}
	}
	return nil
}

// PutFull atomically stores a checkpoint with its pending writes in a single transaction.
func (s *Saver) PutFull(ctx context.Context, req graph.PutFullRequest) (map[string]any, error) {
	lineageID := graph.GetLineageID(req.Config)
	checkpointNS := graph.GetNamespace(req.Config)
	if lineageID == "" {
		return nil, errors.New("lineage_id is required")
	}
	if req.Checkpoint == nil {
		return nil, errors.New("checkpoint cannot be nil")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	checkpointJSON, err := json.Marshal(req.Checkpoint)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	parentID := req.Checkpoint.ParentCheckpointID
	_, err = tx.ExecContext(
		ctx, pgInsertCheckpoint,
		lineageID, checkpointNS, req.Checkpoint.ID, parentID,
		req.Checkpoint.Timestamp.UnixNano(), checkpointJSON, metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint: %w", err)
	}

	for idx, w := range req.PendingWrites {
		valueJSON, err := json.Marshal(w.Value)
		if err != nil {
			return nil, fmt.Errorf("marshal write value: %w", err)
		}
		seq := w.Sequence
		if seq == 0 {
			seq = time.Now().UnixNano()
		}
		_, err = tx.ExecContext(
			ctx, pgInsertWrite,
			lineageID, checkpointNS, req.Checkpoint.ID, w.TaskID, idx, w.Channel, valueJSON, "", seq,
		)
		if err != nil {
			return nil, fmt.Errorf("insert write: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return graph.CreateCheckpointConfig(lineageID, req.Checkpoint.ID, checkpointNS), nil
}

// DeleteLineage deletes all checkpoints and writes for the lineage.
func (s *Saver) DeleteLineage(ctx context.Context, lineageID string) error {
	if lineageID == "" {
		return errors.New("lineage_id is required")
	}
	if _, err := s.db.ExecContext(ctx, pgDeleteLineageCkpts, lineageID); err != nil {
		return fmt.Errorf("delete checkpoints: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, pgDeleteLineageWrites, lineageID); err != nil {
		return fmt.Errorf("delete writes: %w", err)
	}
	return nil
}

// Close releases resources held by the saver. The underlying *sql.DB is
// owned by the caller and is NOT closed here (matching sqlite.Saver
// semantics); callers should close the DB separately.
func (s *Saver) Close() error {
	return nil
}

func (s *Saver) loadWrites(
	ctx context.Context,
	lineageID, checkpointNS, checkpointID string,
) ([]graph.PendingWrite, error) {
	rows, err := s.db.QueryContext(
		ctx, pgSelectWrites, lineageID, checkpointNS, checkpointID,
	)
	if err != nil {
		return nil, fmt.Errorf("select writes: %w", err)
	}
	defer rows.Close()
	var writes []graph.PendingWrite
	for rows.Next() {
		var taskID string
		var idx int
		var channel string
		var valueJSON []byte
		var taskPath string
		var seq int64
		if err := rows.Scan(&taskID, &idx, &channel, &valueJSON, &taskPath, &seq); err != nil {
			return nil, fmt.Errorf("scan write: %w", err)
		}
		var value any
		if err := json.Unmarshal(valueJSON, &value); err != nil {
			return nil, fmt.Errorf("unmarshal write: %w", err)
		}
		writes = append(writes, graph.PendingWrite{
			Channel:  channel,
			Value:    value,
			TaskID:   taskID,
			Sequence: seq,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter writes: %w", err)
	}
	return writes, nil
}

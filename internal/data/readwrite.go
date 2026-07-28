package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

// ReadWriteClient encapsulates read/write Ent client selection with automatic
// transaction awareness. Read operations use the read-only client (readClient),
// Write operations use the write client (entClient). Both fall back to the
// transaction client when a transaction is active in the context.
type ReadWriteClient struct {
	write *ent.Client // entClient (MaxOpenConns=16, Postgres write pool)
	read  *ent.Client // readClient (MaxOpenConns=32, Postgres read pool)
}

// NewReadWriteClient creates a ReadWriteClient with the given write and read Ent clients.
func NewReadWriteClient(write, read *ent.Client) *ReadWriteClient {
	return &ReadWriteClient{write: write, read: read}
}

// Read returns the appropriate Ent client for read operations.
// If a transaction is active in the context, it returns the transaction's client.
// Otherwise, it returns the read-only client.
func (c *ReadWriteClient) Read(ctx context.Context) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return c.read
}

// Write returns the appropriate Ent client for write operations.
// If a transaction is active in the context, it returns the transaction's client.
// Otherwise, it returns the write client.
func (c *ReadWriteClient) Write(ctx context.Context) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return c.write
}

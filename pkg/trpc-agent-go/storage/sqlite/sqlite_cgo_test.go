//go:build cgo

//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// SQLite treats most non-empty DSNs as paths; an incomplete file URI should fail open or ping.
func TestDefaultClientBuilder_InvalidDSN(t *testing.T) {
	_, err := defaultClientBuilder(context.Background(), WithClientConnString("file:"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "sqlite")
}

func TestDefaultClientBuilder_MemoryPing(t *testing.T) {
	client, err := defaultClientBuilder(context.Background(), WithClientConnString(":memory:"))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestDefaultClientBuilder_PingError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := defaultClientBuilder(ctx, WithClientConnString(":memory:"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "sqlite")
}

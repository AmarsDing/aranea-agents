package session

import (
	"net/http"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/wire"
)

// ProviderSet wires session runtime + compression (trpc session.Service backed).
var ProviderSet = wire.NewSet(
	NewRuntime,
	NewCompressor,
	wire.Bind(new(biz.NativeTurnCompressor), new(*Compressor)),
	wire.Bind(new(biz.ManualCompressor), new(*Compressor)),
	wire.Bind(new(biz.RunnerSnapshotSync), new(*Runtime)),
)

// NewCompressHTTPClient returns the HTTP client used by LLM compression calls.
func NewCompressHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

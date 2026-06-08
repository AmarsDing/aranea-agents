package session

import (
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/wire"
)

// ProviderSet wires session runtime + compression (trpc session.Service backed).
var ProviderSet = wire.NewSet(
	NewRuntime,
	NewCompressor,
	ProvideCompressorConfig,
	wire.Bind(new(biz.NativeTurnCompressor), new(*Compressor)),
	wire.Bind(new(biz.ManualCompressor), new(*Compressor)),
	wire.Bind(new(biz.CompressStatusReader), new(*Compressor)),
	wire.Bind(new(biz.RunnerSnapshotSync), new(*Runtime)),
)

// ProvideCompressorConfig assembles the CompressorConfig from individual dependencies.
func ProvideCompressorConfig(
	readDeps CompressReadDeps,
	writeDeps CompressWriteDeps,
	txDeps CompressTxDeps,
	agents AgentKeyLookup,
	runtime *Runtime,
	memory MemoryResync,
	comp compress.Compressor,
	eventBus event.Bus,
	memoryReader biz.MemoryFactReader,
	l1Reader biz.L1AdminReader,
	lg loggateway.Logger,
) CompressorConfig {
	return CompressorConfig{
		ReadDeps:     readDeps,
		WriteDeps:    writeDeps,
		TxDeps:       txDeps,
		Agents:       agents,
		Runtime:      runtime,
		Memory:       memory,
		Compress:     comp,
		EventBus:     eventBus,
		MemoryReader: memoryReader,
		L1Reader:     l1Reader,
		Logger:       lg,
	}
}

// NewCompressHTTPClient returns the HTTP client used by LLM compression calls.
func NewCompressHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

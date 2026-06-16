module trpc.group/trpc-go/trpc-agent-go-benchmark/memory/trpc-agent-go-impl

go 1.24.0

replace (
	trpc.group/trpc-go/trpc-agent-go => github.com/trpc-group/trpc-agent-go v1.7.1-0.20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/memory/mysql => github.com/trpc-group/trpc-agent-go/memory/mysql v1.7.1-0.20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/memory/pgvector => github.com/trpc-group/trpc-agent-go/memory/pgvector v1.7.1-0.20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/memory/sqlite => github.com/trpc-group/trpc-agent-go/memory/sqlite v1.7.1-0.20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/memory/sqlitevec => github.com/trpc-group/trpc-agent-go/memory/sqlitevec v1.7.1-0.20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/session/pgvector => github.com/trpc-group/trpc-agent-go/session/pgvector v0.0.0-20260402032440-a4e36132659f
	trpc.group/trpc-go/trpc-agent-go/storage/postgres => github.com/trpc-group/trpc-agent-go/storage/postgres v1.7.1-0.20260402032440-a4e36132659f
)

require (
	github.com/mattn/go-sqlite3 v1.14.32
	trpc.group/trpc-go/trpc-agent-go v1.7.0
	trpc.group/trpc-go/trpc-agent-go/memory/mysql v1.7.0
	trpc.group/trpc-go/trpc-agent-go/memory/pgvector v1.7.0
	trpc.group/trpc-go/trpc-agent-go/memory/sqlite v1.7.0
	trpc.group/trpc-go/trpc-agent-go/memory/sqlitevec v1.7.0
	trpc.group/trpc-go/trpc-agent-go/session/pgvector v1.7.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/asg017/sqlite-vec-go-bindings v0.1.6 // indirect
	github.com/bmatcuk/doublestar/v4 v4.9.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/go-ego/gse v1.0.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.22.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/ncruces/go-sqlite3 v0.32.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/openai/openai-go v1.12.0 // indirect
	github.com/panjf2000/ants/v2 v2.10.0 // indirect
	github.com/pgvector/pgvector-go v0.2.3 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/vcaesar/cedar v0.20.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.29.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.29.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.29.0 // indirect
	go.opentelemetry.io/otel/metric v1.41.0 // indirect
	go.opentelemetry.io/otel/sdk v1.41.0 // indirect
	go.opentelemetry.io/otel/trace v1.41.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240822170219-fc7c04adadcd // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240822170219-fc7c04adadcd // indirect
	google.golang.org/grpc v1.65.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	trpc.group/trpc-go/trpc-a2a-go v0.2.5 // indirect
	trpc.group/trpc-go/trpc-agent-go/storage/mysql v0.0.0-20251126064502-c8c2594d2519 // indirect
	trpc.group/trpc-go/trpc-agent-go/storage/postgres v1.6.0 // indirect
)

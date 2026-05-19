module aranea-agents

go 1.25.0

toolchain go1.26.1

// Agent runtime migrations must resolve to the vendored framework source.
replace trpc.group/trpc-go/trpc-agent-go => ./pkg/trpc-agent-go

replace trpc.group/trpc-go/trpc-agent-go/model/hunyuan => ./pkg/trpc-agent-go/model/hunyuan

replace trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf => ./pkg/trpc-agent-go/knowledge/document/reader/pdf

require (
	entgo.io/ent v0.14.5
	github.com/fsnotify/fsnotify v1.6.0
	github.com/glebarez/go-sqlite v1.22.0
	github.com/go-kratos/aip-go/ents v0.0.0-20251213081434-74ffa1fc1588
	github.com/go-kratos/kratos/v2 v2.9.2
	github.com/go-sql-driver/mysql v1.9.3
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.6.0
	github.com/gorilla/mux v1.8.1
	github.com/lib/pq v1.12.3
	github.com/pgvector/pgvector-go v0.3.0
	github.com/prometheus/client_golang v1.23.2
	github.com/xeipuuv/gojsonschema v1.2.0
	go.einride.tech/aip v0.76.0
	go.opentelemetry.io/otel v1.40.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0
	go.opentelemetry.io/otel/sdk v1.40.0
	go.opentelemetry.io/otel/trace v1.40.0
	go.uber.org/automaxprocs v1.5.1
	google.golang.org/genai v1.40.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260316180232-0b37fe3546d5
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
	trpc.group/trpc-go/trpc-agent-go v1.9.1
	trpc.group/trpc-go/trpc-agent-go/evaluation v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/anthropic v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/gemini v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/ollama v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/provider v1.9.0
	trpc.group/trpc-go/trpc-agent-go/session/sqlite v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/arxivsearch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/claudecode v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/email v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/google v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/openapi v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/wikipedia v1.9.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.18.2 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.14 // indirect
	github.com/googleapis/gax-go/v2 v2.18.0 // indirect
	github.com/gorilla/websocket v1.5.3
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.63.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
)

require (
	ariga.io/atlas v0.32.1-0.20250325101103-175b25e1c1b9 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	dario.cat/mergo v1.0.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/JohannesKaufmann/dom v0.2.0 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.0 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/anthropics/anthropic-sdk-go v1.37.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/bmatcuk/doublestar/v4 v4.9.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/getkin/kin-openapi v0.133.0 // indirect
	github.com/go-kratos/aegis v0.2.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/inflect v0.21.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-playground/form/v4 v4.3.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl/v2 v2.23.0 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.0 // indirect
	github.com/hhrutter/tiff v1.0.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/neurosnap/sentences v1.1.2 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/ollama/ollama v0.16.3 // indirect
	github.com/openai/openai-go v1.12.0 // indirect
	github.com/panjf2000/ants/v2 v2.10.0 // indirect
	github.com/pdfcpu/pdfcpu v0.11.1 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/wneessen/go-mail v0.7.2 // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yuin/goldmark v1.7.13 // indirect
	github.com/zclconf/go-cty v1.16.2 // indirect
	github.com/zclconf/go-cty-yaml v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric v0.42.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v0.42.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v0.42.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.40.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/image v0.32.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	google.golang.org/api v0.267.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260316180232-0b37fe3546d5 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/gorm v1.31.0 // indirect
	modernc.org/libc v1.37.6 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.7.2 // indirect
	modernc.org/sqlite v1.28.0 // indirect
	trpc.group/trpc-go/trpc-a2a-go v0.2.5 // indirect
	trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf v0.0.0-20251126064502-c8c2594d2519 // indirect
	trpc.group/trpc-go/trpc-mcp-go v0.0.10 // indirect
)

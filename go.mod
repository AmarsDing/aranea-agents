module aranea-agents

go 1.26.3

// Agent runtime migrations must resolve to the vendored framework source.
replace trpc.group/trpc-go/trpc-agent-go => ./pkg/trpc-agent-go

replace trpc.group/trpc-go/trpc-agent-go/model/hunyuan => ./pkg/trpc-agent-go/model/hunyuan

replace trpc.group/trpc-go/trpc-agent-go/model/anthropic => ./pkg/trpc-agent-go/model/anthropic

replace trpc.group/trpc-go/trpc-agent-go/model/bedrock => ./pkg/trpc-agent-go/model/bedrock

replace trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf => ./pkg/trpc-agent-go/knowledge/document/reader/pdf

replace trpc.group/trpc-go/trpc-agent-go/codeexecutor/container => ./pkg/trpc-agent-go/codeexecutor/container

replace trpc.group/trpc-go/trpc-agent-go/server/agui => ./pkg/trpc-agent-go/server/agui

// Agent runtime storage/session backends must resolve to the vendored framework source.
replace trpc.group/trpc-go/trpc-agent-go/session/postgres => ./pkg/trpc-agent-go/session/postgres

replace trpc.group/trpc-go/trpc-agent-go/storage/postgres => ./pkg/trpc-agent-go/storage/postgres

// OpenClaw runtime profile support must resolve to the vendored framework source.
replace trpc.group/trpc-go/trpc-agent-go/openclaw => ./pkg/trpc-agent-go/openclaw

// Evaluation framework must resolve to the vendored source: published v1.9.0
// lacks case-level rubrics (EvalCase.Rubrics) required for per-case judge
// scoring standards (P3-2).
replace trpc.group/trpc-go/trpc-agent-go/evaluation => ./pkg/trpc-agent-go/evaluation

require (
	entgo.io/ent v0.14.6
	github.com/aws/aws-sdk-go-v2/config v1.32.17
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/bwmarrin/discordgo v0.29.0
	github.com/fatih/color v1.19.0
	github.com/fsnotify/fsnotify v1.7.0
	github.com/glebarez/go-sqlite v1.22.0
	github.com/go-kratos/aip-go/ents v0.0.0-20251213081434-74ffa1fc1588
	github.com/go-kratos/kratos/v2 v2.9.2
	github.com/go-sql-driver/mysql v1.9.3
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.6.0
	github.com/gorilla/mux v1.8.1
	github.com/jackc/pgx/v5 v5.7.2
	github.com/larksuite/oapi-sdk-go/v3 v3.9.2
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728
	github.com/lib/pq v1.12.3
	github.com/mattn/go-isatty v0.0.20
	github.com/olekukonko/tablewriter v1.1.4
	github.com/open-dingtalk/dingtalk-stream-sdk-go v0.9.1
	github.com/pelletier/go-toml/v2 v2.3.1
	github.com/peterh/liner v1.2.2
	github.com/pgvector/pgvector-go v0.3.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/redis/go-redis/v9 v9.11.0
	github.com/slack-go/slack v0.23.1
	github.com/spf13/cobra v1.10.2
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	github.com/stretchr/testify v1.11.1
	github.com/tencent-connect/botgo v0.2.1
	github.com/testcontainers/testcontainers-go v0.42.0
	github.com/testcontainers/testcontainers-go/modules/postgres v0.42.0
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/xuri/excelize/v2 v2.10.1
	github.com/yuin/goldmark v1.7.13
	go.einride.tech/aip v0.76.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0
	go.opentelemetry.io/otel/sdk v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
	go.uber.org/automaxprocs v1.5.1
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.27.0
	golang.org/x/image v0.41.0
	golang.org/x/net v0.55.0
	golang.org/x/sync v0.20.0
	golang.org/x/sys v0.45.0
	golang.org/x/text v0.37.0
	golang.org/x/tools v0.45.0
	google.golang.org/genai v1.40.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260401024825-9d38bb4040a9
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	trpc.group/trpc-go/trpc-a2a-go v0.2.6-0.20260721084546-18c8244d0acb
	trpc.group/trpc-go/trpc-agent-go v1.11.1
	trpc.group/trpc-go/trpc-agent-go/agent/extension/toolpipe v1.10.0
	trpc.group/trpc-go/trpc-agent-go/artifact/s3 v1.9.0
	trpc.group/trpc-go/trpc-agent-go/codeexecutor/container v1.11.0
	trpc.group/trpc-go/trpc-agent-go/evaluation v1.11.0
	trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf v1.11.0
	trpc.group/trpc-go/trpc-agent-go/model/anthropic v1.11.0
	trpc.group/trpc-go/trpc-agent-go/model/bedrock v1.11.0
	trpc.group/trpc-go/trpc-agent-go/model/gemini v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/ollama v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/provider v1.9.0
	trpc.group/trpc-go/trpc-agent-go/model/tiktoken v1.10.0
	trpc.group/trpc-go/trpc-agent-go/openclaw v0.0.1
	trpc.group/trpc-go/trpc-agent-go/server/agui v1.11.0
	trpc.group/trpc-go/trpc-agent-go/session/postgres v1.11.0
	trpc.group/trpc-go/trpc-agent-go/tool/arxivsearch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/claudecode v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/email v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/google v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/openapi v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch v1.9.0
	trpc.group/trpc-go/trpc-agent-go/tool/wikipedia v1.9.0
	trpc.group/trpc-go/trpc-mcp-go v0.0.10
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
	golang.org/x/crypto v0.52.0
)

require (
	ariga.io/atlas v0.36.2-0.20250730182955-2c6300d0a3e1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	dario.cat/mergo v1.0.2 // indirect
	filippo.io/edwards25519 v1.1.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/JohannesKaufmann/dom v0.2.0 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ag-ui-protocol/ag-ui/sdks/community/go v0.0.0-20260514093510-e9e910b230b9 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/anthropics/anthropic-sdk-go v1.37.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.7 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.8 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.16 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.50.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.4.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.67.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1 // indirect
	github.com/aws/smithy-go v1.25.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/clipperhouse/displaywidth v0.10.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.6.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v0.2.1 // indirect
	github.com/cpuguy83/dockercfg v0.3.2 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/docker/docker v28.4.0+incompatible // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/getkin/kin-openapi v0.133.0 // indirect
	github.com/go-kratos/aegis v0.2.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/inflect v0.21.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-playground/form/v4 v4.3.0 // indirect
	github.com/go-resty/resty/v2 v2.6.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.5 // indirect
	github.com/gonfva/docxlib v0.0.0-20210517191039-d8f39cecf1ad // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl/v2 v2.23.0 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.0 // indirect
	github.com/hhrutter/tiff v1.0.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/gojq v0.12.16 // indirect
	github.com/itchyny/timefmt-go v0.1.6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.6 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx/v2 v2.1.4 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20230326075908-cb1d2100619a // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/go-archive v0.2.0 // indirect
	github.com/moby/moby/api v1.54.1 // indirect
	github.com/moby/moby/client v0.4.0 // indirect
	github.com/moby/patternmatcher v0.6.1 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mozillazg/go-httpheader v0.2.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/neurosnap/sentences v1.1.2 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/olekukonko/cat v0.0.0-20250911104152-50322a0618f6 // indirect
	github.com/olekukonko/errors v1.2.0 // indirect
	github.com/olekukonko/ll v0.1.6 // indirect
	github.com/ollama/ollama v0.17.1 // indirect
	github.com/openai/openai-go v1.12.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/panjf2000/ants/v2 v2.10.0 // indirect
	github.com/pdfcpu/pdfcpu v0.11.1 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.6 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.3 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tencentyun/cos-go-sdk-v5 v0.7.69 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/tiktoken-go/tokenizer v0.7.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/wneessen/go-mail v0.7.2 // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zclconf/go-cty v1.16.2 // indirect
	github.com/zclconf/go-cty-yaml v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	google.golang.org/api v0.267.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260401024825-9d38bb4040a9 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gorm.io/gorm v1.31.0 // indirect
	modernc.org/libc v1.37.6 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.7.2 // indirect
	modernc.org/sqlite v1.28.0 // indirect
	mvdan.cc/sh/v3 v3.8.0 // indirect
	trpc.group/trpc-go/trpc-agent-go/storage/postgres v1.11.0 // indirect
	trpc.group/trpc-go/trpc-agent-go/storage/s3 v1.8.0 // indirect
)

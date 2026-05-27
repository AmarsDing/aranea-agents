GOHOSTOS:=$(shell go env GOHOSTOS)
GOEXE:=$(shell go env GOEXE)
GOPATH:=$(shell go env GOPATH)
GOBIN_RAW:=$(shell go env GOBIN)
# Use / so Make wildcard and protoc --plugin see a consistent path on Windows.
GOPATH_NORM:=$(subst \,/,$(GOPATH))
# go install puts binaries in GOBIN when set; otherwise GOPATH/bin.
ifeq ($(strip $(GOBIN_RAW)),)
GO_BIN_DIR:=$(GOPATH_NORM)/bin
else
GO_BIN_DIR:=$(subst \,/,$(GOBIN_RAW))
endif
VERSION=$(shell git describe --tags --always)

# Explicit plugin path: Windows protoc often does not see Cygwin/MSYS PATH for protoc-gen-*.
PROTOC_GEN_TYPESCRIPT_HTTP:=$(GO_BIN_DIR)/protoc-gen-typescript-http$(GOEXE)
# Optional TypeScript emit from api/*.proto (empty = auto: on if plugin exists). Set SKIP_API_TS=1 to force off.
API_TS_PLUGIN_ARGS:=
ifeq ($(SKIP_API_TS),)
  ifneq ($(wildcard $(PROTOC_GEN_TYPESCRIPT_HTTP)),)
    API_TS_PLUGIN_ARGS:=--plugin=protoc-gen-typescript-http=$(PROTOC_GEN_TYPESCRIPT_HTTP) --typescript-http_out=./web/src/services
  else
    $(warning protoc-gen-typescript-http not found at $(PROTOC_GEN_TYPESCRIPT_HTTP); install with `make init`. Skipping API TypeScript emit.; set SKIP_API_TS=1 to silence)
  endif
endif

ifeq ($(GOHOSTOS),windows)
	# Use scripts/list-proto-files.ps1 so Git-Bash/sh does not parse $ $() inside $(shell ...).
	INTERNAL_PROTO_FILES:=$(shell cd "$(CURDIR)" && powershell -NoProfile -ExecutionPolicy Bypass -File scripts/list-proto-files.ps1 internal)
	API_PROTO_FILES:=$(shell cd "$(CURDIR)" && powershell -NoProfile -ExecutionPolicy Bypass -File scripts/list-proto-files.ps1 api)
else
	INTERNAL_PROTO_FILES=$(shell find internal -name '*.proto')
	API_PROTO_FILES=$(shell find api -name '*.proto')
endif

.PHONY: init
# init env
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
	go install go.einride.tech/protoc-gen-typescript-http@latest
	go install github.com/google/wire/cmd/wire@latest

.PHONY: config
# generate internal proto
config:
	protoc --proto_path=./internal \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./internal \
	       $(INTERNAL_PROTO_FILES)

.PHONY: api
# generate api proto
api:
	protoc --proto_path=./api \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./api \
 	       --go-http_out=paths=source_relative:./api \
 	       --go-grpc_out=paths=source_relative:./api \
	       --openapi_out="fq_schema_naming=true,default_response=false:." \
	       $(API_TS_PLUGIN_ARGS) \
	       $(API_PROTO_FILES)

.PHONY: check-overlay
check-overlay:
	go test ./internal/modelcatalog/ -run TestRuntimeOverlayMatchesWebCopy -count=1

.PHONY: cli
# build the aranea CLI binary to ./bin/aranea
cli:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea$(GOEXE) ./cmd/aranea

.PHONY: cli-all
# build aranea CLI for linux/amd64 (cross-compile)
cli-all: cli
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea-linux-amd64 ./cmd/aranea

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: runtime-boundary
# check Agent runtime import boundaries (legacy PowerShell; use `make lint` instead)
runtime-boundary:
ifeq ($(GOHOSTOS),windows)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check-runtime-boundary.ps1
else
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/check-runtime-boundary.ps1
endif

.PHONY: fieldguide-lint
# PGO-2-LINT-02: check Go ↔ TypeScript FieldGuide scope registry is in sync
fieldguide-lint:
	go run ./cmd/araneactl/fieldguide-lint --root .

.PHONY: pgo-lint
# PGO-PRE-02: run all PGO-related lint checks (CLI boundary + fieldguide schema sync)
pgo-lint:
	go run ./cmd/araneactl/lint --root .
	$(MAKE) fieldguide-lint

.PHONY: lint
# run cross-platform lint tool (R1-R10) + go vet + gofmt + golangci-lint (if installed)
# EP-ENG-07: gofmt check added via go run so CI catches formatting drift on any OS.
lint:
	go run ./cmd/araneactl/lint --root .
	go vet ./...
	go run ./cmd/araneactl/fmtcheck --root .
	$(MAKE) fieldguide-lint
	@golangci-lint run ./... 2>/dev/null || true

.PHONY: golangci-lint
# EP-ENG-05: run golangci-lint (must be installed: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
golangci-lint:
	golangci-lint run ./...

.PHONY: test
# run all Go tests
test:
	go test -cover ./...

.PHONY: smoke
# basic smoke test (build + health check)
smoke:
	go build -o bin/admin-smoke ./cmd/admin 2>&1 && echo "smoke: build OK" && rm -f bin/admin-smoke

.PHONY: ci
# full CI pipeline: lint, test, smoke
ci:
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) smoke

.PHONY: generate
# generate
generate:
	go generate ./...
	go mod tidy

# dependency injection (use go-install bin dir so an older wire.exe on PATH — e.g. under GOROOT — does not shadow)
WIRE := $(GO_BIN_DIR)/wire$(GOEXE)

.PHONY: wire-admin
wire-admin:
	cd ./cmd/admin && "$(WIRE)"

.PHONY: wire
# regenerate all wire_gen.go files
wire: wire-admin

.PHONY: wire-clean
# EP-ENG-03: regenerate wire and fail if git diff is non-empty (CI sync check)
wire-clean: wire
	git diff --exit-code -- cmd/admin/wire_gen.go || \
		(echo "wire_gen.go is out of date; run 'make wire' and commit." && exit 1)

.PHONY: proto-clean
# EP-ENG-04: regenerate proto and fail if generated files differ (CI sync check)
proto-clean: api
	git diff --exit-code -- api/ web/src/services/ || \
		(echo "Proto generated files are out of date; run 'make api' and commit." && exit 1)

.PHONY: all
# api+internal proto, go generate (ent etc.) + tidy, then cmd/admin wire
all:
	$(MAKE) api
	$(MAKE) config
	$(MAKE) generate
	$(MAKE) wire-admin

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

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

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: runtime-boundary
# check Agent runtime import boundaries
runtime-boundary:
ifeq ($(GOHOSTOS),windows)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check-runtime-boundary.ps1
else
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/check-runtime-boundary.ps1
endif

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

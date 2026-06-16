# Session Summary Benchmark for trpc-agent-go

This benchmark suite evaluates session-summary behavior in two complementary settings:

- `MT-Bench-101`: baseline vs summary on multi-turn dialogue quality
- `QMSum`: `long_context` vs `summary` vs `summary_ondemand` on long-meeting detail recovery

## Repository Layout

```text
summary/
├── README.md
├── data/
│   ├── download_datasets.sh
│   └── README.md
├── results/
└── trpc-agent-go-impl/
    ├── main.go
    ├── config.go
    ├── benchmark.go
    ├── mtbench.go
    ├── qmsum.go
    └── evaluation/
        ├── dataset/
        └── evaluator/
```

## Setup

This benchmark module is wired to the local `my-trpc-agent-go` checkout through a relative `replace` in [go.mod](trpc-agent-go-impl/go.mod). The expected workspace layout is:

```text
/workspace/github/
├── my-trpc-agent-go
└── my-trpc-agent-go-benchmark
```

## Quick Start

### 1. Download datasets

```bash
cd summary/data
./download_datasets.sh
```

### 2. Run MT-Bench-101

```bash
cd ../trpc-agent-go-impl
go run . \
  -dataset ../data/mt-bench-101 \
  -dataset-format mtbench101 \
  -task CM \
  -num-cases 10 \
  -llm-eval
```

### 3. Run QMSum

```bash
cd ../trpc-agent-go-impl
PGVECTOR_DSN='postgres://USER:PASSWORD@HOST:5432/DB?sslmode=disable' \
go run . \
  -dataset ../data/QMSum \
  -dataset-format qmsum \
  -qmsum-split test \
  -qmsum-domain Committee \
  -qmsum-query-type specific \
  -num-cases 5 \
  -events 40 \
  -qmsum-visible-events 20 \
  -qmsum-min-distance-from-end 80 \
  -qmsum-max-tool-iterations 6
```

## CLI Overview

### Common Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-model` | `gpt-4o-mini` | Model name (`MODEL_NAME` overrides default) |
| `-dataset` | `../data/mt-bench-101` | Dataset path |
| `-dataset-format` | auto | `mtbench101`, `qmsum`, or `longmemeval` |
| `-num-cases` | `0` | Number of cases to run (`0` = all) |
| `-output` | `../results` | Output directory |
| `-events` | `2` | Summary trigger event threshold |
| `-detailed-prompt` | `true` | Enable the nine-section detailed continuity prompt and verbatim user-message appendix for all summary modes |
| `-llm-eval` | `false` | Enable LLM-based evaluation where supported |
| `-resume` | `false` | Resume from checkpoint |
| `-verbose` | `false` | Print detailed conversation logs |

### MT-Bench-101 Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-task` | `""` | Task filter like `CM,GR` |
| `-num-runs` | `1` | Runs per mode for Pass^k |
| `-consistency-threshold` | `0.7` | Pass/fail threshold for consistency |
| `-retention-threshold` | `0.7` | Pass/fail threshold for retention |
| `-k-values` | `1,2,4` | Pass^k values |

### QMSum Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-qmsum-split` | `test` | Dataset split |
| `-qmsum-domain` | `ALL` | `ALL`, `Academic`, `Committee`, or `Product` |
| `-qmsum-query-type` | `specific` | `specific`, `general`, or `all` |
| `-pgvector-dsn` | env | PostgreSQL DSN for `summary` / `summary_ondemand` |
| `-embed-model` | env / `text-embedding-3-small` | Embedding model name |
| `-qmsum-max-tokens` | `384` | Max answer tokens per query |
| `-qmsum-max-tool-iterations` | `6` | Max tool loops for `summary_ondemand` |
| `-qmsum-summary-wait` | `45s` | Max wait time for async summary generation |
| `-qmsum-visible-events` | `20` | Number of most recent transcript turns kept directly visible in `summary` / `summary_ondemand` |
| `-qmsum-min-distance-from-end` | `0` | Minimum support distance from the transcript end; useful for a harder hidden-detail subset |

### LongMemEval Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-lme-question-types` | `""` | LongMemEval question types to include (empty = all) |
| `-lme-visible-events` | `20` | Number of most recent turns kept directly visible in LongMemEval summary modes |

## What Each Dataset Measures

### MT-Bench-101

Best for overall multi-turn quality regression:

- token savings
- prompt savings
- response consistency
- information retention

### QMSum

Best for testing whether on-demand session retrieval can recover details that summary no longer keeps in the prompt:

- answer quality under `long_context`
- degradation under `summary`
- recovery under `summary_ondemand`

## Results

Results are written to the chosen output directory as:

- `results.json`
- `checkpoint.json`
- per-case `*.log`

See [results/REPORT.md](results/REPORT.md) and [results/REPORT.zh_CN.md](results/REPORT.zh_CN.md) for previous reports and analysis context.

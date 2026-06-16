# How to Contribute

Thank you for your interest in `trpc-agent-go-benchmark`.

This repository contains benchmark suites, evaluation assets, benchmark-specific runners, reports, and supporting scripts for `trpc-agent-go`. Contributions are welcome, including bug fixes, benchmark improvements, dataset handling updates, documentation fixes, and new benchmark suites.

## Before contributing code

For significant changes, please discuss the plan before implementation when possible. The best place to start is the issue tracker for this repository:

- Existing issues: https://github.com/trpc-group/trpc-agent-go-benchmark/issues
- New issue: https://github.com/trpc-group/trpc-agent-go-benchmark/issues/new

If the proposed change depends on framework APIs or behavior in the main `trpc-agent-go` repository, it is usually better to align the benchmark-side change with the corresponding framework-side discussion or pull request.

## Repository layout

This repository is not a single Go module. Different benchmark suites have their own structure and validation entry points.

Examples:

- `anthropic_skills/trpc-agent-go-impl`: Go benchmark runner
- `gaia/trpc-agent-go-impl`: Go benchmark runner
- `knowledge/knowledge_system/trpc_agent_go/trpc_knowledge`: Go benchmark runner
- `memory/trpc-agent-go-impl`: Go benchmark runner
- `summary/trpc-agent-go-impl`: Go benchmark runner
- `toolsearch/trpc-agent-go-impl`: Go benchmark runner

Other directories may contain Python scripts, benchmark data, reports, or documentation rather than Go packages.

## Contributing code

Follow the [GitHub flow](https://docs.github.com/en/get-started/quickstart/github-flow) and open a pull request against this repository.

Some things to keep in mind:

- Keep changes scoped to the benchmark you are modifying.
- Avoid mixing benchmark logic changes, dataset updates, and unrelated documentation cleanup in the same pull request.
- If you change benchmark methodology or reported outputs, explain the reason clearly in the pull request description.

If this is your first time submitting a pull request to `trpc-agent-go-benchmark`, you will be asked to sign the [Contributor License Agreement](https://github.com/trpc-group/cla-database/blob/main/Tencent-Contributor-License-Agreement.md) in the pull request conversation before the pull request can be accepted.

## Validation

Before submitting a pull request, run the validation that matches the files you changed.

For Go benchmark runners, run commands in the affected module directory:

```bash
go mod tidy
go test ./...
```

Common examples:

```bash
cd anthropic_skills/trpc-agent-go-impl && go mod tidy && go test ./...
cd gaia/trpc-agent-go-impl && go mod tidy && go test ./...
cd knowledge/knowledge_system/trpc_agent_go/trpc_knowledge && go mod tidy && go test ./...
cd memory/trpc-agent-go-impl && go mod tidy && go test ./...
cd summary/trpc-agent-go-impl && go mod tidy && go test ./...
cd toolsearch/trpc-agent-go-impl && go mod tidy && go test ./...
```

For Python or data-processing changes, run the relevant script or checks for the touched benchmark and confirm that command examples, paths, and generated outputs are still valid.

For documentation-only changes, verify links, commands, paths, and filenames carefully.

## Writing good commit messages

Use concise English commit messages that describe the actual change. Prefer a short subject line prefixed by the primary affected area.

Examples:

- `memory: align module versions with published releases`
- `gaia: update benchmark migration links`
- `toolsearch: fix model module dependency versions`

If your change is associated with an issue, reference it in the pull request or commit message when appropriate, for example:

- `Fixes #123`
- `Updates #123`

## Benchmark assets and reports

Please be careful when changing benchmark datasets, generated results, or reports.

- Do not overwrite historical benchmark outputs unless the change is intentional.
- Keep generated artifacts out of the repository unless the repository already tracks them.
- When updating reported numbers, ensure the surrounding methodology and command examples still match the new results.

## Copyright headers

If you add new Go files to this repository, use the same standard copyright header as the main project:

```go
//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
```

Do not update the copyright year on existing files unless there is a specific reason to do so.

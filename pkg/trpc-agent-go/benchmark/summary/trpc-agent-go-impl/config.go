//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type datasetFormat string

const (
	datasetFormatMTBench101  datasetFormat = "mtbench101"
	datasetFormatQMSum       datasetFormat = "qmsum"
	datasetFormatLongMemEval datasetFormat = "longmemeval"
)

type appConfig struct {
	ModelName      string
	DatasetPath    string
	DatasetFormat  datasetFormat
	NumCases       int
	OutputDir      string
	Events         int
	UseLLMEval     bool
	Verbose        bool
	Resume         bool
	DetailedPrompt bool
	MTBench        mtBenchConfig
	QMSum          qmsumConfig
	LongMemEval    longMemEvalConfig
}

type mtBenchConfig struct {
	TaskFilterRaw        string
	TaskFilters          []string
	NumRuns              int
	ConsistencyThreshold float64
	RetentionThreshold   float64
	KValues              []int
}

type qmsumConfig struct {
	Split              string
	Domain             string
	QueryType          string
	PGVectorDSN        string
	EmbedModel         string
	MaxTokens          int
	MaxToolIterations  int
	SummaryWait        time.Duration
	VisibleEvents      int
	MinDistanceFromEnd int
}

type longMemEvalConfig struct {
	QuestionTypes     string
	PGVectorDSN       string
	EmbedModel        string
	MaxTokens         int
	MaxToolIterations int
	SummaryWait       time.Duration
	VisibleEvents     int
}

var (
	flagModel          = flag.String("model", "", "Model name (env MODEL_NAME or gpt-4o-mini)")
	flagDataset        = flag.String("dataset", "../data/mt-bench-101", "Dataset path")
	flagOutput         = flag.String("output", "../results", "Output directory")
	flagEvents         = flag.Int("events", 2, "Event threshold for summarization")
	flagDetailedPrompt = flag.Bool(
		"detailed-prompt",
		true,
		"Enable nine-section detailed continuity summary prompt + verbatim user-message preservation for summary modes",
	)

	flagNumCases   = flag.Int("num-cases", 0, "Number of test cases (0=all)")
	flagVerbose    = flag.Bool("verbose", false, "Print full conversation content")
	flagResume     = flag.Bool("resume", false, "Resume from previous checkpoint")
	flagUseLLMEval = flag.Bool(
		"llm-eval",
		false,
		"Use LLM for semantic evaluation",
	)

	flagDatasetFormat = flag.String(
		"dataset-format",
		"",
		"Dataset format: mtbench101 or qmsum (default: auto-detect from -dataset)",
	)

	flagTask = flag.String(
		"task",
		"",
		"Filter MT-Bench-101 entries by task code (e.g., CM, GR)",
	)
	flagNumRuns              = flag.Int("num-runs", 1, "Runs per case for Pass^k consistency")
	flagConsistencyThreshold = flag.Float64(
		"consistency-threshold",
		0.7,
		"Threshold for consistency pass/fail (0.0-1.0)",
	)
	flagRetentionThreshold = flag.Float64(
		"retention-threshold",
		0.7,
		"Threshold for retention pass/fail (0.0-1.0)",
	)
	flagKValues = flag.String(
		"k-values",
		"1,2,4",
		"Pass^k k values (comma-separated)",
	)

	flagQMSumSplit = flag.String(
		"qmsum-split",
		"test",
		"QMSum split: train, val, or test",
	)
	flagQMSumDomain = flag.String(
		"qmsum-domain",
		"ALL",
		"QMSum domain: ALL, Academic, Committee, or Product",
	)
	flagQMSumQueryType = flag.String(
		"qmsum-query-type",
		"specific",
		"QMSum query type: specific, general, or all",
	)
	flagPGVectorDSN = flag.String(
		"pgvector-dsn",
		"",
		"PostgreSQL DSN for QMSum summary/on-demand modes (env PGVECTOR_DSN)",
	)
	flagEmbedModel = flag.String(
		"embed-model",
		"",
		"Embedding model for QMSum session pgvector indexing (env EMBED_MODEL_NAME or text-embedding-3-small)",
	)
	flagQMSumMaxTokens = flag.Int(
		"qmsum-max-tokens",
		384,
		"Maximum answer tokens for one QMSum query",
	)
	flagQMSumMaxToolIterations = flag.Int(
		"qmsum-max-tool-iterations",
		6,
		"Maximum tool iterations for summary_ondemand mode",
	)
	flagQMSumSummaryWait = flag.Duration(
		"qmsum-summary-wait",
		45*time.Second,
		"Maximum time to wait for session summary generation before querying",
	)
	flagQMSumVisibleEvents = flag.Int(
		"qmsum-visible-events",
		20,
		"Number of most recent transcript turns kept directly visible in QMSum summary modes",
	)
	flagQMSumMinDistanceFromEnd = flag.Int(
		"qmsum-min-distance-from-end",
		0,
		"Minimum support distance from transcript end for QMSum case selection (0 keeps all cases)",
	)

	flagLMEQuestionTypes = flag.String(
		"lme-question-types",
		"",
		"LongMemEval question types to include (comma-separated; empty=all). Values: single-session-user, single-session-assistant, single-session-preference, multi-session, temporal-reasoning, knowledge-update",
	)
	flagLMEVisibleEvents = flag.Int(
		"lme-visible-events",
		20,
		"Number of most recent turns kept visible in LongMemEval summary modes",
	)
)

func loadAppConfig() (*appConfig, error) {
	taskFilters, err := parseTaskFilter(*flagTask)
	if err != nil {
		return nil, err
	}
	kValues, err := parseKValues(*flagKValues)
	if err != nil {
		return nil, err
	}

	cfg := &appConfig{
		ModelName:      resolveModelName(),
		DatasetPath:    strings.TrimSpace(*flagDataset),
		DatasetFormat:  detectDatasetFormat(*flagDatasetFormat, *flagDataset),
		NumCases:       *flagNumCases,
		OutputDir:      *flagOutput,
		Events:         *flagEvents,
		UseLLMEval:     *flagUseLLMEval,
		Verbose:        *flagVerbose,
		Resume:         *flagResume,
		DetailedPrompt: *flagDetailedPrompt,
		MTBench: mtBenchConfig{
			TaskFilterRaw:        strings.TrimSpace(*flagTask),
			TaskFilters:          taskFilters,
			NumRuns:              *flagNumRuns,
			ConsistencyThreshold: *flagConsistencyThreshold,
			RetentionThreshold:   *flagRetentionThreshold,
			KValues:              kValues,
		},
		QMSum: qmsumConfig{
			Split:              *flagQMSumSplit,
			Domain:             *flagQMSumDomain,
			QueryType:          *flagQMSumQueryType,
			PGVectorDSN:        resolvePGVectorDSN(),
			EmbedModel:         resolveEmbedModelName(),
			MaxTokens:          *flagQMSumMaxTokens,
			MaxToolIterations:  *flagQMSumMaxToolIterations,
			SummaryWait:        *flagQMSumSummaryWait,
			VisibleEvents:      *flagQMSumVisibleEvents,
			MinDistanceFromEnd: *flagQMSumMinDistanceFromEnd,
		},
		LongMemEval: longMemEvalConfig{
			QuestionTypes:     strings.TrimSpace(*flagLMEQuestionTypes),
			PGVectorDSN:       resolvePGVectorDSN(),
			EmbedModel:        resolveEmbedModelName(),
			MaxTokens:         *flagQMSumMaxTokens,
			MaxToolIterations: *flagQMSumMaxToolIterations,
			SummaryWait:       *flagQMSumSummaryWait,
			VisibleEvents:     *flagLMEVisibleEvents,
		},
	}

	return cfg, validateAppConfig(cfg)
}

func validateAppConfig(cfg *appConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.ModelName == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(cfg.DatasetPath) == "" {
		return fmt.Errorf("dataset path is required, please set -dataset")
	}
	if cfg.NumCases < 0 {
		return fmt.Errorf("invalid -num-cases: %d", cfg.NumCases)
	}
	if cfg.Events < 0 {
		return fmt.Errorf("invalid -events: %d", cfg.Events)
	}
	if cfg.MTBench.NumRuns < 1 {
		return fmt.Errorf("invalid -num-runs: %d", cfg.MTBench.NumRuns)
	}
	if cfg.MTBench.ConsistencyThreshold < 0 || cfg.MTBench.ConsistencyThreshold > 1 {
		return fmt.Errorf(
			"invalid -consistency-threshold: %.3f",
			cfg.MTBench.ConsistencyThreshold,
		)
	}
	if cfg.MTBench.RetentionThreshold < 0 || cfg.MTBench.RetentionThreshold > 1 {
		return fmt.Errorf(
			"invalid -retention-threshold: %.3f",
			cfg.MTBench.RetentionThreshold,
		)
	}
	if cfg.QMSum.VisibleEvents <= 0 {
		return fmt.Errorf("invalid -qmsum-visible-events: %d", cfg.QMSum.VisibleEvents)
	}
	if cfg.QMSum.MinDistanceFromEnd < 0 {
		return fmt.Errorf(
			"invalid -qmsum-min-distance-from-end: %d",
			cfg.QMSum.MinDistanceFromEnd,
		)
	}
	return nil
}

func parseTaskFilter(filter string) ([]string, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}

	parts := strings.Split(filter, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := mtBench101TaskCodeSet[p]; !ok {
			return nil, fmt.Errorf(
				"invalid task code: %s, valid values: %s",
				p,
				strings.Join(validMTBench101TaskCodes(), ","),
			)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf(
			"invalid task filter: %s, valid values: %s",
			filter,
			strings.Join(validMTBench101TaskCodes(), ","),
		)
	}
	return out, nil
}

func parseKValues(input string) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []int{1}, nil
	}

	parts := strings.Split(input, ",")
	out := make([]int, 0, len(parts))
	seen := make(map[int]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid -k-values: %s", input)
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid -k-values: %s", input)
	}
	return out, nil
}

func detectDatasetFormat(explicit, datasetPath string) datasetFormat {
	switch strings.ToLower(strings.TrimSpace(explicit)) {
	case string(datasetFormatQMSum):
		return datasetFormatQMSum
	case string(datasetFormatMTBench101):
		return datasetFormatMTBench101
	case string(datasetFormatLongMemEval):
		return datasetFormatLongMemEval
	}

	lowerPath := strings.ToLower(strings.TrimSpace(datasetPath))
	if strings.Contains(lowerPath, "longmemeval") {
		return datasetFormatLongMemEval
	}
	if strings.Contains(lowerPath, "qmsum") {
		return datasetFormatQMSum
	}
	if strings.Contains(lowerPath, "mtbench") || strings.Contains(lowerPath, "mt-bench") {
		return datasetFormatMTBench101
	}

	cleaned := filepath.Clean(datasetPath)
	if info, err := os.Stat(cleaned); err == nil {
		if info.Mode().IsRegular() && strings.HasSuffix(lowerPath, "mtbench101.jsonl") {
			return datasetFormatMTBench101
		}
		if info.IsDir() {
			if _, err := os.Stat(filepath.Join(cleaned, "subjective", "mtbench101.jsonl")); err == nil {
				return datasetFormatMTBench101
			}
			if _, err := os.Stat(filepath.Join(cleaned, "data", "Committee")); err == nil {
				return datasetFormatQMSum
			}
			if _, err := os.Stat(filepath.Join(cleaned, "data", "ALL")); err == nil {
				return datasetFormatQMSum
			}
		}
	}

	return datasetFormatMTBench101
}

func resolveModelName() string {
	if *flagModel != "" {
		return *flagModel
	}
	if env := os.Getenv("MODEL_NAME"); env != "" {
		return env
	}
	return "gpt-4o-mini"
}

func resolvePGVectorDSN() string {
	if *flagPGVectorDSN != "" {
		return *flagPGVectorDSN
	}
	return os.Getenv("PGVECTOR_DSN")
}

func resolveEmbedModelName() string {
	if *flagEmbedModel != "" {
		return *flagEmbedModel
	}
	if env := os.Getenv("EMBED_MODEL_NAME"); env != "" {
		return env
	}
	return "text-embedding-3-small"
}

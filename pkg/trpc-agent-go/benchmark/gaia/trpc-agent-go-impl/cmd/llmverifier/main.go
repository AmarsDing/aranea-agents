//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"flag"
	"log"

	gaiaeval "trpc.group/trpc-go/trpc-agent-go-benchmark/gaia/trpc-agent-go-impl"
)

func main() {
	cfg := gaiaeval.DefaultConfig()
	cfg.ModeName = "react + llm-verifier best-of-N"
	cfg.Framework = "trpc-agent-go-llm-verifier"
	cfg.OutputPath = "../results/trpc-agent-go_llmverifier.json"
	verifierCfg := gaiaeval.DefaultLLMVerifierConfig()

	flag.StringVar(&cfg.DatasetPath, "dataset", cfg.DatasetPath, "Path to GAIA dataset")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory containing data files")
	flag.StringVar(&cfg.OutputPath, "output", cfg.OutputPath, "Path to output results")
	flag.IntVar(&cfg.MaxTasks, "tasks", cfg.MaxTasks, "Maximum number of tasks to run")
	flag.StringVar(&cfg.ModelName, "model", cfg.ModelName, "Candidate model name")
	flag.StringVar(&cfg.TaskID, "task-id", cfg.TaskID, "Run a specific task by task ID or 1-based index")
	flag.IntVar(&verifierCfg.Attempts, "attempts", verifierCfg.Attempts, "Number of candidate attempts")
	flag.StringVar(&verifierCfg.JudgeModelName, "judge-model", verifierCfg.JudgeModelName, "Judge model name")
	flag.IntVar(&verifierCfg.JudgeSamples, "judge-samples", verifierCfg.JudgeSamples, "Number of judge samples per candidate comparison")
	flag.IntVar(&verifierCfg.JudgeMaxTokens, "judge-max-tokens", verifierCfg.JudgeMaxTokens, "Judge max output tokens")
	flag.Parse()

	cfg.RunnerFactory = gaiaeval.LLMVerifierRunnerFactory(cfg, verifierCfg)
	if err := gaiaeval.Run(context.Background(), cfg); err != nil {
		log.Fatalf("run LLM verifier benchmark: %v", err)
	}
}

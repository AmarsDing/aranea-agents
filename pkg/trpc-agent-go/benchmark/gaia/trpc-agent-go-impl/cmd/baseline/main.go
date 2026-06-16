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
	cfg.ModeName = "react baseline"
	cfg.RunnerFactory = gaiaeval.BaselineRunnerFactory()

	flag.StringVar(&cfg.DatasetPath, "dataset", cfg.DatasetPath, "Path to GAIA dataset")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory containing data files")
	flag.StringVar(&cfg.OutputPath, "output", cfg.OutputPath, "Path to output results")
	flag.IntVar(&cfg.MaxTasks, "tasks", cfg.MaxTasks, "Maximum number of tasks to run")
	flag.StringVar(&cfg.ModelName, "model", cfg.ModelName, "Model name to use")
	flag.StringVar(&cfg.TaskID, "task-id", cfg.TaskID, "Run a specific task by task ID or 1-based index")
	flag.Parse()

	if err := gaiaeval.Run(context.Background(), cfg); err != nil {
		log.Fatalf("run baseline benchmark: %v", err)
	}
}

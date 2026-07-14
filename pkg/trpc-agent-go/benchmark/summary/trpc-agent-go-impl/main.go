//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package main provides benchmark entrypoints for evaluating session summary
// behavior on multi-turn dialogue and long-meeting datasets.
package main

import (
	"context"
	"flag"
	"log"
	"os"
)

func main() {
	flag.Parse()

	cfg, err := loadAppConfig()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	if err := newBenchmark(cfg).Run(context.Background()); err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}
}

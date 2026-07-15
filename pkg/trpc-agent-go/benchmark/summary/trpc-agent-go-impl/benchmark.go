//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import "context"

type benchmark interface {
	Run(context.Context) error
}

func newBenchmark(cfg *appConfig) benchmark {
	switch cfg.DatasetFormat {
	case datasetFormatQMSum:
		return newQMSumBenchmark(cfg)
	case datasetFormatLongMemEval:
		return newLongMemEvalBenchmark(cfg)
	case datasetFormatMTBench101:
		fallthrough
	default:
		return newMTBenchBenchmark(cfg)
	}
}

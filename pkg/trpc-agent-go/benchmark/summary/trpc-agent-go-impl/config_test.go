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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectDatasetFormat(t *testing.T) {
	t.Parallel()

	mtDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mtDir, "subjective"), 0755); err != nil {
		t.Fatalf("mkdir mtbench dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mtDir, "subjective", "mtbench101.jsonl"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write mtbench file: %v", err)
	}

	qmDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(qmDir, "data", "Committee", "test"), 0755); err != nil {
		t.Fatalf("mkdir qmsum dir: %v", err)
	}

	cases := []struct {
		name     string
		explicit string
		path     string
		want     datasetFormat
	}{
		{name: "explicit_qmsum", explicit: "qmsum", path: mtDir, want: datasetFormatQMSum},
		{name: "explicit_mtbench", explicit: "mtbench101", path: qmDir, want: datasetFormatMTBench101},
		{name: "detect_mtbench_by_layout", path: mtDir, want: datasetFormatMTBench101},
		{name: "detect_qmsum_by_layout", path: qmDir, want: datasetFormatQMSum},
		{name: "detect_qmsum_by_name", path: "/tmp/QMSum", want: datasetFormatQMSum},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := detectDatasetFormat(tc.explicit, tc.path); got != tc.want {
				t.Fatalf("detectDatasetFormat(%q, %q) = %q, want %q", tc.explicit, tc.path, got, tc.want)
			}
		})
	}
}

func TestParseKValues(t *testing.T) {
	t.Parallel()

	got, err := parseKValues("1, 2,2,4")
	if err != nil {
		t.Fatalf("parseKValues returned error: %v", err)
	}
	want := []int{1, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKValues = %v, want %v", got, want)
	}

	if _, err := parseKValues("0,2"); err == nil {
		t.Fatalf("parseKValues should reject zero")
	}
}

func TestValidateAppConfigRejectsInvalidQMSumFlags(t *testing.T) {
	t.Parallel()

	cfg := &appConfig{
		ModelName:     "gpt-4o-mini",
		DatasetPath:   "/tmp/QMSum",
		DatasetFormat: datasetFormatQMSum,
		QMSum: qmsumConfig{
			VisibleEvents:      0,
			MinDistanceFromEnd: -1,
		},
		MTBench: mtBenchConfig{
			NumRuns:              1,
			ConsistencyThreshold: 0.7,
			RetentionThreshold:   0.7,
		},
	}

	if err := validateAppConfig(cfg); err == nil {
		t.Fatalf("validateAppConfig should reject invalid QMSum flags")
	}
}

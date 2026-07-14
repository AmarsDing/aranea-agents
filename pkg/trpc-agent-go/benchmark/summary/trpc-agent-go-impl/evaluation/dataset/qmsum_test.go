//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package dataset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadQMSum(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "data", "Committee", "test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := `{
  "topic_list": [{"topic":"budget","relevant_text_span":[["0","1"]]}],
  "general_query_list": [
    {"query":"Summarize the meeting.","answer":"They discussed budget and hiring."}
  ],
  "specific_query_list": [
    {"query":"What did they decide about hiring?","answer":"They decided to hire two people.","relevant_text_span":[["1","2"]]}
  ],
  "meeting_transcripts": [
    {"speaker":"Alice","content":"Let's discuss budget."},
    {"speaker":"Bob","content":"We should hire two people."}
  ]
}`
	path := filepath.Join(dir, "committee_1.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loader := NewDatasetLoader(root)
	cases, err := loader.LoadQMSum("test", "Committee", "all")
	if err != nil {
		t.Fatalf("LoadQMSum returned error: %v", err)
	}

	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}
	if cases[0].CaseID != "committee_1_general_01" {
		t.Fatalf("unexpected general case ID: %s", cases[0].CaseID)
	}
	if cases[1].CaseID != "committee_1_specific_01" {
		t.Fatalf("unexpected specific case ID: %s", cases[1].CaseID)
	}
	if len(cases[0].Transcript) != 2 || cases[0].Transcript[0].Speaker != "Alice" {
		t.Fatalf("unexpected transcript payload: %+v", cases[0].Transcript)
	}
}

func TestNormalizeQMSumInputs(t *testing.T) {
	t.Parallel()

	if got := normalizeQMSumSplit("validation"); got != "val" {
		t.Fatalf("normalizeQMSumSplit(validation) = %q, want val", got)
	}

	if got, err := normalizeQMSumDomain("committee"); err != nil || got != "Committee" {
		t.Fatalf("normalizeQMSumDomain(committee) = %q, %v", got, err)
	}
	if _, err := normalizeQMSumDomain("unknown"); err == nil {
		t.Fatalf("normalizeQMSumDomain should reject invalid domain")
	}

	if got, err := normalizeQMSumQueryType("GENERAL"); err != nil || got != "general" {
		t.Fatalf("normalizeQMSumQueryType(GENERAL) = %q, %v", got, err)
	}
	if _, err := normalizeQMSumQueryType("detail"); err == nil {
		t.Fatalf("normalizeQMSumQueryType should reject invalid query type")
	}
}

func TestQMSumCaseSupportWindowAndDistance(t *testing.T) {
	t.Parallel()

	qcase := &QMSumCase{
		RelevantTextSpan: [][]string{
			{"10", "12"},
			{"4", "5"},
			{"20", "18"},
		},
		Transcript: make([]QMSumTranscriptTurn, 30),
	}

	start, end, ok := qcase.SupportTurnWindow()
	if !ok {
		t.Fatalf("SupportTurnWindow should succeed")
	}
	if start != 4 || end != 20 {
		t.Fatalf("SupportTurnWindow = (%d, %d), want (4, 20)", start, end)
	}

	distance, ok := qcase.SupportDistanceFromEnd()
	if !ok {
		t.Fatalf("SupportDistanceFromEnd should succeed")
	}
	if distance != 9 {
		t.Fatalf("SupportDistanceFromEnd = %d, want 9", distance)
	}
}

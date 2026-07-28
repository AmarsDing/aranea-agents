package cmd

import (
	"testing"
)

func TestNewEvaluationCmd_Structure(t *testing.T) {
	root := NewEvaluationCmd()
	if root.Use != "eval" {
		t.Errorf("Use: got %q, want %q", root.Use, "eval")
	}
	findCmdPath(t, root, "datasets", "ls")
	dsGet := findCmdPath(t, root, "datasets", "get")
	dsCreate := findCmdPath(t, root, "datasets", "create")
	findCmdPath(t, root, "runs", "ls")
	runGet := findCmdPath(t, root, "runs", "get")
	runCreate := findCmdPath(t, root, "runs", "create")
	results := findCmdPath(t, root, "results")

	requireExactArgs(t, dsGet)
	requireExactArgs(t, runGet)
	requireExactArgs(t, results)
	requireFlag(t, dsCreate, "name", true)
	requireFlag(t, runCreate, "dataset-id", true)
	requireFlag(t, runCreate, "agent-id", true)
}

func TestFormatScore(t *testing.T) {
	if got := formatScore(0.875); got != "0.875" {
		t.Errorf("formatScore(0.875): got %q, want %q", got, "0.875")
	}
	if got := formatScore(0); got != "0.000" {
		t.Errorf("formatScore(0): got %q, want %q", got, "0.000")
	}
}

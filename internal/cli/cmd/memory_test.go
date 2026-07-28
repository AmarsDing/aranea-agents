package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// findCmdPath locates a nested subcommand by path (e.g. "proposals", "approve").
func findCmdPath(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	found, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %v: %v", path, err)
	}
	if found == root {
		t.Fatalf("subcommand %v not found under %q", path, root.Use)
	}
	return found
}

// requireExactArgs asserts the command rejects wrong positional-arg counts.
func requireExactArgs(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Errorf("%s: expected error for 0 args", cmd.Use)
	}
	if err := cmd.Args(cmd, []string{"x1"}); err != nil {
		t.Errorf("%s: unexpected error for 1 arg: %v", cmd.Use, err)
	}
}

// requireFlag asserts a flag exists; required also checks the required annotation.
func requireFlag(t *testing.T, cmd *cobra.Command, name string, required bool) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	if f == nil {
		t.Fatalf("%s: flag --%s not defined", cmd.Use, name)
	}
	if required {
		ann := f.Annotations[cobra.BashCompOneRequiredFlag]
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("%s: flag --%s should be required", cmd.Use, name)
		}
	}
}

func TestNewMemoryCmd_Structure(t *testing.T) {
	root := NewMemoryCmd()
	if root.Use != "memory" {
		t.Errorf("Use: got %q, want %q", root.Use, "memory")
	}
	findCmdPath(t, root, "facts", "ls")
	findCmdPath(t, root, "proposals", "ls")
	approve := findCmdPath(t, root, "proposals", "approve")
	reject := findCmdPath(t, root, "proposals", "reject")
	search := findCmdPath(t, root, "search")
	recall := findCmdPath(t, root, "recall-debug")

	requireExactArgs(t, approve)
	requireExactArgs(t, reject)
	requireFlag(t, search, "agent-id", true)
	requireFlag(t, search, "query", true)
	requireFlag(t, recall, "agent-id", true)
}

func TestMemoryProposalsLs_RequiresAgentID(t *testing.T) {
	ls := findCmdPath(t, NewMemoryCmd(), "proposals", "ls")
	requireFlag(t, ls, "agent-id", true)
}

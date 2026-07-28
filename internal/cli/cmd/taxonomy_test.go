package cmd

import (
	"errors"
	"testing"

	"aranea-agents/internal/cli"
)

func TestNewTaxonomyCmd_Constructs(t *testing.T) {
	c := NewTaxonomyCmd()
	if c.Use != "taxonomy" {
		t.Errorf("Use: got %q, want %q", c.Use, "taxonomy")
	}
	want := []string{"ls", "tree", "get", "create", "update", "delete", "reorder"}
	for _, name := range want {
		if findSubCmd(c, name) == nil {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestTaxonomyCreateCmd_MissingRequiredFlags(t *testing.T) {
	c := taxonomyCreateCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--name", "Golang"})
	if err := c.Execute(); err == nil {
		t.Fatal("expected error for missing --key, got nil")
	}
}

func TestTaxonomyReorderCmd_EmptyIDs(t *testing.T) {
	c := taxonomyReorderCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--ids", ""})
	err := c.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "INVALID_IDS" {
		t.Errorf("code: got %q, want %q", ce.Code, "INVALID_IDS")
	}
}

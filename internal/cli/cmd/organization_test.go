package cmd

import (
	"errors"
	"io"
	"testing"

	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

func TestNewOrganizationCmd_Constructs(t *testing.T) {
	c := NewOrganizationCmd()
	if c.Use != "org" {
		t.Errorf("Use: got %q, want %q", c.Use, "org")
	}
	want := []string{"ls", "tree", "get", "create", "update", "delete", "reorder"}
	for _, name := range want {
		if findSubCmd(c, name) == nil {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestOrgCreateCmd_InvalidLevel(t *testing.T) {
	c := orgCreateCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--key", "acme", "--name", "Acme", "--level", "planet"})
	err := c.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "INVALID_LEVEL" {
		t.Errorf("code: got %q, want %q", ce.Code, "INVALID_LEVEL")
	}
}

func TestOrgCreateCmd_MissingRequiredFlags(t *testing.T) {
	c := orgCreateCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--name", "Acme", "--level", "company"})
	if err := c.Execute(); err == nil {
		t.Fatal("expected error for missing --key, got nil")
	}
}

func TestOrgUpdateCmd_InvalidLevel(t *testing.T) {
	c := orgUpdateCmd()
	silenceCmd(c)
	c.SetArgs([]string{"org-1", "--level", "planet"})
	err := c.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "INVALID_LEVEL" {
		t.Errorf("code: got %q, want %q", ce.Code, "INVALID_LEVEL")
	}
}

func TestOrgReorderCmd_EmptyIDs(t *testing.T) {
	c := orgReorderCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--ids", " , ,"})
	err := c.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "INVALID_IDS" {
		t.Errorf("code: got %q, want %q", ce.Code, "INVALID_IDS")
	}
}

// findSubCmd 按名称查找子命令。
func findSubCmd(c *cobra.Command, name string) *cobra.Command {
	for _, sub := range c.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

// silenceCmd 抑制测试中的 usage/error 输出。
func silenceCmd(c *cobra.Command) {
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
}

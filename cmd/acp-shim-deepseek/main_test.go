package main

import (
	"errors"
	"flag"
	"os"
	"testing"
	"time"
)

// 评审 2026-11-09 D-LOW：--help/-h 须为正常退出（exit 0），非法参数仍 exit 2。
// 原实现用全局 flag（ExitOnError），--help 被逐出为 exit 2。

func TestParseOptionsHelpYieldsErrHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		if _, err := parseOptions(args); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("parseOptions(%v) error = %v, want flag.ErrHelp", args, err)
		}
	}
}

func TestParseOptionsInvalidFlagIsNotErrHelp(t *testing.T) {
	if _, err := parseOptions([]string{"--no-such-flag"}); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseOptions(--no-such-flag) error = %v, want non-help parse error", err)
	}
}

func TestParseOptionsDefaultsAndOverrides(t *testing.T) {
	o, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions(nil) error = %v", err)
	}
	if o.baseURL != "https://api.deepseek.com" || o.model != "deepseek-chat" ||
		o.system != "" || o.heartbeat != 30*time.Second || o.logLevel != "info" {
		t.Fatalf("parseOptions(nil) = %+v, want documented defaults", o)
	}
	o, err = parseOptions([]string{"-model", "deepseek-reasoner", "-heartbeat", "0", "-system", "s"})
	if err != nil {
		t.Fatalf("parseOptions(overrides) error = %v", err)
	}
	if o.model != "deepseek-reasoner" || o.heartbeat != 0 || o.system != "s" {
		t.Fatalf("parseOptions(overrides) = %+v", o)
	}
}

// run() 读 os.Args：还原现场，断退出码语义（不到 API key 检查即返回，无需注入 env）。
func TestRunHelpExitsZero(t *testing.T) {
	old := os.Args
	os.Args = []string{"acp-shim-deepseek", "--help"}
	defer func() { os.Args = old }()
	if code := run(); code != 0 {
		t.Fatalf("run() with --help = %d, want 0", code)
	}
}

func TestRunInvalidFlagExitsTwo(t *testing.T) {
	old := os.Args
	os.Args = []string{"acp-shim-deepseek", "--no-such-flag"}
	defer func() { os.Args = old }()
	if code := run(); code != 2 {
		t.Fatalf("run() with bad flag = %d, want 2", code)
	}
}

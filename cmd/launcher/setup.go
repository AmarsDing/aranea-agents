package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// setupState records the first-run configuration wizard choices.
// Stored at configs/launcher-setup.json; detectRuntime honors it on every start.
type setupState struct {
	Done      bool   `json:"done"`
	PGMode    string `json:"pg_mode"` // bundled | system
	PGHost    string `json:"pg_host,omitempty"`
	PGPort    string `json:"pg_port,omitempty"`
	PGUser    string `json:"pg_user,omitempty"`
	RedisMode string `json:"redis_mode"` // bundled | system
	RedisAddr string `json:"redis_addr,omitempty"`
	Autostart string `json:"autostart"` // none | service | task
}

func setupStatePath(root string) string {
	return filepath.Join(root, "configs", "launcher-setup.json")
}

func loadSetupState(root string) *setupState {
	b, err := os.ReadFile(setupStatePath(root))
	if err != nil {
		return nil
	}
	var st setupState
	if json.Unmarshal(b, &st) != nil || !st.Done {
		return nil
	}
	return &st
}

func saveSetupState(root string, st *setupState) error {
	st.Done = true
	if err := os.MkdirAll(filepath.Dir(setupStatePath(root)), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(setupStatePath(root), b, 0o644)
}

func setupDone(root string) bool { return loadSetupState(root) != nil }

// prompter reads interactive line input from the allocated console.
type prompter struct {
	r   *bufio.Reader
	out func(string)
}

func newPrompter(out func(string)) *prompter {
	return &prompter{r: bufio.NewReader(consoleInput()), out: out}
}

func (p *prompter) ask(prompt, def string) string {
	if def != "" {
		p.out(fmt.Sprintf("%s [%s]: ", prompt, def))
	} else {
		p.out(prompt + ": ")
	}
	line, err := p.r.ReadString('\n')
	if err != nil && err != io.EOF {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func (p *prompter) askYesNo(prompt string, defYes bool) bool {
	def := "y/N"
	if defYes {
		def = "Y/n"
	}
	ans := strings.ToLower(p.ask(prompt+" ("+def+")", ""))
	if ans == "" {
		return defYes
	}
	return ans == "y" || ans == "yes"
}

// runSetupWizard runs the interactive first-run configuration wizard in the
// status console (openclaw-style): detect → choose PG/Redis → verify → save →
// optionally register auto-start. Choices persist to configs/launcher-setup.json.
func runSetupWizard(root string, ui *statusConsole, log func(string, ...any)) *setupState {
	out := ui.Println
	p := newPrompter(out)

	out("")
	out("================ Initial Configuration ================")
	out("Press Enter to accept [defaults]. 直接回车使用 [默认值]。")
	out("")

	st := &setupState{Autostart: "none"}

	// --- PostgreSQL ---
	out("PostgreSQL database / 数据库:")
	out("  [1] Bundled portable (内置便携版, :5433) — recommended, zero setup")
	out("  [2] System / external PostgreSQL (系统或外部实例)")
	if p.ask("Choose PostgreSQL mode 选择模式", "1") == "2" {
		st.PGMode = "system"
		st.PGHost = p.ask("  host", "127.0.0.1")
		st.PGPort = p.ask("  port", "5432")
		st.PGUser = p.ask("  user", "postgres")
		pass := p.ask("  password (empty = trust/无密码)", "")
		if verifyPGChoice(root, st, pass, out) {
			out("  [OK] PostgreSQL connection succeeded / 连接成功")
			if pass != "" {
				// Persist for non-interactive runs; loadPGPassword reads this file.
				_ = os.MkdirAll(filepath.Join(root, "configs"), 0o755)
				_ = os.WriteFile(filepath.Join(root, "configs", "pg.password"), []byte(pass+"\r\n"), 0o600)
			}
		} else {
			out("  [FAIL] PostgreSQL not connectable with these settings")
			if !p.askYesNo("  Keep this config anyway? 仍然保存该配置", false) {
				out("  -> falling back to bundled PostgreSQL")
				st.PGMode = "bundled"
			}
		}
	} else {
		st.PGMode = "bundled"
	}
	if st.PGMode == "bundled" {
		st.PGHost, st.PGPort, st.PGUser = "", "", ""
	}
	out("")

	// --- Redis ---
	out("Redis:")
	out("  [1] Bundled / auto-detect :6379 (内置或本机已有) — recommended")
	out("  [2] External Redis (外部实例, custom address)")
	if p.ask("Choose Redis mode 选择模式", "1") == "2" {
		st.RedisMode = "system"
		st.RedisAddr = p.ask("  addr (host:port)", "127.0.0.1:6379")
		if host, port, err := net.SplitHostPort(st.RedisAddr); err == nil && tcpOpen(host, port, 2*time.Second) {
			out("  [OK] Redis reachable / 可连接")
		} else {
			out("  [FAIL] Redis " + st.RedisAddr + " unreachable / 不可连接")
			if !p.askYesNo("  Keep this config anyway? 仍然保存该配置", false) {
				st.RedisMode = "bundled"
				st.RedisAddr = ""
			}
		}
	} else {
		st.RedisMode = "bundled"
		st.RedisAddr = ""
	}
	out("")

	// --- auto-start ---
	out("Auto-start on boot / 开机自启动:")
	if p.askYesNo("Register backend auto-start? 注册后端开机自启动", true) {
		kind, err := installAutostart(root, st, log)
		if err != nil {
			out("  [WARN] auto-start registration failed: " + err.Error())
			out("         You can retry later: AraneaLauncher.exe -install-autostart")
		} else {
			st.Autostart = kind
			if kind == "service" {
				out("  [OK] Windows service registered (system PG/Redis mode)")
			} else {
				out("  [OK] Logon scheduled task registered (bundled mode)")
			}
		}
	}
	out("")

	if err := saveSetupState(root, st); err != nil {
		out("[WARN] failed to save setup state: " + err.Error())
	} else {
		out("Configuration saved: configs\\launcher-setup.json")
		log("setup wizard done: pg=%s redis=%s autostart=%s", st.PGMode, st.RedisMode, st.Autostart)
	}
	out("========================================================")
	out("")
	return st
}

// verifyPGChoice probes the user-chosen PostgreSQL with psql (never prompts).
func verifyPGChoice(root string, st *setupState, pass string, out func(string)) bool {
	if !tcpOpen(st.PGHost, st.PGPort, 2*time.Second) {
		return false
	}
	_, psql := findSystemPSQL()
	if psql == "" {
		psql = filepath.Join(root, "postgres", "bin", "psql.exe")
	}
	if _, err := os.Stat(psql); err != nil {
		out("  [WARN] psql.exe not found; TCP open but auth unverified")
		return true // port reachable; give benefit of the doubt
	}
	env := &runtimeEnv{
		Root: root, PGMode: "system",
		PGHost: st.PGHost, PGPort: st.PGPort, PGUser: st.PGUser, PGPass: pass,
		PSQL: psql, PGBinDir: filepath.Dir(psql),
	}
	return canConnectPSQL(env, "postgres")
}

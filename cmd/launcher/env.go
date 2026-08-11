package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type checkLevel string

const (
	checkOK   checkLevel = "OK"
	checkWarn checkLevel = "WARN"
	checkFail checkLevel = "FAIL"
	checkInfo checkLevel = "INFO"
)

type checkItem struct {
	Name   string
	Level  checkLevel
	Detail string
	Fatal  bool
}

type runtimeEnv struct {
	mu sync.Mutex

	Root string

	PGMode   string // system | bundled
	PGHost   string
	PGPort   string
	PGUser   string
	PGPass   string
	PSQL     string
	PGBinDir string
	VectorOK bool

	RedisMode string // system | bundled
	RedisAddr string // host:port

	Checks []checkItem
}

func (e *runtimeEnv) add(name string, level checkLevel, detail string, fatal bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Checks = append(e.Checks, checkItem{Name: name, Level: level, Detail: detail, Fatal: fatal})
}

func (e *runtimeEnv) hasFatal() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.Checks {
		if c.Fatal || c.Level == checkFail {
			return true
		}
	}
	return false
}

func (e *runtimeEnv) hasWarn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.Checks {
		if c.Level == checkWarn {
			return true
		}
	}
	return false
}

func (e *runtimeEnv) reportText() string {
	e.mu.Lock()
	checks := append([]checkItem(nil), e.Checks...)
	e.mu.Unlock()

	var b strings.Builder
	b.WriteString("Aranea-Agents Environment Check\n")
	b.WriteString(strings.Repeat("-", 44) + "\n")
	for _, c := range checks {
		b.WriteString(fmt.Sprintf("[%s] %s\n    %s\n", c.Level, c.Name, c.Detail))
	}
	b.WriteString("\n")
	b.WriteString(e.configGuideText())
	return b.String()
}

// configGuideText explains current DB/Redis choice and how to point at system services.
func (e *runtimeEnv) configGuideText() string {
	pgPassHint := "(none - bundled uses trust / system needs configs\\pg.password)"
	if e.PGPass != "" {
		pgPassHint = "(set via env or configs\\pg.password)"
	}
	var b strings.Builder
	b.WriteString("Database\n")
	b.WriteString(fmt.Sprintf("  mode : %s\n", emptyDefault(e.PGMode, "unknown")))
	b.WriteString(fmt.Sprintf("  addr : %s:%s\n", emptyDefault(e.PGHost, "127.0.0.1"), emptyDefault(e.PGPort, "?")))
	b.WriteString(fmt.Sprintf("  user : %s\n", emptyDefault(e.PGUser, "postgres")))
	b.WriteString(fmt.Sprintf("  pass : %s\n", pgPassHint))
	b.WriteString(fmt.Sprintf("  db   : aranea\n"))
	b.WriteString("\nRedis\n")
	b.WriteString(fmt.Sprintf("  mode : %s\n", emptyDefault(e.RedisMode, "unknown")))
	b.WriteString(fmt.Sprintf("  addr : %s\n", emptyDefault(e.RedisAddr, "127.0.0.1:6379")))
	b.WriteString("\nHow to use system PostgreSQL (:5432)\n")
	b.WriteString("  1) Ensure PostgreSQL is running and listening on 127.0.0.1:5432\n")
	b.WriteString("  2) Run the interactive wizard: AraneaLauncher.exe -setup\n")
	b.WriteString("     (or write the password to ONE line in: configs\\pg.password,\n")
	b.WriteString("     or set user env ARANEA_PG_PASSWORD)\n")
	b.WriteString("  3) Restart AraneaLauncher (Stop first if needed)\n")
	b.WriteString("  If connect still fails, launcher falls back to bundled :5433.\n")
	b.WriteString("\nHow to use system Redis (:6379)\n")
	b.WriteString("  Start Redis on 127.0.0.1:6379 before launching; otherwise bundled Redis starts.\n")
	b.WriteString("\nAuto-start on boot / 开机自启动\n")
	b.WriteString("  Register: AraneaLauncher.exe -install-autostart\n")
	b.WriteString("  Remove  : AraneaLauncher.exe -uninstall-autostart\n")
	b.WriteString("  (system PG+Redis -> Windows service; bundled -> logon scheduled task)\n")
	b.WriteString("\nLogs (UTF-8 with BOM — open with Notepad / VS Code)\n")
	b.WriteString("  logs\\launcher.log   logs\\preflight.txt   logs\\server.log\n")
	return b.String()
}

func emptyDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func (e *runtimeEnv) postgresDSN(db string) string {
	user := e.PGUser
	if user == "" {
		user = "postgres"
	}
	auth := user
	if e.PGPass != "" {
		auth = user + ":" + urlQueryEscape(e.PGPass)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=disable", auth, e.PGHost, e.PGPort, db)
}

// urlQueryEscape is a minimal escape for password special chars in DSN.
func urlQueryEscape(s string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		":", "%3A",
		"@", "%40",
		"/", "%2F",
		"?", "%3F",
		"#", "%23",
		" ", "%20",
	)
	return replacer.Replace(s)
}

func tcpOpen(host, port string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func findSystemPSQL() (binDir, psql string) {
	// PATH first
	if p, err := lookPath("psql.exe"); err == nil {
		return filepath.Dir(p), p
	}
	roots := []string{
		os.Getenv("ProgramFiles") + `\PostgreSQL`,
		os.Getenv("ProgramFiles(x86)") + `\PostgreSQL`,
		`C:\Program Files\PostgreSQL`,
		`C:\Program Files (x86)\PostgreSQL`,
		`D:\Program Files\PostgreSQL`,
		`D:\Program Files (x86)\PostgreSQL`,
		`E:\Program Files\PostgreSQL`,
	}
	// Also scan fixed drives for non-standard Program Files locations.
	for _, letter := range []string{"C", "D", "E", "F"} {
		roots = append(roots,
			letter+`:\Program Files\PostgreSQL`,
			letter+`:\Program Files (x86)\PostgreSQL`,
		)
	}
	best := ""
	seen := map[string]bool{}
	for _, root := range roots {
		if root == `\PostgreSQL` || seen[root] {
			continue
		}
		seen[root] = true
		matches, _ := filepath.Glob(filepath.Join(root, "*", "bin", "psql.exe"))
		for _, m := range matches {
			if best == "" || m > best {
				best = m
			}
		}
	}
	if best != "" {
		return filepath.Dir(best), best
	}
	return "", ""
}

func loadPGPassword(root string) string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_PG_PASSWORD")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("PGPASSWORD")); v != "" {
		return v
	}
	// Optional local file (one line password) — never commit; user-created.
	for _, name := range []string{"pg.password", "postgres.password"} {
		b, err := os.ReadFile(filepath.Join(root, "configs", name))
		if err == nil {
			if line := strings.TrimSpace(strings.Split(string(b), "\n")[0]); line != "" {
				return line
			}
		}
	}
	return ""
}

func lookPath(file string) (string, error) {
	return execLookPath(file)
}

// detectRuntime probes the machine and chooses system vs bundled deps.
func detectRuntime(root string, log func(string, ...any)) *runtimeEnv {
	env := &runtimeEnv{
		Root:      root,
		PGHost:    "127.0.0.1",
		PGUser:    "postgres",
		PGPass:    loadPGPassword(root),
		RedisAddr: "127.0.0.1:6379",
	}

	// --- binaries present ---
	serverExe := filepath.Join(root, "aranea-server.exe")
	if _, err := os.Stat(serverExe); err != nil {
		env.add("Backend binary", checkFail, "missing aranea-server.exe", true)
	} else {
		env.add("Backend binary", checkOK, serverExe, false)
	}
	desktopExe := filepath.Join(root, "frontend", "AraneaAgents.exe")
	if _, err := os.Stat(desktopExe); err != nil {
		env.add("Desktop app", checkFail, "missing frontend\\AraneaAgents.exe", true)
	} else {
		env.add("Desktop app", checkOK, desktopExe, false)
	}

	bundledPSQL := filepath.Join(root, "postgres", "bin", "psql.exe")
	sysBin, sysPSQL := findSystemPSQL()
	if sysPSQL != "" {
		env.add("PostgreSQL client", checkOK, "system: "+sysPSQL, false)
	} else if _, err := os.Stat(bundledPSQL); err == nil {
		env.add("PostgreSQL client", checkOK, "bundled: "+bundledPSQL, false)
	} else {
		env.add("PostgreSQL client", checkFail, "psql.exe not found (system or bundled)", true)
	}

	// --- PostgreSQL: saved setup choice (launcher-setup.json) > auto-detect ---
	st := loadSetupState(root)
	pgHost, pgPort := "127.0.0.1", "5432"
	trySystem := true
	if st != nil {
		switch st.PGMode {
		case "bundled":
			trySystem = false
		case "system":
			pgHost = emptyDefault(st.PGHost, "127.0.0.1")
			pgPort = emptyDefault(st.PGPort, "5432")
			env.PGUser = emptyDefault(st.PGUser, "postgres")
		}
	}
	systemPortOpen := trySystem && tcpOpen(pgHost, pgPort, 400*time.Millisecond)
	if systemPortOpen {
		// Client binary: prefer system install; bundled psql also works for remote hosts.
		if sysPSQL != "" {
			env.PSQL = sysPSQL
			env.PGBinDir = sysBin
		} else {
			env.PSQL = bundledPSQL
			env.PGBinDir = filepath.Join(root, "postgres", "bin")
		}
		env.PGHost = pgHost
		env.PGPort = pgPort
		if canConnectPSQL(env, "postgres") {
			env.PGMode = "system"
			env.add("PostgreSQL", checkOK, fmt.Sprintf("using system instance %s:%s", pgHost, pgPort), false)
		} else {
			env.add("PostgreSQL (system)", checkWarn, fmt.Sprintf("%s:%s is open but non-interactive connect failed (re-run -setup, or write configs\\pg.password / set ARANEA_PG_PASSWORD). Falling back to bundled :5433.", pgHost, pgPort), false)
			systemPortOpen = false
		}
	} else if st != nil && st.PGMode == "system" {
		env.add("PostgreSQL (system)", checkWarn, fmt.Sprintf("configured system PostgreSQL %s:%s unreachable; falling back to bundled :5433", pgHost, pgPort), false)
	}

	if env.PGMode != "system" {
		env.PGMode = "bundled"
		env.PGHost = "127.0.0.1"
		env.PGPort = "5433"
		env.PGUser = "postgres"
		env.PGPass = "" // bundled uses trust
		env.PSQL = bundledPSQL
		env.PGBinDir = filepath.Join(root, "postgres", "bin")
		if _, err := os.Stat(filepath.Join(env.PGBinDir, "pg_ctl.exe")); err != nil {
			env.add("PostgreSQL", checkFail, "bundled PostgreSQL missing and system instance unavailable", true)
		} else {
			detail := "using bundled instance 127.0.0.1:5433"
			if systemPortOpen {
				detail += " (system :5432 unavailable or unauthorized)"
			}
			env.add("PostgreSQL", checkOK, detail, false)
		}
	}

	// --- Redis: saved setup choice > auto-detect ---
	if st != nil && st.RedisMode == "system" && st.RedisAddr != "" {
		env.RedisAddr = st.RedisAddr
		host, port, err := net.SplitHostPort(st.RedisAddr)
		if err == nil && tcpOpen(host, port, 500*time.Millisecond) {
			env.RedisMode = "system"
			env.add("Redis", checkOK, "using configured instance "+st.RedisAddr, false)
		} else {
			env.RedisMode = "system" // never start bundled against a custom addr
			env.add("Redis", checkFail, "configured Redis "+st.RedisAddr+" unreachable", true)
		}
	} else if tcpOpen("127.0.0.1", "6379", 300*time.Millisecond) {
		env.RedisMode = "system"
		env.add("Redis", checkOK, "using system/running instance 127.0.0.1:6379", false)
	} else {
		env.RedisMode = "bundled"
		redisExe := filepath.Join(root, "redis", "redis-server.exe")
		if _, err := os.Stat(redisExe); err != nil {
			env.add("Redis", checkFail, "Redis not detected and bundled redis-server.exe missing", true)
		} else {
			env.add("Redis", checkOK, "will start bundled Redis 127.0.0.1:6379", false)
		}
	}

	// ports that must be free for backend
	if tcpOpen("127.0.0.1", "8800", 200*time.Millisecond) {
		if healthy() {
			env.add("Port 8800", checkInfo, "backend already running and healthy", false)
		} else {
			env.add("Port 8800", checkWarn, "in use but /healthz not ready; launcher will try to take over", false)
		}
	} else {
		env.add("Port 8800", checkOK, "available", false)
	}

	_ = log
	return env
}

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	Name    string
	Level   checkLevel
	Detail  string
	Fatal   bool
}

type runtimeEnv struct {
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
	e.Checks = append(e.Checks, checkItem{Name: name, Level: level, Detail: detail, Fatal: fatal})
}

func (e *runtimeEnv) hasFatal() bool {
	for _, c := range e.Checks {
		if c.Fatal || c.Level == checkFail {
			return true
		}
	}
	return false
}

func (e *runtimeEnv) hasWarn() bool {
	for _, c := range e.Checks {
		if c.Level == checkWarn {
			return true
		}
	}
	return false
}

func (e *runtimeEnv) reportText() string {
	var b strings.Builder
	b.WriteString("Aranea-Agents 环境检查\n")
	b.WriteString(strings.Repeat("─", 36) + "\n")
	for _, c := range e.Checks {
		b.WriteString(fmt.Sprintf("[%s] %s\n    %s\n", c.Level, c.Name, c.Detail))
	}
	return b.String()
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
		env.add("后端程序", checkFail, "缺少 aranea-server.exe", true)
	} else {
		env.add("后端程序", checkOK, serverExe, false)
	}
	electronExe := filepath.Join(root, "frontend", "AraneaAgents.exe")
	if _, err := os.Stat(electronExe); err != nil {
		env.add("桌面应用", checkFail, "缺少 frontend\\AraneaAgents.exe", true)
	} else {
		env.add("桌面应用", checkOK, electronExe, false)
	}

	bundledPSQL := filepath.Join(root, "postgres", "bin", "psql.exe")
	sysBin, sysPSQL := findSystemPSQL()
	if sysPSQL != "" {
		env.add("PostgreSQL 客户端", checkOK, "系统: "+sysPSQL, false)
	} else if _, err := os.Stat(bundledPSQL); err == nil {
		env.add("PostgreSQL 客户端", checkOK, "内置: "+bundledPSQL, false)
	} else {
		env.add("PostgreSQL 客户端", checkFail, "未找到 psql.exe（系统或内置）", true)
	}

	// --- PostgreSQL: prefer system :5432 if reachable & connectable ---
	systemPortOpen := tcpOpen("127.0.0.1", "5432", 800*time.Millisecond)
	if systemPortOpen && sysPSQL != "" {
		env.PSQL = sysPSQL
		env.PGBinDir = sysBin
		env.PGPort = "5432"
		if canConnectPSQL(env, "postgres") {
			env.PGMode = "system"
			env.add("PostgreSQL", checkOK, "使用系统实例 127.0.0.1:5432", false)
		} else {
			env.add("PostgreSQL(系统)", checkWarn, "检测到 :5432 但无法以当前凭据连接（可设环境变量 ARANEA_PG_PASSWORD）。将改用内置实例。", false)
			systemPortOpen = false
		}
	}

	if env.PGMode != "system" {
		env.PGMode = "bundled"
		env.PGPort = "5433"
		env.PGPass = "" // bundled uses trust
		env.PSQL = bundledPSQL
		env.PGBinDir = filepath.Join(root, "postgres", "bin")
		if _, err := os.Stat(filepath.Join(env.PGBinDir, "pg_ctl.exe")); err != nil {
			env.add("PostgreSQL", checkFail, "内置 PostgreSQL 缺失且系统实例不可用", true)
		} else {
			detail := "使用内置实例 127.0.0.1:5433"
			if systemPortOpen {
				detail += "（系统 :5432 不可用或无权限）"
			}
			env.add("PostgreSQL", checkOK, detail, false)
		}
	}

	// --- Redis: prefer existing :6379 ---
	if tcpOpen("127.0.0.1", "6379", 500*time.Millisecond) {
		env.RedisMode = "system"
		env.add("Redis", checkOK, "使用系统/已运行实例 127.0.0.1:6379", false)
	} else {
		env.RedisMode = "bundled"
		redisExe := filepath.Join(root, "redis", "redis-server.exe")
		if _, err := os.Stat(redisExe); err != nil {
			env.add("Redis", checkFail, "未检测到 Redis，且缺少内置 redis-server.exe", true)
		} else {
			env.add("Redis", checkOK, "将启动内置 Redis 127.0.0.1:6379", false)
		}
	}

	// ports that must be free for backend
	if tcpOpen("127.0.0.1", "8000", 300*time.Millisecond) {
		if healthy() {
			env.add("端口 8000", checkInfo, "后端已在运行且健康", false)
		} else {
			env.add("端口 8000", checkWarn, "已被占用但 /healthz 未就绪，启动器将尝试接管", false)
		}
	} else {
		env.add("端口 8000", checkOK, "可用", false)
	}

	_ = log
	return env
}

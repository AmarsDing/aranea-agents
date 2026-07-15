// AraneaLauncher — silent Windows desktop launcher for the portable install.
//
// Built with -H windowsgui so double-clicking never shows a console.
// Starts PostgreSQL → Redis → aranea-server, waits for /healthz, then opens
// the Electron desktop app. Pass -stop to tear everything down.
//
// On failure a MessageBox points the user at logs\launcher.log.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	appTitle       = "Aranea-Agents"
	healthURL      = "http://127.0.0.1:8000/healthz"
	pgPort         = "5433"
	redisPort      = "6379"
	backendWaitSec = 45
	mutexName      = "Local\\AraneaAgentsLauncherMutex"
)

func main() {
	stop := flag.Bool("stop", false, "stop all Aranea-Agents services")
	flag.Parse()

	root, err := installRoot()
	if err != nil {
		showError("无法定位安装目录", err.Error())
		os.Exit(1)
	}

	logPath := filepath.Join(root, "logs", "launcher.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		showError("无法写日志", err.Error())
		os.Exit(1)
	}
	defer logf.Close()
	logger := func(format string, args ...any) {
		line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
		_, _ = io.WriteString(logf, line)
	}

	if *stop {
		logger("stop requested")
		if err := stopAll(root, logger); err != nil {
			logger("stop error: %v", err)
			showError("停止失败", err.Error()+"\n\n详见: "+logPath)
			os.Exit(1)
		}
		return
	}

	releaseMutex, already := acquireSingleInstance()
	if already {
		logger("another launcher is running; focusing desktop app")
		_ = launchElectron(root, logger)
		return
	}
	defer releaseMutex()

	logger("start begin root=%s", root)
	if err := ensureEnv(root); err != nil {
		logger("env error: %v", err)
		showError("环境准备失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	if err := startPostgres(root, logger); err != nil {
		logger("postgres error: %v", err)
		showError("PostgreSQL 启动失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	if err := startRedis(root, logger); err != nil {
		logger("redis error: %v", err)
		showError("Redis 启动失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	if err := startBackend(root, logger); err != nil {
		logger("backend error: %v", err)
		showError("后端服务启动失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	if err := waitHealthy(logger); err != nil {
		logger("health error: %v", err)
		showError("后端未就绪", err.Error()+"\n\n详见: "+logPath+"\n与 logs\\server.log")
		os.Exit(1)
	}
	if err := launchElectron(root, logger); err != nil {
		logger("electron error: %v", err)
		showError("桌面应用启动失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	logger("start complete")
}

func installRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func ensureEnv(root string) error {
	_ = os.Setenv("KRATOS_AUTH_SECRET", "aranea-portable-dev-secret-32chars!!")
	_ = os.Setenv("DEPLOY_ENV", "dev")
	_ = os.Setenv("DAO_VECTOR_PGVECTOR", "1")
	_ = os.Setenv("PGDATA", filepath.Join(root, "postgres", "data"))
	_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	return nil
}

func startPostgres(root string, log func(string, ...any)) error {
	pgBin := filepath.Join(root, "postgres", "bin")
	pgData := filepath.Join(root, "postgres", "data")
	initdb := filepath.Join(pgBin, "initdb.exe")
	pgCtl := filepath.Join(pgBin, "pg_ctl.exe")
	psql := filepath.Join(pgBin, "psql.exe")
	pgIsReady := filepath.Join(pgBin, "pg_isready.exe")
	logDir := filepath.Join(root, "logs")

	if _, err := os.Stat(pgCtl); err != nil {
		return fmt.Errorf("缺少 PostgreSQL: %w", err)
	}

	if _, err := os.Stat(filepath.Join(pgData, "PG_VERSION")); err != nil {
		log("initializing postgres data dir")
		cmd := hiddenCmd(initdb, "-D", pgData, "-U", "postgres", "--auth=trust", "--encoding=UTF8")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		_ = os.WriteFile(filepath.Join(logDir, "initdb.log"), out, 0o644)
		if err != nil {
			return fmt.Errorf("initdb failed: %w", err)
		}
	}

	confPath := filepath.Join(pgData, "postgresql.conf")
	if data, err := os.ReadFile(confPath); err == nil {
		if !strings.Contains(string(data), "lc_messages = 'C'") {
			f, err := os.OpenFile(confPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				_, _ = f.WriteString("\nlc_messages = 'C'\n")
				_ = f.Close()
			}
		}
	}

	log("starting postgres on :%s", pgPort)
	cmd := hiddenCmd(pgCtl, "start", "-D", pgData, "-l", filepath.Join(logDir, "postgres.log"), "-o", "-p "+pgPort, "-w")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	_ = os.WriteFile(filepath.Join(logDir, "pgctl.log"), out, 0o644)
	if err != nil {
		// May already be running — continue and probe readiness.
		log("pg_ctl start returned: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		probe := hiddenCmd(pgIsReady, "-U", "postgres", "-h", "127.0.0.1", "-p", pgPort)
		if err := probe.Run(); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	check := hiddenCmd(psql, "-U", "postgres", "-h", "127.0.0.1", "-p", pgPort, "-tc", "SELECT 1 FROM pg_database WHERE datname = 'aranea'")
	out, _ = check.CombinedOutput()
	if !strings.Contains(string(out), "1") {
		log("creating database aranea")
		_ = hiddenCmd(psql, "-U", "postgres", "-h", "127.0.0.1", "-p", pgPort, "-c", "CREATE DATABASE aranea").Run()
		_ = hiddenCmd(psql, "-U", "postgres", "-h", "127.0.0.1", "-p", pgPort, "-d", "aranea", "-c", "CREATE EXTENSION IF NOT EXISTS vector").Run()
	}
	return nil
}

func startRedis(root string, log func(string, ...any)) error {
	if processRunning("redis-server.exe") {
		log("redis already running")
		return nil
	}
	exe := filepath.Join(root, "redis", "redis-server.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("缺少 Redis: %w", err)
	}
	log("starting redis on :%s", redisPort)
	cmd := hiddenCmd(exe, "--port", redisPort, "--bind", "127.0.0.1")
	cmd.Dir = filepath.Join(root, "redis")
	if err := cmd.Start(); err != nil {
		return err
	}
	// Detach — redis keeps running after launcher exits.
	go func() { _ = cmd.Wait() }()
	time.Sleep(500 * time.Millisecond)
	return nil
}

func startBackend(root string, log func(string, ...any)) error {
	if healthy() {
		log("backend already healthy")
		return nil
	}
	_ = killImage("aranea-server.exe")
	exe := filepath.Join(root, "aranea-server.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("缺少后端: %w", err)
	}
	logDir := filepath.Join(root, "logs")
	stdout, err := os.OpenFile(filepath.Join(logDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := hiddenCmd(exe, "-conf", "configs")
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	cmd.Env = append(os.Environ(),
		"KRATOS_AUTH_SECRET=aranea-portable-dev-secret-32chars!!",
		"DEPLOY_ENV=dev",
		"DAO_VECTOR_PGVECTOR=1",
	)
	log("starting aranea-server")
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = stdout.Close()
	}()
	return nil
}

func waitHealthy(log func(string, ...any)) error {
	deadline := time.Now().Add(time.Duration(backendWaitSec) * time.Second)
	for time.Now().Before(deadline) {
		if healthy() {
			log("backend healthy")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("healthz 在 %ds 内未就绪", backendWaitSec)
}

func healthy() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func launchElectron(root string, log func(string, ...any)) error {
	if processRunning("AraneaAgents.exe") {
		log("desktop app already running")
		return nil
	}
	exe := filepath.Join(root, "frontend", "AraneaAgents.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("缺少桌面应用: %w", err)
	}
	log("launching desktop app")
	cmd := hiddenCmd(exe)
	cmd.Dir = filepath.Join(root, "frontend")
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func stopAll(root string, log func(string, ...any)) error {
	_ = killImage("AraneaAgents.exe")
	_ = killImage("aranea-server.exe")
	_ = killImage("redis-server.exe")
	pgCtl := filepath.Join(root, "postgres", "bin", "pg_ctl.exe")
	pgData := filepath.Join(root, "postgres", "data")
	if _, err := os.Stat(pgCtl); err == nil {
		cmd := hiddenCmd(pgCtl, "stop", "-D", pgData, "-m", "fast")
		_ = cmd.Run()
	}
	log("all services stopped")
	return nil
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideConsoleWindow(cmd)
	return cmd
}

func processRunning(image string) bool {
	cmd := hiddenCmd("tasklist", "/FI", "IMAGENAME eq "+image, "/NH")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(image))
}

func killImage(image string) error {
	cmd := hiddenCmd("taskkill", "/IM", image, "/F", "/T")
	_ = cmd.Run()
	return nil
}

func acquireSingleInstance() (release func(), alreadyRunning bool) {
	if runtime.GOOS != "windows" {
		return func() {}, false
	}
	return acquireMutexWindows(mutexName)
}

func showError(title, msg string) {
	if runtime.GOOS == "windows" {
		messageBoxWindows(appTitle+" — "+title, msg)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
}

// Windows-specific helpers live in main_windows.go / main_stub.go.
var (
	acquireMutexWindows func(name string) (func(), bool)
	messageBoxWindows   func(title, msg string)
)

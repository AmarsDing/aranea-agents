// AraneaLauncher — silent Windows desktop launcher for the portable install.
//
// Built with -H windowsgui so double-clicking never shows a console.
// Detects system PostgreSQL/Redis when available, otherwise starts bundled
// instances. Runs environment preflight checks and shows errors clearly.
//
// Flags:
//
//	(default)  start stack + desktop app
//	-stop      stop bundled services (does not stop system PG/Redis)
//	-check     run environment checks and show report (no start)
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
	backendWaitSec = 60
	mutexName      = "Local\\AraneaAgentsLauncherMutex"
)

func main() {
	stop := flag.Bool("stop", false, "stop Aranea-Agents services")
	checkOnly := flag.Bool("check", false, "run environment checks and exit")
	quiet := flag.Bool("quiet", false, "with -check: write report to file only, no UI")
	noConsole := flag.Bool("no-console", false, "do not open the startup status console")
	flag.Parse()

	root, err := installRoot()
	if err != nil {
		showError("无法定位安装目录", err.Error())
		os.Exit(1)
	}

	logPath, logf := openLauncherLog(root)
	defer logf.Close()

	var ui *statusConsole
	logger := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
		_, _ = io.WriteString(logf, line)
		if ui != nil {
			ui.Println(msg)
		}
	}
	logger("launcher start: root=%s logPath=%s args=%v", root, logPath, os.Args[1:])

	if *stop {
		logger("stop requested")
		if err := stopAll(root, logger); err != nil {
			logger("stop error: %v", err)
			showError("停止失败", err.Error()+"\n\n详见: "+logPath)
			os.Exit(1)
		}
		return
	}

	wantConsole := !*quiet && !*noConsole
	if wantConsole {
		ui = openStatusConsole(appTitle + " - Environment")
		defer ui.Close()
	}

	if *checkOnly {
		// Lightweight only: never start services / never block on password prompts.
		env := detectRuntime(root, logger)
		report := env.reportText() + "\nLog: " + logPath
		_ = writeFileUTF8BOM(filepath.Join(root, "logs", "preflight.txt"), report)
		if ui != nil {
			ui.Println(report)
			ui.WaitDismiss("Press Enter to close this window...")
		} else if !*quiet {
			showInfo("环境检查", report)
		}
		if env.hasFatal() {
			os.Exit(1)
		}
		return
	}

	releaseMutex, already := acquireSingleInstance()
	if already {
		logger("another launcher holds mutex; waiting for backend health")
		if waitHealthy(logger) == nil {
			_ = launchDesktopApp(root, logger)
			if ui != nil {
				ui.Println("Desktop already starting — closing console.")
				time.Sleep(800 * time.Millisecond)
			}
			return
		}
		showError("启动器已在运行",
			"检测到另有启动过程正在进行，但后端尚未就绪。\n\n请稍候桌面窗口出现，或先执行「停止」后再启动。\n\n详见: "+logPath)
		return
	}
	defer releaseMutex()

	logger("start begin root=%s", root)
	_ = os.Setenv("KRATOS_AUTH_SECRET", "aranea-portable-dev-secret-32chars!!")
	_ = os.Setenv("DEPLOY_ENV", "dev")
	_ = os.Setenv("DAO_VECTOR_PGVECTOR", "1")

	// Fast path: stack already healthy → skip PG/Redis/backend bring-up.
	if healthy() {
		logger("fast-path: backend already healthy, launching desktop")
		if err := launchDesktopApp(root, logger); err != nil {
			logger("desktop app error: %v", err)
			showError("桌面应用启动失败", err.Error()+"\n\n详见: "+logPath)
			os.Exit(1)
		}
		logger("start complete (fast-path)")
		if ui != nil {
			time.Sleep(600 * time.Millisecond)
		}
		return
	}

	env := detectRuntime(root, logger)
	if ui != nil {
		ui.Println(env.reportText())
	}
	if env.hasFatal() {
		logger("preflight fatal\n%s", env.reportText())
		_ = writeFileUTF8BOM(filepath.Join(root, "logs", "preflight.txt"), env.reportText()+"\nLog: "+logPath)
		if ui != nil {
			ui.WaitDismiss("Environment check FAILED. Press Enter to close...")
		}
		showError("环境检查未通过", env.reportText()+"\n详见: "+logPath)
		os.Exit(1)
	}

	// Parallel: Redis while Postgres starts/initializes (biggest cold-start win after initdb).
	logger("ensurePostgres begin mode=%s port=%s", env.PGMode, env.PGPort)
	logger("ensureRedis begin mode=%s (parallel)", env.RedisMode)
	redisErrCh := make(chan error, 1)
	go func() { redisErrCh <- ensureRedis(env, logger) }()

	if err := ensurePostgres(env, logger); err != nil {
		logger("postgres error: %v\n%s", err, env.reportText())
		showError("PostgreSQL 未就绪", env.reportText()+"\n\n"+err.Error()+"\n\n详见: "+logPath+"\n与 logs\\postgres.log / initdb.log")
		os.Exit(1)
	}
	logger("ensurePostgres ok")
	if err := <-redisErrCh; err != nil {
		logger("redis error: %v", err)
		showError("Redis 未就绪", env.reportText()+"\n\n"+err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	logger("ensureRedis ok")
	if err := writeRuntimeConfig(env, logger); err != nil {
		logger("config error: %v", err)
		showError("写入配置失败", err.Error())
		os.Exit(1)
	}
	_ = writeModeFile(root, env)

	report := env.reportText()
	_ = writeFileUTF8BOM(filepath.Join(root, "logs", "preflight.txt"), report)
	logger("preflight saved (UTF-8 BOM)")

	logger("starting backend...")
	if err := startBackend(root, env, logger); err != nil {
		logger("backend error: %v", err)
		showError("后端服务启动失败", env.reportText()+"\n\n"+err.Error()+"\n\n详见: "+logPath+"\n与 logs\\server.log")
		os.Exit(1)
	}
	if err := waitHealthy(logger); err != nil {
		logger("health error: %v\n%s", err, env.reportText())
		tail := readLogTail(filepath.Join(root, "logs", "server.log"), 25)
		showError("后端未就绪", env.reportText()+"\n\n"+err.Error()+
			"\n\n常见原因：数据库未连接、Redis 未就绪、端口被占用。"+
			"\n\n--- logs\\server.log (末尾) ---\n"+tail+
			"\n\n详见: "+logPath)
		os.Exit(1)
	}
	env.add("Backend health", checkOK, healthURL, false)
	_ = writeFileUTF8BOM(filepath.Join(root, "logs", "preflight.txt"), env.reportText())

	if err := launchDesktopApp(root, logger); err != nil {
		logger("desktop app error: %v", err)
		showError("桌面应用启动失败", err.Error()+"\n\n详见: "+logPath)
		os.Exit(1)
	}
	logger("start complete")
	if ui != nil {
		ui.Println("")
		ui.Println("Desktop launched. This console will close shortly...")
		ui.Println("Tip: Start Menu → Environment Check  to re-open guidance anytime.")
		time.Sleep(1200 * time.Millisecond)
	}
}

func installRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// openLauncherLog opens the launcher log file, falling back to %TEMP% if the
// installation directory is not writable (e.g. user installed to a path under
// Program Files without elevation). Without this fallback, launcher would
// silently os.Exit(1) before writing any log, leaving users with no clue why
// the app "flashes and disappears".
//
// Returns the effective log path and an open *os.File (never nil on success).
// On total failure, calls showError + os.Exit(1).
//
// 注意：此函数不弹任何 MessageBox（即使在 fallback 时），避免在 NSIS -quiet 模式下
// 阻塞安装器。日志路径会在 -check 报告和 showError 消息中体现，用户可据此查找日志。
func openLauncherLog(root string) (string, *os.File) {
	preferred := filepath.Join(root, "logs", "launcher.log")
	_ = os.MkdirAll(filepath.Dir(preferred), 0o755)
	if f, err := os.OpenFile(preferred, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		ensureLogBOM(f)
		return preferred, f
	}
	// Fallback: %TEMP%\aranea-launcher.log (user-temp is always writable).
	fallback := filepath.Join(os.TempDir(), "aranea-launcher.log")
	if f, err := os.OpenFile(fallback, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		ensureLogBOM(f)
		_, _ = io.WriteString(f, fmt.Sprintf("%s [WARN] install dir not writable; log fallback to %s\n",
			time.Now().Format("2006-01-02 15:04:05"), fallback))
		return fallback, f
	}
	// Both paths failed — extremely rare. Surface the error and exit.
	showError("无法写日志",
		"无法在以下位置创建日志:\n"+preferred+"\n"+fallback+
			"\n\n请检查文件系统权限或磁盘空间。")
	os.Exit(1)
	return "", nil // unreachable
}

func startBackend(root string, env *runtimeEnv, log func(string, ...any)) error {
	if healthy() {
		log("backend already healthy")
		env.add("Backend", checkOK, "already running", false)
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
	log("starting aranea-server (pg=%s:%s)", env.PGHost, env.PGPort)
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = stdout.Close()
	}()
	env.add("Backend", checkOK, "process started", false)
	return nil
}

func waitHealthy(log func(string, ...any)) error {
	deadline := time.Now().Add(time.Duration(backendWaitSec) * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		ok, body := healthStatus()
		lastBody = body
		if ok {
			log("backend healthy")
			return nil
		}
		if body != "" {
			log("healthz not ready: %s", trimHealthBody(body))
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastBody != "" {
		return fmt.Errorf("healthz not ready within %ds: %s", backendWaitSec, trimHealthBody(lastBody))
	}
	return fmt.Errorf("healthz not ready within %ds", backendWaitSec)
}

func healthy() bool {
	ok, _ := healthStatus()
	return ok
}

func healthStatus() (ok bool, body string) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return resp.StatusCode == http.StatusOK, strings.TrimSpace(string(b))
}

func trimHealthBody(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func launchDesktopApp(root string, log func(string, ...any)) error {
	if processRunning("AraneaAgents.exe") {
		log("desktop app already running")
		return nil
	}
	exe := filepath.Join(root, "frontend", "AraneaAgents.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("缺少桌面应用: %w", err)
	}
	log("launching desktop app (GUI, visible)")
	// CRITICAL: must NOT use hiddenCmd — HideWindow makes the GUI window invisible.
	cmd := guiCmd(exe)
	cmd.Dir = filepath.Join(root, "frontend")
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	// Brief settle; if it exits immediately, surface failure.
	time.Sleep(300 * time.Millisecond)
	if !processRunning("AraneaAgents.exe") {
		return fmt.Errorf("AraneaAgents.exe 启动后立即退出，请检查 frontend 目录是否完整")
	}
	return nil
}

func stopAll(root string, log func(string, ...any)) error {
	mode := readModeFile(root)
	_ = killImage("AraneaAgents.exe")
	_ = killImage("aranea-server.exe")
	// Never stop system Redis/PostgreSQL — only what we started.
	if mode.StartedBundledRedis || mode.RedisMode == "bundled" {
		_ = killImage("redis-server.exe")
		log("stopped bundled redis")
	} else {
		log("skip system redis stop")
	}
	if mode.StartedBundledPG || mode.PGMode == "bundled" {
		pgCtl := filepath.Join(root, "postgres", "bin", "pg_ctl.exe")
		pgData := filepath.Join(root, "postgres", "data")
		if _, err := os.Stat(pgCtl); err == nil {
			if _, err := os.Stat(filepath.Join(pgData, "PG_VERSION")); err == nil {
				cmd := hiddenCmd(pgCtl, "stop", "-D", pgData, "-m", "fast")
				_ = cmd.Run()
				log("stopped bundled postgres")
			}
		}
	} else {
		log("skip system postgres stop")
	}
	log("all aranea-managed services stopped")
	return nil
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideConsoleWindow(cmd)
	return cmd
}

func guiCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	showGUIWindow(cmd)
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
		messageBoxWindows(appTitle+" — "+title, msg, true)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
}

func showInfo(title, msg string) {
	if runtime.GOOS == "windows" {
		messageBoxWindows(appTitle+" — "+title, msg, false)
		return
	}
	fmt.Fprintf(os.Stdout, "%s: %s\n", title, msg)
}

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func readLogTail(path string, maxLines int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(无法读取 " + path + ")"
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	s := strings.TrimSpace(strings.Join(lines, "\n"))
	if s == "" {
		return "(日志为空)"
	}
	if len(s) > 1500 {
		return s[len(s)-1500:]
	}
	return s
}

// Windows-specific helpers live in main_windows.go / main_stub.go.
var (
	acquireMutexWindows func(name string) (func(), bool)
	messageBoxWindows   func(title, msg string, isError bool)
)

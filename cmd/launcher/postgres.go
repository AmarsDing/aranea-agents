package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func canConnectPSQL(env *runtimeEnv, db string) bool {
	out, err := runPSQL(env, db, "-tAc", "SELECT 1")
	return err == nil && strings.Contains(strings.TrimSpace(out), "1")
}

func psqlArgs(env *runtimeEnv, db string, extra ...string) []string {
	// -w / --no-password: NEVER prompt (prompt hangs forever under windowsgui / NSIS).
	args := []string{
		"-w",
		"-U", env.PGUser,
		"-h", env.PGHost,
		"-p", env.PGPort,
		"-d", db,
	}
	return append(args, extra...)
}

func runPSQL(env *runtimeEnv, db string, extra ...string) (string, error) {
	if env.PSQL == "" {
		return "", fmt.Errorf("psql path empty")
	}
	cmd := hiddenCmd(env.PSQL, psqlArgs(env, db, extra...)...)
	applyPGEnv(cmd, env)
	return runWithTimeout(cmd, 8*time.Second)
}

func applyPGEnv(cmd *exec.Cmd, env *runtimeEnv) {
	cmd.Env = append(os.Environ(),
		"PGCLIENTENCODING=UTF8",
		"PGCONNECT_TIMEOUT=3", // fail fast instead of hanging
	)
	if env.PGPass != "" {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+env.PGPass)
	}
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return buf.String(), fmt.Errorf("command timed out after %s", timeout)
	}
}

func ensurePostgres(env *runtimeEnv, log func(string, ...any)) error {
	logDir := filepath.Join(env.Root, "logs")
	_ = os.MkdirAll(logDir, 0o755)

	if env.PGMode == "bundled" {
		if err := startBundledPostgres(env, log); err != nil {
			env.add("PostgreSQL 启动", checkFail, err.Error(), true)
			return err
		}
	} else {
		log("using system postgres :%s", env.PGPort)
	}

	if err := waitPGReady(env, 30*time.Second, log); err != nil {
		env.add("PostgreSQL 就绪", checkFail, err.Error(), true)
		return err
	}
	env.add("PostgreSQL 就绪", checkOK, fmt.Sprintf("%s:%s 可连接", env.PGHost, env.PGPort), false)

	out, err := runPSQL(env, "postgres", "-tAc", "SELECT 1 FROM pg_database WHERE datname='aranea'")
	if err != nil {
		env.add("数据库 aranea", checkFail, "无法查询: "+strings.TrimSpace(out)+" "+err.Error(), true)
		return fmt.Errorf("query databases: %w", err)
	}
	if !strings.Contains(out, "1") {
		log("creating database aranea")
		out, err = runPSQL(env, "postgres", "-c", "CREATE DATABASE aranea")
		if err != nil {
			env.add("数据库 aranea", checkFail, "创建失败: "+strings.TrimSpace(out), true)
			return fmt.Errorf("create database: %w", err)
		}
	}
	env.add("数据库 aranea", checkOK, "已就绪", false)

	if err := ensurePgvector(env, log); err != nil {
		env.add("pgvector 扩展", checkWarn, err.Error()+"；向量检索可能不可用", false)
		env.VectorOK = false
		log("pgvector warn: %v", err)
	} else {
		env.VectorOK = true
		env.add("pgvector 扩展", checkOK, "CREATE EXTENSION vector 成功", false)
	}
	return nil
}

func startBundledPostgres(env *runtimeEnv, log func(string, ...any)) error {
	pgBin := env.PGBinDir
	pgData := filepath.Join(env.Root, "postgres", "data")
	initdb := filepath.Join(pgBin, "initdb.exe")
	pgCtl := filepath.Join(pgBin, "pg_ctl.exe")
	logDir := filepath.Join(env.Root, "logs")

	if tcpOpen(env.PGHost, env.PGPort, 500*time.Millisecond) {
		log("bundled postgres port %s already open", env.PGPort)
		return nil
	}

	if _, err := os.Stat(filepath.Join(pgData, "PG_VERSION")); err != nil {
		log("initializing postgres data dir (locale=C)")
		cmd := hiddenCmd(initdb,
			"-D", pgData,
			"-U", "postgres",
			"--auth=trust",
			"--encoding=UTF8",
			"--locale=C",
			"--lc-messages=C",
		)
		cmd.Dir = env.Root
		cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
		out, err := runWithTimeout(cmd, 120*time.Second)
		_ = os.WriteFile(filepath.Join(logDir, "initdb.log"), []byte(out), 0o644)
		if err != nil {
			cmd = hiddenCmd(initdb, "-D", pgData, "-U", "postgres", "--auth=trust", "--encoding=UTF8")
			cmd.Dir = env.Root
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
			out, err = runWithTimeout(cmd, 120*time.Second)
			_ = os.WriteFile(filepath.Join(logDir, "initdb.log"), []byte(out), 0o644)
			if err != nil {
				return fmt.Errorf("initdb 失败: %v\n%s", err, trimOut([]byte(out)))
			}
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

	log("starting bundled postgres on :%s", env.PGPort)
	cmd := hiddenCmd(pgCtl, "start", "-D", pgData,
		"-l", filepath.Join(logDir, "postgres.log"),
		"-o", "-p "+env.PGPort,
		"-w", "-t", "30",
	)
	cmd.Dir = env.Root
	out, err := runWithTimeout(cmd, 60*time.Second)
	_ = os.WriteFile(filepath.Join(logDir, "pgctl.log"), []byte(out), 0o644)
	if err != nil && !tcpOpen(env.PGHost, env.PGPort, time.Second) {
		return fmt.Errorf("pg_ctl start 失败: %v\n%s", err, trimOut([]byte(out)))
	}
	return nil
}

func waitPGReady(env *runtimeEnv, timeout time.Duration, log func(string, ...any)) error {
	deadline := time.Now().Add(timeout)
	pgIsReady := filepath.Join(env.PGBinDir, "pg_isready.exe")
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pgIsReady); err == nil {
			cmd := hiddenCmd(pgIsReady, "-U", env.PGUser, "-h", env.PGHost, "-p", env.PGPort, "-t", "2")
			applyPGEnv(cmd, env)
			_, _ = runWithTimeout(cmd, 4*time.Second)
		}
		if canConnectPSQL(env, "postgres") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("PostgreSQL 在 %v 内未在 %s:%s 就绪", timeout, env.PGHost, env.PGPort)
}

func ensurePgvector(env *runtimeEnv, log func(string, ...any)) error {
	out, err := runPSQL(env, "aranea", "-tAc", "SELECT 1 FROM pg_extension WHERE extname='vector'")
	if err == nil && strings.Contains(out, "1") {
		log("pgvector already installed")
		return nil
	}
	out, err = runPSQL(env, "aranea", "-c", "CREATE EXTENSION IF NOT EXISTS vector")
	if err == nil {
		log("CREATE EXTENSION vector ok")
		return nil
	}
	log("CREATE EXTENSION vector failed: %s", trimOut([]byte(out)))

	if err2 := installBundledVectorFiles(env, log); err2 != nil {
		return fmt.Errorf("无法启用 pgvector: %s；%v", strings.TrimSpace(out), err2)
	}
	out, err = runPSQL(env, "aranea", "-c", "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("已复制 vector 文件但仍失败: %s", trimOut([]byte(out)))
	}
	return nil
}

func installBundledVectorFiles(env *runtimeEnv, log func(string, ...any)) error {
	srcLib := filepath.Join(env.Root, "postgres", "lib", "vector.dll")
	if _, err := os.Stat(srcLib); err != nil {
		return fmt.Errorf("内置包缺少 postgres\\lib\\vector.dll（向量功能将降级）")
	}
	pgRoot := filepath.Dir(env.PGBinDir)
	destLib := filepath.Join(pgRoot, "lib", "vector.dll")
	destShare := filepath.Join(pgRoot, "share", "extension")
	_ = os.MkdirAll(destShare, 0o755)

	log("copying vector.dll -> %s", destLib)
	if err := copyFile(srcLib, destLib); err != nil {
		return fmt.Errorf("复制 vector.dll 失败（系统 PostgreSQL 可能需要管理员权限）: %w", err)
	}

	srcShare := filepath.Join(env.Root, "postgres", "share", "extension")
	matches, _ := filepath.Glob(filepath.Join(srcShare, "vector*"))
	for _, m := range matches {
		_ = copyFile(m, filepath.Join(destShare, filepath.Base(m)))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o644)
}

func trimOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 800 {
		return s[:800] + "..."
	}
	return s
}

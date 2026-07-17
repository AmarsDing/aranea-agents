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
			env.add("PostgreSQL start", checkFail, err.Error(), true)
			return err
		}
	} else {
		log("using system postgres :%s", env.PGPort)
	}

	if err := waitPGReady(env, 30*time.Second, log); err != nil {
		env.add("PostgreSQL ready", checkFail, err.Error(), true)
		return err
	}
	env.add("PostgreSQL ready", checkOK, fmt.Sprintf("%s:%s connectable", env.PGHost, env.PGPort), false)

	out, err := runPSQL(env, "postgres", "-tAc", "SELECT 1 FROM pg_database WHERE datname='aranea'")
	if err != nil {
		env.add("Database aranea", checkFail, "query failed: "+strings.TrimSpace(out)+" "+err.Error(), true)
		return fmt.Errorf("query databases: %w", err)
	}
	if !strings.Contains(out, "1") {
		log("creating database aranea")
		out, err = runPSQL(env, "postgres", "-c", "CREATE DATABASE aranea")
		if err != nil {
			env.add("Database aranea", checkFail, "create failed: "+strings.TrimSpace(out), true)
			return fmt.Errorf("create database: %w", err)
		}
	}
	env.add("Database aranea", checkOK, "ready", false)

	if err := ensurePgvector(env, log); err != nil {
		env.add("pgvector extension", checkWarn, err.Error()+"; vector search may be unavailable", false)
		env.VectorOK = false
		log("pgvector warn: %v", err)
	} else {
		env.VectorOK = true
		env.add("pgvector extension", checkOK, "CREATE EXTENSION vector ok", false)
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
		_ = writeFileUTF8BOM(filepath.Join(logDir, "initdb.log"), out)
		if err != nil {
			cmd = hiddenCmd(initdb, "-D", pgData, "-U", "postgres", "--auth=trust", "--encoding=UTF8")
			cmd.Dir = env.Root
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
			out, err = runWithTimeout(cmd, 120*time.Second)
			_ = writeFileUTF8BOM(filepath.Join(logDir, "initdb.log"), out)
			if err != nil {
				return fmt.Errorf("initdb failed: %v\n%s", err, trimOut([]byte(out)))
			}
		}
	} else if err := ensureBundledPGDataMajor(env, pgBin, pgData, log); err != nil {
		return err
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

	// IMPORTANT: do NOT use pg_ctl -w here. On Windows, pg_ctl -w with captured
	// stdout/stderr pipes often hangs forever even after the server is ready
	// (matches user logs: "starting bundled postgres" then silence).
	log("starting bundled postgres on :%s (no -w; poll readiness)", env.PGPort)
	pgctlLog := filepath.Join(logDir, "pgctl.log")
	outFile, err := os.OpenFile(pgctlLog, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("无法写 pgctl.log: %w", err)
	}
	cmd := hiddenCmd(pgCtl, "start", "-D", pgData,
		"-l", filepath.Join(logDir, "postgres.log"),
		"-o", "-p "+env.PGPort,
	)
	cmd.Dir = env.Root
	cmd.Stdout = outFile
	cmd.Stderr = outFile
	if err := cmd.Start(); err != nil {
		_ = outFile.Close()
		return fmt.Errorf("pg_ctl start 失败: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		_ = outFile.Close()
	}()
	select {
	case err := <-done:
		if err != nil && !tcpOpen(env.PGHost, env.PGPort, time.Second) {
			b, _ := os.ReadFile(pgctlLog)
			return fmt.Errorf("pg_ctl start 失败: %v\n%s", err, trimOut(b))
		}
		log("pg_ctl returned (port open=%v)", tcpOpen(env.PGHost, env.PGPort, 300*time.Millisecond))
	case <-time.After(12 * time.Second):
		// pg_ctl may still be attached; readiness is confirmed by waitPGReady.
		log("pg_ctl wait timed out; polling TCP/pg_isready instead")
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
		time.Sleep(200 * time.Millisecond)
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
		return fmt.Errorf("cannot enable pgvector: %s; %v", strings.TrimSpace(out), err2)
	}
	out, err = runPSQL(env, "aranea", "-c", "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("copied vector files but CREATE EXTENSION still failed: %s", trimOut([]byte(out)))
	}
	return nil
}

func installBundledVectorFiles(env *runtimeEnv, log func(string, ...any)) error {
	srcLib := filepath.Join(env.Root, "postgres", "lib", "vector.dll")
	if _, err := os.Stat(srcLib); err != nil {
		return fmt.Errorf("package missing postgres\\lib\\vector.dll (vector search will be degraded)")
	}
	pgRoot := filepath.Dir(env.PGBinDir)
	destLib := filepath.Join(pgRoot, "lib", "vector.dll")
	destShare := filepath.Join(pgRoot, "share", "extension")
	_ = os.MkdirAll(destShare, 0o755)

	log("copying vector.dll -> %s", destLib)
	if err := copyFile(srcLib, destLib); err != nil {
		return fmt.Errorf("copy vector.dll failed (system PostgreSQL may need admin rights): %w", err)
	}

	srcShare := filepath.Join(env.Root, "postgres", "share", "extension")
	matches, _ := filepath.Glob(filepath.Join(srcShare, "vector*"))
	for _, m := range matches {
		_ = copyFile(m, filepath.Join(destShare, filepath.Base(m)))
	}
	return nil
}

// ensureBundledPGDataMajor archives data if cluster major != postgres.exe major (e.g. PG17→PG18 upgrade).
func ensureBundledPGDataMajor(env *runtimeEnv, pgBin, pgData string, log func(string, ...any)) error {
	dataMajor := readPGMajorFile(filepath.Join(pgData, "PG_VERSION"))
	binMajor := postgresBinaryMajor(filepath.Join(pgBin, "postgres.exe"))
	if dataMajor == "" || binMajor == "" || dataMajor == binMajor {
		return nil
	}
	bak := pgData + ".pg" + dataMajor + "-" + time.Now().Format("20060102-150405")
	log("postgres major mismatch data=%s binary=%s; archiving data to %s and re-init", dataMajor, binMajor, bak)
	if err := os.Rename(pgData, bak); err != nil {
		return fmt.Errorf("archive old postgres data failed: %w", err)
	}
	initdb := filepath.Join(pgBin, "initdb.exe")
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
	_ = writeFileUTF8BOM(filepath.Join(env.Root, "logs", "initdb.log"), out)
	if err != nil {
		return fmt.Errorf("re-initdb after major upgrade failed: %v\n%s", err, trimOut([]byte(out)))
	}
	return nil
}

func readPGMajorFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if i := strings.IndexAny(s, ".\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func postgresBinaryMajor(postgresExe string) string {
	cmd := hiddenCmd(postgresExe, "--version")
	out, err := runWithTimeout(cmd, 5*time.Second)
	if err != nil {
		return ""
	}
	// e.g. "postgres (PostgreSQL) 18.3"
	fields := strings.Fields(out)
	for _, f := range fields {
		if len(f) > 0 && f[0] >= '1' && f[0] <= '9' {
			maj := f
			if i := strings.IndexByte(maj, '.'); i >= 0 {
				maj = maj[:i]
			}
			return maj
		}
	}
	return ""
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

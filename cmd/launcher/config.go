package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type modeFile struct {
	PGMode              string `json:"pg_mode"`
	PGPort              string `json:"pg_port"`
	RedisMode           string `json:"redis_mode"`
	StartedBundledRedis bool   `json:"started_bundled_redis"`
	StartedBundledPG    bool   `json:"started_bundled_pg"`
}

func writeModeFile(root string, env *runtimeEnv) error {
	m := modeFile{
		PGMode:              env.PGMode,
		PGPort:              env.PGPort,
		RedisMode:           env.RedisMode,
		StartedBundledRedis: env.RedisMode == "bundled",
		StartedBundledPG:    env.PGMode == "bundled",
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(root, "logs", "runtime-mode.json"), b, 0o644)
}

func readModeFile(root string) modeFile {
	b, err := os.ReadFile(filepath.Join(root, "logs", "runtime-mode.json"))
	if err != nil {
		return modeFile{StartedBundledRedis: true, StartedBundledPG: true}
	}
	var m modeFile
	if json.Unmarshal(b, &m) != nil {
		return modeFile{StartedBundledRedis: true, StartedBundledPG: true}
	}
	return m
}

func writeRuntimeConfig(env *runtimeEnv, log func(string, ...any)) error {
	cfgDir := filepath.Join(env.Root, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	dsn := env.postgresDSN("aranea")
	content := fmt.Sprintf(`server:
  http:
    addr: 0.0.0.0:8800
    timeout: 0s
  grpc:
    addr: 0.0.0.0:9900
    timeout: 120s
  ws:
    enable: true
    network: tcp
    addr: 0.0.0.0:8802
  monitor:
    process_log_enabled: true
data:
  driver: postgres
  sqlite:
    enable: false
    source: file:./data/arenea.sqlite?cache=shared&_fk=1
  initial_admin:
    name: admin
    email: admin@local.invalid
    password: changeme
    access: admin
  postgres:
    source: %q
    vector_dim: 1024
  redis:
    addr: %q
    read_timeout: 0.2s
    write_timeout: 0.2s
logging:
  level: info
  output_dir: "./logs"
  max_size_mb: 100
  max_backups: 10
  max_age_days: 30
  compress: true
  stdout_enabled: true
  hook_level: info
`, dsn, env.RedisAddr)

	path := filepath.Join(cfgDir, "config.yaml")
	if prev, err := os.ReadFile(path); err == nil && string(prev) == content {
		log("config unchanged (%s)", path)
		env.add("Runtime config", checkOK, fmt.Sprintf("postgres %s:%s (%s), redis %s", env.PGHost, env.PGPort, env.PGMode, env.RedisAddr), false)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	log("wrote %s (pg=%s:%s mode=%s redis=%s)", path, env.PGHost, env.PGPort, env.PGMode, env.RedisMode)
	env.add("Runtime config", checkOK, fmt.Sprintf("postgres %s:%s (%s), redis %s", env.PGHost, env.PGPort, env.PGMode, env.RedisAddr), false)
	return nil
}

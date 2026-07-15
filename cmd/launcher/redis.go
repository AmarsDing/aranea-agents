package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ensureRedis(env *runtimeEnv, log func(string, ...any)) error {
	if env.RedisMode == "system" || tcpOpen("127.0.0.1", "6379", 400*time.Millisecond) {
		log("redis available on :6379")
		env.add("Redis 就绪", checkOK, "127.0.0.1:6379", false)
		return nil
	}
	exe := filepath.Join(env.Root, "redis", "redis-server.exe")
	if _, err := os.Stat(exe); err != nil {
		env.add("Redis 就绪", checkFail, "缺少 redis-server.exe", true)
		return fmt.Errorf("缺少 Redis: %w", err)
	}
	log("starting bundled redis on :6379")
	cmd := hiddenCmd(exe, "--port", "6379", "--bind", "127.0.0.1")
	cmd.Dir = filepath.Join(env.Root, "redis")
	if err := cmd.Start(); err != nil {
		env.add("Redis 就绪", checkFail, err.Error(), true)
		return err
	}
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if tcpOpen("127.0.0.1", "6379", 300*time.Millisecond) {
			env.add("Redis 就绪", checkOK, "内置实例已启动", false)
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	env.add("Redis 就绪", checkFail, "启动后端口仍不可用", true)
	return fmt.Errorf("Redis 启动超时")
}

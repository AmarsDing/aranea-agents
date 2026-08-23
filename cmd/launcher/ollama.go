package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Ollama provides the default local embedding runtime (bge-m3, vector_dim
// 1024) for the knowledge subsystem. It is an optional dependency: core chat
// works without it, but vector search degrades. Never fatal in preflight.
const (
	ollamaHost         = "127.0.0.1"
	ollamaPort         = "11434"
	ollamaDefaultModel = "bge-m3"
)

func ollamaBaseURL() string {
	return "http://" + ollamaHost + ":" + ollamaPort
}

// ollamaBinary locates the ollama CLI: PATH first, then the per-user default
// install location used by OllamaSetup.exe.
func ollamaBinary() string {
	if p, err := execLookPath("ollama.exe"); err == nil {
		return p
	}
	if p, err := execLookPath("ollama"); err == nil {
		return p
	}
	if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
		cand := filepath.Join(lad, "Programs", "Ollama", "ollama.exe")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

func ollamaRunning() bool {
	return tcpOpen(ollamaHost, ollamaPort, 400*time.Millisecond)
}

// ollamaModels returns the locally available model names (GET /api/tags).
func ollamaModels(client *http.Client, baseURL string) ([]string, error) {
	resp, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/tags: status %d", resp.StatusCode)
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		if n := strings.TrimSpace(m.Name); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// ollamaModelPresent matches "bge-m3" against "bge-m3" or any tag of it
// ("bge-m3:latest"), but not against merely similar names ("bge", "bge-m4").
func ollamaModelPresent(models []string, want string) bool {
	for _, m := range models {
		if m == want || strings.HasPrefix(m, want+":") {
			return true
		}
	}
	return false
}

// ensureOllamaServer starts `ollama serve` hidden when the CLI is installed
// but the daemon port is closed, then polls readiness. Best-effort: callers
// treat a failure as WARN, never abort startup.
func ensureOllamaServer(bin string, log func(string, ...any)) error {
	if ollamaRunning() {
		return nil
	}
	log("starting ollama serve (hidden)")
	cmd := hiddenCmd(bin, "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ollama serve start: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ollamaRunning() {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("ollama serve did not open :%s within 10s", ollamaPort)
}

// runOllamaSetupStep is the wizard's Ollama section: detect → auto-start the
// daemon → offer to pull the default embedding model when missing.
func runOllamaSetupStep(p *prompter, out func(string), log func(string, ...any)) {
	out("Ollama embedding runtime (knowledge vector search):")
	bin := ollamaBinary()
	if bin == "" {
		out("  [SKIP] Ollama not installed — knowledge search will be degraded.")
		out("         Re-run the installer (offers Ollama) or install from https://ollama.com")
		return
	}
	if !ollamaRunning() {
		out("  Ollama installed, daemon not running; starting...")
		if err := ensureOllamaServer(bin, log); err != nil {
			out("  [WARN] auto-start failed: " + err.Error())
			return
		}
	}
	models, err := ollamaModels(&http.Client{Timeout: 3 * time.Second}, ollamaBaseURL())
	if err != nil {
		out("  [WARN] query models failed: " + err.Error())
		return
	}
	if ollamaModelPresent(models, ollamaDefaultModel) {
		out("  [OK] embedding model " + ollamaDefaultModel + " ready")
		return
	}
	out("  embedding model " + ollamaDefaultModel + " missing")
	if p.askYesNo("  Pull "+ollamaDefaultModel+" now? 现在下载嵌入模型 (~1.2GB)", true) {
		if err := pullOllamaModel(bin, ollamaDefaultModel, out, log); err != nil {
			out("  [WARN] pull failed: " + err.Error())
			out("         retry later: ollama pull " + ollamaDefaultModel)
		} else {
			out("  [OK] " + ollamaDefaultModel + " pulled")
		}
	} else {
		out("  -> skipped; run later: ollama pull " + ollamaDefaultModel)
	}
}

// checkOllama records Ollama runtime/model status as non-fatal check items.
// Never fatal: core chat works without it; only knowledge embedding degrades.
func (e *runtimeEnv) checkOllama() {
	bin := ollamaBinary()
	if bin == "" {
		e.add("Ollama", checkWarn, "not installed — knowledge embedding ("+ollamaDefaultModel+") unavailable; rerun the installer or install from https://ollama.com", false)
		return
	}
	if !ollamaRunning() {
		e.add("Ollama", checkWarn, "installed but daemon not running ("+bin+"); launcher auto-starts it on launch", false)
		return
	}
	models, err := ollamaModels(&http.Client{Timeout: 2 * time.Second}, ollamaBaseURL())
	if err != nil {
		e.add("Ollama", checkWarn, "running but /api/tags failed: "+err.Error(), false)
		return
	}
	if ollamaModelPresent(models, ollamaDefaultModel) {
		e.add("Ollama", checkOK, "running, embedding model "+ollamaDefaultModel+" ready", false)
	} else {
		e.add("Ollama embedding model", checkWarn, ollamaDefaultModel+" missing — run: ollama pull "+ollamaDefaultModel+" (or AraneaLauncher.exe -setup)", false)
	}
}

// pullOllamaModel runs `ollama pull <model>` streaming progress lines to out.
// Large download (bge-m3 ≈ 1.2GB) — generous 20-minute cap.
func pullOllamaModel(bin, model string, out func(string), log func(string, ...any)) error {
	log("ollama pull %s begin", model)
	cmd := hiddenCmd(bin, "pull", model)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ollama pull start: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && out != nil {
				out("  ollama> " + line)
			}
		}
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("ollama pull %s: %w", model, err)
		}
		log("ollama pull %s done", model)
		return nil
	case <-time.After(20 * time.Minute):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return fmt.Errorf("ollama pull %s timed out (20m)", model)
	}
}

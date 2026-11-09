package ceoboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/biz/ceoboard/store/storetest"
	"aranea-agents/internal/conf"
	loggateway "aranea-agents/pkg/loggateway"
)

// captureLogger 捕获 Warn 消息(告警路径单测用)。
type captureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *captureLogger) Debug(string, ...loggateway.Field) {}
func (l *captureLogger) Info(string, ...loggateway.Field)  {}
func (l *captureLogger) Error(string, ...loggateway.Field) {}
func (l *captureLogger) Warn(msg string, _ ...loggateway.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}
func (l *captureLogger) With(...loggateway.Field) loggateway.Logger { return l }

// newRuntimeArgs 给出最小可装配参数:空注册表目录 + 临时 spool/归档目录
// (NewRuntime 不启动周期循环,装配即返回)。
func newRuntimeArgs(t *testing.T, webhookKey string) (*conf.Ceoboard, *store.Store) {
	t.Helper()
	root := t.TempDir()
	regDir := filepath.Join(root, "workflows")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bc := &conf.Ceoboard{
		RegistryDir: regDir,
		SpoolDir:    filepath.Join(root, "spool"),
		ArchiveDir:  filepath.Join(root, "archive"),
		WebhookKey:  webhookKey,
	}
	client, _ := storetest.Open(t)
	return bc, store.New(client, nil)
}

// 评审 2026-11-09 M1(B2):webhook_key 为空必须在装配期打持久 Warn
// (no-auth 入口唯一闸门失效的可见性)。
func TestNewRuntimeWarnsOnEmptyWebhookKey(t *testing.T) {
	bc, st := newRuntimeArgs(t, "")
	lg := &captureLogger{}
	rt, err := NewRuntime(bc, st, lg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Stop()

	found := false
	for _, w := range lg.warns {
		if strings.Contains(w, "webhook_key") {
			found = true
		}
	}
	if !found {
		t.Fatalf("空 webhook_key 应打 Warn,得到 warns=%v", lg.warns)
	}
}

func TestNewRuntimeNoWarnWhenWebhookKeySet(t *testing.T) {
	bc, st := newRuntimeArgs(t, "secret")
	lg := &captureLogger{}
	rt, err := NewRuntime(bc, st, lg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Stop()

	for _, w := range lg.warns {
		if strings.Contains(w, "webhook") {
			t.Fatalf("已配置 webhook_key 不应打 webhook 告警,得到 %q", w)
		}
	}
}

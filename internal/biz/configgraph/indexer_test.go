package configgraph

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// captureLogger 记录 Info/Warn 消息（判据日志断言用）；With 派生共享底层状态。
type captureLogger struct {
	mu    *sync.Mutex
	infos *[]string
	warns *[]string
}

func newCaptureLogger() *captureLogger {
	return &captureLogger{mu: &sync.Mutex{}, infos: &[]string{}, warns: &[]string{}}
}

func (c *captureLogger) Debug(string, ...loggateway.Field) {}
func (c *captureLogger) Error(string, ...loggateway.Field) {}

func (c *captureLogger) Info(msg string, _ ...loggateway.Field) {
	c.mu.Lock()
	*c.infos = append(*c.infos, msg)
	c.mu.Unlock()
}

func (c *captureLogger) Warn(msg string, _ ...loggateway.Field) {
	c.mu.Lock()
	*c.warns = append(*c.warns, msg)
	c.mu.Unlock()
}

func (c *captureLogger) With(_ ...loggateway.Field) loggateway.Logger {
	return &captureLogger{mu: c.mu, infos: c.infos, warns: c.warns}
}

func (c *captureLogger) hasInfo(want string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range *c.infos {
		if m == want {
			return true
		}
	}
	return false
}

func TestIndexer_NilRebuilder(t *testing.T) {
	if NewIndexer(nil, nil) != nil {
		t.Fatal("nil rebuilder must yield nil indexer")
	}
	var i *Indexer
	if i.Rebuilder() != nil {
		t.Fatal("nil indexer must yield nil rebuilder")
	}
	i.Start(context.Background()) // 不应 panic
}

func TestIndexer_StartSeedsAndLogsAcceptanceLine(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	repo.maxGen = 5
	rb := NewRebuilder(src, repo, prov, nil, nil)
	lg := newCaptureLogger()
	idx := NewIndexer(rb, lg)
	if idx == nil || idx.Rebuilder() != rb {
		t.Fatalf("indexer wiring broken: %+v", idx)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		idx.Start(ctx)
		close(done)
	}()

	// Start 播种 + 判据日志后驻留；取消 ctx 必须退出。
	deadline := time.Now().Add(2 * time.Second)
	for !lg.hasInfo(LogIndexerStarted) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !lg.hasInfo(LogIndexerStarted) {
		t.Fatalf("acceptance log %q not emitted; infos=%v", LogIndexerStarted, *lg.infos)
	}
	if rb.Current() != 5 || !rb.Ready() {
		t.Fatalf("start must seed generation: current=%d ready=%v", rb.Current(), rb.Ready())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start must return after ctx cancel")
	}
}

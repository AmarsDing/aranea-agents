package tools

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func concurrencyTrackingTool(name string, delay time.Duration, active, maxActive *int32) trpctool.CallableTool {
	return &decoratorMockTool{
		name: name,
		call: func(ctx context.Context, args []byte) (any, error) {
			cur := atomic.AddInt32(active, 1)
			for {
				old := atomic.LoadInt32(maxActive)
				if cur <= old || atomic.CompareAndSwapInt32(maxActive, old, cur) {
					break
				}
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				atomic.AddInt32(active, -1)
				return nil, ctx.Err()
			}
			atomic.AddInt32(active, -1)
			return "ok", nil
		},
	}
}

func TestToolDecorator_ExclusiveHostexecFamilySerializes(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	execCmd := NewToolDecorator(
		concurrencyTrackingTool("exec_command", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	writeStdin := NewToolDecorator(
		concurrencyTrackingTool("write_stdin", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := execCmd.Call(context.Background(), nil); err != nil {
			t.Errorf("exec_command: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := writeStdin.Call(context.Background(), nil); err != nil {
			t.Errorf("write_stdin: %v", err)
		}
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("hostexec family max concurrency = %d, want 1", got)
	}
}

func TestToolDecorator_ConcurrentSafeFileReadsStayParallel(t *testing.T) {
	var active, maxActive int32
	const delay = 50 * time.Millisecond
	a := NewToolDecorator(
		concurrencyTrackingTool("read_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	b := NewToolDecorator(
		concurrencyTrackingTool("read_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := a.Call(context.Background(), nil); err != nil {
			t.Errorf("read_file a: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := b.Call(context.Background(), nil); err != nil {
			t.Errorf("read_file b: %v", err)
		}
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Errorf("read_file max concurrency = %d, want 2", got)
	}
	if elapsed >= 2*delay {
		t.Errorf("read_file calls serialized (%v), expected parallel < %v", elapsed, 2*delay)
	}
}

func TestExclusiveMutexKey(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"exec_command", "hostexec"},
		{"write_stdin", "hostexec"},
		{"kill_session", "hostexec"},
		{"shell_exec", "hostexec"},
		{"shell", "hostexec"},
		{"workspace_exec", "workspace_exec"},
		{"save_file", "file_write"},
		{"write_file", "file_write"},
		{"read_file", ""},
		{"list_file", ""},
		{"file", ""},
	}
	for _, tt := range tests {
		if got := ExclusiveMutexKey(tt.name); got != tt.want {
			t.Errorf("ExclusiveMutexKey(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestExclusiveLockKey_PerFilePath(t *testing.T) {
	a := exclusiveLockKey("save_file", []byte(`{"file_name":"src/a.go"}`))
	aAlias := exclusiveLockKey("write_file", []byte(`{"file_name":"src/a.go"}`))
	aEdit := exclusiveLockKey("diff_edit", []byte(`{"file_name":"src/a.go"}`))
	b := exclusiveLockKey("save_file", []byte(`{"file_name":"src/b.go"}`))
	missing := exclusiveLockKey("save_file", nil)

	if a != aAlias || a != aEdit {
		t.Fatalf("same target file should share lock, got save=%q alias=%q edit=%q", a, aAlias, aEdit)
	}
	if a == b {
		t.Fatalf("different files should not share lock, both %q", a)
	}
	if !strings.HasPrefix(a, "file_write:") {
		t.Fatalf("path lock should be file_write:<path>, got %q", a)
	}
	if missing != "file_write" {
		t.Fatalf("missing path should fall back to family lock, got %q", missing)
	}
}

func TestToolDecorator_FileWritesDifferentPathsParallel(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	a := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	b := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := a.Call(context.Background(), []byte(`{"file_name":"a.go"}`)); err != nil {
			t.Errorf("save_file a.go: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := b.Call(context.Background(), []byte(`{"file_name":"b.go"}`)); err != nil {
			t.Errorf("save_file b.go: %v", err)
		}
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Errorf("different-file writes max concurrency = %d, want 2", got)
	}
	if elapsed >= 2*delay {
		t.Errorf("different-file writes serialized (%v), expected parallel < %v", elapsed, 2*delay)
	}
}

func TestToolDecorator_FileWritesSamePathSerialize(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	save := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	edit := NewToolDecorator(
		concurrencyTrackingTool("diff_edit", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := save.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("save_file: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := edit.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("diff_edit: %v", err)
		}
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("same-file writes max concurrency = %d, want 1", got)
	}
}

func TestExclusiveLockKey_PathCaseInsensitive(t *testing.T) {
	upper := exclusiveLockKey("save_file", []byte(`{"file_name":"Src/A.go"}`))
	lower := exclusiveLockKey("save_file", []byte(`{"file_name":"src/a.go"}`))
	if upper != lower {
		t.Fatalf("case-insensitive lock identity: %q vs %q", upper, lower)
	}
}

func TestToolDecorator_ReadAndWriteSamePathSerialize(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	read := NewToolDecorator(
		concurrencyTrackingTool("read_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	write := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := read.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("read_file: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := write.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("save_file: %v", err)
		}
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("read+write same path max concurrency = %d, want 1", got)
	}
}

func TestToolDecorator_ListDirAndWriteChildSerialize(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	list := NewToolDecorator(
		concurrencyTrackingTool("list_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	write := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := list.Call(context.Background(), []byte(`{"path":"locktest-src"}`)); err != nil {
			t.Errorf("list_file: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := write.Call(context.Background(), []byte(`{"file_name":"locktest-src/a.go"}`)); err != nil {
			t.Errorf("save_file: %v", err)
		}
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("list+write child max concurrency = %d, want 1", got)
	}
}

func TestToolDecorator_ListUnrelatedDirAndWriteStayParallel(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	list := NewToolDecorator(
		concurrencyTrackingTool("list_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	write := NewToolDecorator(
		concurrencyTrackingTool("save_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := list.Call(context.Background(), []byte(`{"path":"locktest-pkg"}`)); err != nil {
			t.Errorf("list_file: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := write.Call(context.Background(), []byte(`{"file_name":"locktest-src/b.go"}`)); err != nil {
			t.Errorf("save_file: %v", err)
		}
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Errorf("unrelated list+write max concurrency = %d, want 2", got)
	}
	if elapsed >= 2*delay {
		t.Errorf("unrelated list+write serialized (%v), expected parallel < %v", elapsed, 2*delay)
	}
}

func TestToolDecorator_ReadsSamePathStayParallel(t *testing.T) {
	var active, maxActive int32
	const delay = 40 * time.Millisecond
	a := NewToolDecorator(
		concurrencyTrackingTool("read_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)
	b := NewToolDecorator(
		concurrencyTrackingTool("read_file", delay, &active, &maxActive),
		ToolDecoratorConfig{Logger: loggateway.NewNoop()},
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := a.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("read a: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := b.Call(context.Background(), []byte(`{"file_name":"same.go"}`)); err != nil {
			t.Errorf("read b: %v", err)
		}
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Errorf("shared reads max concurrency = %d, want 2", got)
	}
}

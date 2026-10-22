package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// --- cappedBuffer unit matrix (stderr/stdout retention caps) ---

func TestCappedBufferTruncation(t *testing.T) {
	tests := []struct {
		name      string
		limit     int64
		writes    []int // byte counts per Write call
		wantLen   int
		wantTrunc bool
	}{
		{name: "under limit", limit: 8, writes: []int{3, 4}, wantLen: 7, wantTrunc: false},
		{name: "exact fit", limit: 8, writes: []int{8}, wantLen: 8, wantTrunc: false},
		{name: "over by one", limit: 8, writes: []int{9}, wantLen: 8, wantTrunc: true},
		{name: "split across writes", limit: 8, writes: []int{6, 6}, wantLen: 8, wantTrunc: true},
		{name: "sustained flush", limit: 4, writes: []int{4, 4, 4, 4}, wantLen: 4, wantTrunc: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := &cappedBuffer{buf: &buf, limit: tc.limit}
			for i, n := range tc.writes {
				written, err := c.Write(make([]byte, n))
				if err != nil {
					t.Fatalf("write %d: %v", i, err)
				}
				if written != n {
					t.Fatalf("write %d: got %d, want %d (writes must never short-report)", i, written, n)
				}
			}
			if got := buf.Len(); got != tc.wantLen {
				t.Fatalf("retained=%d, want %d", got, tc.wantLen)
			}
			if c.truncated != tc.wantTrunc {
				t.Fatalf("truncated=%v, want %v", c.truncated, tc.wantTrunc)
			}
		})
	}
}

// --- Exec stderr cap, enforced through the real os/exec pipeline ---

// TestMain-style helper process: with GO_TEST_HELPER_PROCESS=1 the test binary
// re-execs itself and writes N bytes to stderr, then exits 0.
func TestDockerHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || len(args) < sep+3 {
		fmt.Fprintln(os.Stderr, "bad helper invocation")
		os.Exit(2)
	}
	mode := args[sep+1]
	n, _ := strconv.Atoi(args[sep+2])
	if mode == "stderr" {
		_, _ = os.Stderr.Write(make([]byte, n))
	}
	os.Exit(0)
}

func helperCmd(t *testing.T, ctx context.Context, mode string, n int64) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=TestDockerHelperProcess", "--", mode, strconv.FormatInt(n, 10))
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1")
	return cmd
}

func TestDockerEngineExecCapsStderr(t *testing.T) {
	tests := []struct {
		name           string
		stderrBytes    int64
		specLimit      int64 // 0 = default
		wantTruncated  bool
		wantMaxLen     int // includes tail note when truncated
		wantNote       bool
		wantTotalStderrFloor int
	}{
		{
			name:        "small stderr passes through",
			stderrBytes: 128, specLimit: 0,
			wantTruncated: false, wantMaxLen: 128, wantNote: false, wantTotalStderrFloor: 128,
		},
		{
			name:        "default cap truncates with tail note",
			stderrBytes: StderrLimitDefault + 4096, specLimit: 0,
			wantTruncated: true, wantMaxLen: StderrLimitDefault + 64, wantNote: true,
			wantTotalStderrFloor: StderrLimitDefault,
		},
		{
			name:        "explicit small limit respected",
			stderrBytes: 10_000, specLimit: 2048,
			wantTruncated: true, wantMaxLen: 2048 + 64, wantNote: true, wantTotalStderrFloor: 2048,
		},
		{
			name:        "exact fit does not truncate",
			stderrBytes: 512, specLimit: 512,
			wantTruncated: false, wantMaxLen: 512, wantNote: false, wantTotalStderrFloor: 512,
		},
		{
			name:        "negative limit keeps historical unlimited behavior",
			stderrBytes: StderrLimitDefault + 8192, specLimit: -1,
			wantTruncated: false,
			wantMaxLen:    StderrLimitDefault + 8192,
			wantNote:      false, wantTotalStderrFloor: StderrLimitDefault + 8192,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng := &DockerEngine{}
			nb := tc.stderrBytes
			eng.run = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				return helperCmd(t, ctx, "stderr", nb)
			}
			res, err := eng.Exec(context.Background(), Handle{ID: "c1", SandboxID: "c1"},
				ExecSpec{Argv: []string{"true"}, Timeout: 10 * time.Second, StderrLimit: tc.specLimit})
			if err != nil {
				t.Fatalf("Exec: %v", err)
			}
			if res.StderrTruncated != tc.wantTruncated {
				t.Fatalf("StderrTruncated=%v, want %v", res.StderrTruncated, tc.wantTruncated)
			}
			if len(res.Stderr) > tc.wantMaxLen {
				t.Fatalf("stderr retained %d bytes, want ≤ %d", len(res.Stderr), tc.wantMaxLen)
			}
			if len(res.Stderr) < tc.wantTotalStderrFloor-tc.wantMaxLen { // sanity floor
				t.Fatalf("stderr unexpectedly short: %d", len(res.Stderr))
			}
			hasNote := strings.Contains(res.Stderr, "[stderr truncated at ")
			if hasNote != tc.wantNote {
				t.Fatalf("tail note present=%v, want %v (stderr tail: %q)", hasNote, tc.wantNote, tailOf(res.Stderr, 48))
			}
			if tc.wantNote {
				wantLimit := tc.specLimit
				if wantLimit == 0 {
					wantLimit = StderrLimitDefault
				}
				if !strings.Contains(res.Stderr, fmt.Sprintf("[stderr truncated at %d bytes]", wantLimit)) {
					t.Fatalf("tail note missing limit value, stderr tail: %q", tailOf(res.Stderr, 48))
				}
			}
			// Enforcement honesty: engine-encrypted result is always "full" (isolated).
			if res.Enforcement != EnforcementFull {
				t.Fatalf("Enforcement=%q, want %q", res.Enforcement, EnforcementFull)
			}
		})
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

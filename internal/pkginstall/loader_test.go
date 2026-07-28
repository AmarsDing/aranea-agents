package pkginstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateManifestRejectsTraversalPaths(t *testing.T) {
	base := &Manifest{Version: 1, Metadata: ManifestMetadata{Name: "pkg"}}
	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "skill path",
			mutate: func(m *Manifest) {
				m.Spec.Skills = []SkillSpec{{Path: "../skill.zip"}}
			},
		},
		{
			name: "skill subpath",
			mutate: func(m *Manifest) {
				m.Spec.Skills = []SkillSpec{{URL: "https://example.invalid/repo.git", Subpath: "a/../../b"}}
			},
		},
		{
			name: "graph file",
			mutate: func(m *Manifest) {
				m.Spec.Graphs = []GraphSpec{{File: `..\secret.json`}}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := *base
			tc.mutate(&m)
			if err := ValidateManifest(&m); err == nil {
				t.Fatal("ValidateManifest() error = nil, want traversal rejection")
			}
		})
	}
}

func TestCloneArgsUsesDoubleDashBeforeURL(t *testing.T) {
	args := cloneArgs("--upload-pack=evil", "main", true, "/tmp/pkg")
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "\x00--\x00--upload-pack=evil\x00/tmp/pkg") {
		t.Fatalf("cloneArgs() = %#v, want -- separator before URL", args)
	}
}

func TestFetchFromURLReportsStderrTail(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	missing := filepath.Join(t.TempDir(), "no-such-repo")
	_, _, err := FetchFromURL(missing, "", true)
	if err == nil {
		t.Fatal("FetchFromURL() error = nil, want clone failure")
	}
	// Quiet mode must still surface git's stderr (e.g. "fatal: ..."),
	// not just "exit status 128".
	if !strings.Contains(err.Error(), "fatal") {
		t.Fatalf("FetchFromURL() error = %q, want git stderr tail", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("FetchFromURL() error = %q, want repo URL %q", err, missing)
	}
}

func TestCloneEnvInjectsGitProxy(t *testing.T) {
	t.Setenv("ARANEA_GIT_PROXY", "socks5://127.0.0.1:1080")
	// Pre-existing generic proxy must be overridden, not duplicated.
	t.Setenv("HTTPS_PROXY", "http://stale:1")
	t.Setenv("HTTP_PROXY", "http://stale:1")

	env := cloneEnv()
	counts := map[string]int{}
	vals := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		k = strings.ToUpper(k)
		counts[k]++
		vals[k] = v
	}
	for _, key := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		if vals[key] != "socks5://127.0.0.1:1080" {
			t.Fatalf("cloneEnv() %s = %q, want socks5://127.0.0.1:1080", key, vals[key])
		}
		if counts[key] != 1 {
			t.Fatalf("cloneEnv() %s appears %d times, want exactly 1", key, counts[key])
		}
	}
}

func TestCloneEnvWithoutGitProxyPassthrough(t *testing.T) {
	t.Setenv("ARANEA_GIT_PROXY", "")
	t.Setenv("HTTPS_PROXY", "http://keep:2")

	env := cloneEnv()
	found := false
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "HTTPS_PROXY=") {
			found = true
			if !strings.HasSuffix(kv, "=http://keep:2") {
				t.Fatalf("cloneEnv() HTTPS_PROXY = %q, want passthrough http://keep:2", kv)
			}
		}
	}
	if !found {
		t.Fatal("cloneEnv() dropped existing HTTPS_PROXY when ARANEA_GIT_PROXY unset")
	}
}

// installFakeGit writes a fake `git` executable onto PATH that fails the first
// failCount invocations and succeeds afterwards. It returns the path of the
// counter file recording how many times the fake git was invoked.
func installFakeGit(t *testing.T, failCount int) string {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "count.txt")
	t.Setenv("FAKE_GIT_COUNTER", counter)

	var name, script string
	if runtime.GOOS == "windows" {
		name = "git.bat"
		script = "@echo off\r\n" +
			"set /a COUNT=0\r\n" +
			"if exist \"%FAKE_GIT_COUNTER%\" set /p COUNT=<\"%FAKE_GIT_COUNTER%\"\r\n" +
			"set /a COUNT+=1\r\n" +
			">\"%FAKE_GIT_COUNTER%\" echo %COUNT%\r\n" +
			"if %COUNT% GTR " + itoa(failCount) + " goto ok\r\n" +
			"echo fatal: unable to access: simulated transient reset 1>&2\r\n" +
			"exit /b 128\r\n" +
			":ok\r\n" +
			"for %%I in (%*) do set \"TARGET=%%~I\"\r\n" +
			"mkdir \"%TARGET%\" 2>nul\r\n" +
			"exit /b 0\r\n"
	} else {
		name = "git"
		script = "#!/bin/sh\n" +
			"COUNT=0\n" +
			"[ -f \"$FAKE_GIT_COUNTER\" ] && COUNT=$(cat \"$FAKE_GIT_COUNTER\")\n" +
			"COUNT=$((COUNT+1))\n" +
			"echo $COUNT > \"$FAKE_GIT_COUNTER\"\n" +
			"if [ \"$COUNT\" -le " + itoa(failCount) + " ]; then\n" +
			"  echo \"fatal: unable to access: simulated transient reset\" >&2\n" +
			"  exit 128\n" +
			"fi\n" +
			"for last; do :; done\n" +
			"mkdir -p \"$last\"\n" +
			"exit 0\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Zero the retry backoff so tests stay fast.
	oldBackoff := cloneBackoff
	cloneBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { cloneBackoff = oldBackoff })
	return counter
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func readAttemptCount(t *testing.T, counter string) string {
	t.Helper()
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func TestFetchFromURLRetriesTransientCloneFailure(t *testing.T) {
	counter := installFakeGit(t, 1) // fail once, succeed on 2nd attempt
	dir, cleanup, err := FetchFromURL("https://example.invalid/repo.git", "", true)
	if err != nil {
		t.Fatalf("FetchFromURL() error = %v, want success after retry", err)
	}
	if cleanup == nil || dir == "" {
		t.Fatal("FetchFromURL() returned empty dir/cleanup on success")
	}
	cleanup()
	if got := readAttemptCount(t, counter); got != "2" {
		t.Fatalf("attempt count = %s, want 2 (1 failure + 1 retry)", got)
	}
}

func TestFetchFromURLGivesUpAfterMaxAttempts(t *testing.T) {
	counter := installFakeGit(t, 99) // always fail
	_, _, err := FetchFromURL("https://example.invalid/repo.git", "", true)
	if err == nil {
		t.Fatal("FetchFromURL() error = nil, want persistent failure")
	}
	if !strings.Contains(err.Error(), "simulated transient reset") {
		t.Fatalf("FetchFromURL() error = %q, want git stderr tail", err)
	}
	if got := readAttemptCount(t, counter); got != "3" {
		t.Fatalf("attempt count = %s, want 3 (max attempts)", got)
	}
}

func TestStderrTailTruncates(t *testing.T) {
	long := strings.Repeat("x", 500)
	if got := stderrTail(long, 200); len(got) != 200 {
		t.Fatalf("stderrTail() len = %d, want 200", len(got))
	}
	if got := stderrTail("  short\n", 200); got != "short" {
		t.Fatalf("stderrTail() = %q, want trimmed %q", got, "short")
	}
	if got := stderrTail("", 200); got != "" {
		t.Fatalf("stderrTail() = %q, want empty", got)
	}
}

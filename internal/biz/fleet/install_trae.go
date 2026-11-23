package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TRAE CLI 2.0（1.0 已进入维护模式，启动就提示升级）。官方安装脚本 install_v2.ps1 的在线安装用
// Windows PowerShell 的 Invoke-WebRequest 下载，本机实测比 curl 慢 25 倍、在脚本里干脆卡住（20 分钟 0 字节）；
// 所以照官方 offline-download 的格式由我们下载、逐个校验 SHA-256 与大小拼出离线包，再交给官方脚本
// offline-install 安装（安装、校验、目录布局仍由官方脚本做）。来源 2026-11-21 核对。
const (
	traeCDN      = "https://lf-cdn.trae.com.cn/obj/trae-com-cn/trae-cli"
	traeV2Script = "https://trae.cn/trae-cli/install_v2.ps1"
)

func installTrae(ctx context.Context, dir string, out io.Writer) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("TRAE CLI 一键安装目前只支持 Windows")
	}
	script, err := httpText(ctx, traeV2Script)
	if err != nil {
		return err
	}
	if !strings.Contains(script, "offline-install") || !strings.Contains(script, "TRAECLI_INSTALL_DIR") {
		return fmt.Errorf("TRAE 安装脚本格式变了，找不到离线安装或安装目录参数")
	}
	plat := map[string]string{"amd64": "windows-x86_64", "arm64": "windows-aarch64"}[runtime.GOARCH]
	// 离线包放在 CLI 根目录下、装成功才删：下载服务器慢或断流时，下次点更新从断点续传
	bundle := filepath.Join(filepath.Dir(dir), ".trae-bundle")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		return err
	}
	if err := buildTraeBundle(ctx, traeCDN, bundle, plat, script, out); err != nil {
		return fmt.Errorf("%w（已下载的部分保留在 %s，再点更新会续传）", err, bundle)
	}
	// 2.0 的脚本拒绝覆盖「不认识的文件」，而 1.0 的目录里只有三份同一个 exe：先把它整个挪开留作备份，
	// 装到干净目录；2.0 装失败就把半成品挪走、1.0 挪回来（只挪不删）
	bak, moved, err := moveAsideTraeV1(dir)
	if err != nil {
		return err
	}
	if moved {
		_, _ = fmt.Fprintf(out, "已把 TRAE 1.0 挪到 %s（备份，可手动删除）\n", bak)
	}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"$ProgressPreference='SilentlyContinue'; & "+q(filepath.Join(bundle, "install_v2.ps1"))+" offline-install "+q(bundle)+"; exit $LASTEXITCODE")
	cmd.Env = append(os.Environ(),
		// 状态目录放在安装目录外：放里面会被脚本自己当成「不认识的文件」
		"TRAECLI_INSTALL_DIR="+dir, "TRAECLI_STATE_DIR="+filepath.Join(filepath.Dir(dir), ".trae-state"),
		"TRAECLI_INSTALL_ASSUME_YES=1", "TRAECLI_INSTALL_SKIP_USER_PATH=1",
		"TRAECLI_INSTALL_AUTO_UPDATE=0", "TRAECLI_INSTALL_KEEP_EXISTING_COMMANDS=1")
	cmd.Stdout, cmd.Stderr = out, out
	runErr := cmd.Run()
	// 2.0 把安装目录做成指向状态目录 current 版本的链接，里面是 traecli.cmd → traex.exe（没有 traecli.exe）
	ok := runErr == nil && fileExists(filepath.Join(dir, "traecli.cmd"))
	if ok {
		_ = os.RemoveAll(bundle)
	}
	if moved && !ok {
		_ = os.Rename(dir, dir+".v2-failed-"+time.Now().Format("20060102-150405"))
		if os.Rename(bak, dir) == nil {
			_, _ = fmt.Fprintf(out, "2.0 没装成，已恢复 TRAE 1.0\n")
		}
	}
	return runErr
}

var (
	traeFileRe   = regexp.MustCompile(`^[0-9A-Za-z._-]+$`)
	traeRgPathRe = regexp.MustCompile(`^[0-9A-Za-z._/-]+/rg\.exe$`)
	traeSHARe    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

// buildTraeBundle 按官方 Export-OfflineBundle 的格式在 bundle 里放：artifacts/（主程序、sidecar、沙箱助手）、
// dependencies/ripgrep/（rg 包与它的清单）、offline-manifest.json（地址改成包内相对路径）、install_v2.ps1。
func buildTraeBundle(ctx context.Context, cdn, bundle, plat, script string, out io.Writer) error {
	var man map[string]any
	if err := fetchJSON(ctx, cdn+"/v2/public/stable/latest.json", &man); err != nil {
		return fmt.Errorf("取 TRAE 发布清单失败：%w", err)
	}
	var art map[string]any
	for _, a := range asList(man["artifacts"]) {
		if m, ok := a.(map[string]any); ok && m["platform"] == plat {
			art = m
		}
	}
	if art == nil {
		return fmt.Errorf("TRAE 发布清单里没有 %s 的安装包", plat)
	}
	_, _ = fmt.Fprintf(out, "TRAE CLI %v（%s）\n", man["version"], plat)
	save := func(rec map[string]any, folder, compression string) error {
		name, _ := rec["filename"].(string)
		url, _ := rec["url"].(string)
		sum, _ := rec["sha256"].(string)
		sn, _ := rec["size"].(json.Number)
		size, _ := sn.Int64()
		if !traeFileRe.MatchString(name) || strings.Contains(name, "..") || !strings.HasPrefix(url, cdn+"/") ||
			rec["compression"] != compression || !traeSHARe.MatchString(sum) || size <= 0 {
			return fmt.Errorf("TRAE 发布清单里的文件信息不对：%s", name)
		}
		target := filepath.Join(bundle, filepath.FromSlash(folder), name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := downloadVerified(ctx, url, target, size, sum, out); err != nil {
			return err
		}
		rec["url"] = folder + "/" + name
		return nil
	}
	if err := save(art, "artifacts", "gzip"); err != nil {
		return err
	}
	for _, key := range []string{"sidecars", "windows_sandbox_helpers"} {
		for _, x := range asList(art[key]) {
			if m, ok := x.(map[string]any); ok {
				if err := save(m, "artifacts", "gzip"); err != nil {
					return err
				}
			}
		}
	}
	var dep map[string]any
	if err := fetchJSON(ctx, cdn+"/v2/deps/ripgrep/latest.json", &dep); err != nil {
		return fmt.Errorf("取 ripgrep 依赖清单失败：%w", err)
	}
	var rg map[string]any
	for _, a := range asList(dep["artifacts"]) {
		if m, ok := a.(map[string]any); ok && m["platform"] == plat {
			rg = m
		}
	}
	p, _ := rg["archive_path"].(string)
	if rg == nil || rg["filename"] != "rg-"+plat+".zip" || !traeRgPathRe.MatchString(p) || strings.Contains(p, "..") ||
		!strings.HasPrefix(fmt.Sprint(rg["url"]), cdn+"/v2/deps/ripgrep/") {
		return fmt.Errorf("ripgrep 依赖清单不对")
	}
	if err := save(rg, "dependencies/ripgrep", "zip"); err != nil {
		return err
	}
	dep["artifacts"] = []any{rg}
	man["artifacts"] = []any{art}
	man["offline_channel"], man["release_channel"] = "public", "stable"
	for name, v := range map[string]any{"dependencies/ripgrep/manifest.json": dep, "offline-manifest.json": man} {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(bundle, filepath.FromSlash(name)), b, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(bundle, "install_v2.ps1"), []byte(script), 0o644)
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// fetchJSON 取一个 JSON（数字保留原样，回写清单时大小不变成科学计数）。
func fetchJSON(ctx context.Context, url string, v any) error {
	s, err := httpText(ctx, url)
	if err != nil {
		return err
	}
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	return d.Decode(v)
}

// stallTimeout：下载这么久一个字节都没收到就放弃（慢但在动的不算卡住）。
var stallTimeout = 3 * time.Minute

var errStalled = errors.New("download stalled")

// stallReader 每读到数据就把看门狗往后推。
type stallReader struct {
	r   io.Reader
	dog *time.Timer
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.dog.Reset(stallTimeout)
	}
	return n, err
}

// TRAE 的下载服务器单个连接只有几十 KB/s，多开几个连接总速度能到 700 KB/s 以上（2026-11-21 实测）：
// 按段并行下载，每段一个 .part 文件，断了下次从各段已下的位置续。
var (
	segSize    int64 = 8 << 20
	segWorkers       = 6
)

// downloadVerified 下载到 dst 并校验大小与 SHA-256：已有且校验通过的跳过；没下完的分段（dst.<校验值前缀>.<序号>.part）
// 用 Range 续传；连续 stallTimeout 没有任何进展就暂停（已下的部分留着下次续）。
func downloadVerified(ctx context.Context, url, dst string, size int64, sum string, out io.Writer) error {
	name := filepath.Base(dst)
	if fileMatches(dst, size, sum) {
		_, _ = fmt.Fprintf(out, "已下载过 %s，校验通过\n", name)
		return nil
	}
	_ = os.Remove(dst)
	prefix := dst + "." + strings.ToLower(sum[:12])
	nseg := int((size + segSize - 1) / segSize)
	segPath := func(i int) string { return fmt.Sprintf("%s.%d.part", prefix, i) }
	segLen := func(i int) int64 { return min(segSize, size-int64(i)*segSize) }
	var have int64
	for i := range nseg {
		if fi, err := os.Stat(segPath(i)); err == nil {
			if fi.Size() > segLen(i) {
				_ = os.Remove(segPath(i))
			} else {
				have += fi.Size()
			}
		}
	}
	if have > 0 {
		_, _ = fmt.Fprintf(out, "下载 %s（%.1f MB），已下 %.1f MB，续传\n", name, float64(size)/(1<<20), float64(have)/(1<<20))
	} else {
		_, _ = fmt.Fprintf(out, "下载 %s（%.1f MB）\n", name, float64(size)/(1<<20))
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	dog := time.AfterFunc(stallTimeout, func() { cancel(errStalled) })
	defer dog.Stop()
	sink := &lockedWriter{w: &progress{out: out, total: size, done: have, next: have}}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(segWorkers, nseg) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := fetchSegment(ctx, url, segPath(i), int64(i)*segSize, segLen(i), dog, sink); err != nil {
					cancel(err)
					return
				}
			}
		}()
	}
feed:
	for i := range nseg {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if err := context.Cause(ctx); err != nil {
		if errors.Is(err, errStalled) {
			return fmt.Errorf("%s 下载 %d 分钟没有进展，已暂停（已下的部分留着，下次续传）", name, int(stallTimeout.Minutes()))
		}
		return err
	}
	part := prefix + ".part"
	if err := joinSegments(part, nseg, segPath); err != nil {
		return err
	}
	if !fileMatches(part, size, sum) {
		for i := range nseg { // 坏了就从头来，不留着续
			_ = os.Remove(segPath(i))
		}
		_ = os.Remove(part)
		return fmt.Errorf("%s 校验不通过（大小或 SHA-256 不符）", name)
	}
	for i := range nseg {
		_ = os.Remove(segPath(i))
	}
	return os.Rename(part, dst)
}

// fetchSegment 把 [start, start+length) 下到 p，p 里已有的部分跳过。
func fetchSegment(ctx context.Context, url, p string, start, length int64, dog *time.Timer, sink io.Writer) error {
	var have int64
	if fi, err := os.Stat(p); err == nil {
		have = fi.Size()
	}
	if have >= length {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start+have, start+length-1))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("下载失败：HTTP %d（下载服务器要支持分段下载）", resp.StatusCode)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.TeeReader(io.LimitReader(&stallReader{r: resp.Body, dog: dog}, length-have), sink))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && have+n != length {
		err = io.ErrUnexpectedEOF
	}
	return err
}

func joinSegments(dst string, n int, segPath func(int) string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	for i := range n {
		var g *os.File
		if g, err = os.Open(segPath(i)); err != nil {
			break
		}
		_, err = io.Copy(f, g)
		_ = g.Close()
		if err != nil {
			break
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// fileMatches：文件存在且大小、SHA-256 都对。
func fileMatches(p string, size int64, sum string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return err == nil && n == size && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum)
}

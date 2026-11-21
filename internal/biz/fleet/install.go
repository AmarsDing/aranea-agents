package fleet

import (
	"archive/zip"
	"context"
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

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/safego"
)

// 一键安装：npm 包装到 <CLI 根>\npm，下载器装到 <CLI 根>\<id>。后台执行，同一工具同时只装一个。
// 不改系统 / 用户 PATH：查找命令和终端 PATH 由 cliDirs 补上。

const (
	InstallRunning = "running"
	InstallOK      = "ok"
	InstallFailed  = "failed"
)

type installJob struct {
	mu    sync.Mutex
	state string
	log   []byte
}

func (j *installJob) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.log = append(j.log, p...)
	if n := len(j.log); n > 16<<10 { // 只留最近 16KB
		j.log = j.log[n-16<<10:]
	}
	return len(p), nil
}

func (j *installJob) logf(format string, a ...any) { _, _ = fmt.Fprintf(j, format+"\n", a...) }

var (
	installMu   sync.Mutex
	installJobs = map[string]*installJob{}
)

// InstallStatus 返回某工具最近一次安装的状态与日志尾部；没装过返回空串。
func InstallStatus(id string) (state, log string) {
	installMu.Lock()
	j := installJobs[id]
	installMu.Unlock()
	if j == nil {
		return "", ""
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state, string(j.log)
}

// InstallAgent 在后台安装一个工具，立即返回；进度看 InstallStatus。
func (u *Usecase) InstallAgent(id string) error {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return apierror.BadRequest(domain, "unknown agent: "+id)
	}
	if p.Install == "" {
		return apierror.FailedPrecondition(domain, p.Label+" 没有登记安装方式，请按官方文档手动安装")
	}
	root := CLIRoot()
	if root == "" {
		return apierror.FailedPrecondition(domain, "未配置 ARANEA_FLEET_CLI_ROOT")
	}
	installMu.Lock()
	if j := installJobs[id]; j != nil && j.state == InstallRunning {
		installMu.Unlock()
		return apierror.FailedPrecondition(domain, p.Label+" 正在安装")
	}
	j := &installJob{state: InstallRunning}
	installJobs[id] = j
	installMu.Unlock()

	safego.Go(u.ctx, "fleet-install-"+id, func() {
		ctx, cancel := context.WithTimeout(u.ctx, 20*time.Minute)
		defer cancel()
		j.logf("安装 %s（%s）到 %s", p.Label, p.Install, root)
		err := runInstall(ctx, p, root, j)
		state := InstallOK
		if err == nil {
			if path, lerr := lookCommand(p.Command); lerr != nil {
				err = fmt.Errorf("安装完成但找不到命令 %s", p.Command)
			} else {
				j.logf("完成：%s", path)
			}
		}
		if err != nil {
			state = InstallFailed
			j.logf("失败：%v", err)
		}
		j.mu.Lock()
		j.state = state
		j.mu.Unlock()
	})
	return nil
}

func runInstall(ctx context.Context, p AgentPreset, root string, out io.Writer) error {
	if pkg, ok := strings.CutPrefix(p.Install, "npm:"); ok {
		return npmInstall(ctx, pkg, filepath.Join(root, "npm"), out)
	}
	fn := archiveInstallers[p.Install]
	if fn == nil {
		return fmt.Errorf("未知安装方式 %q", p.Install)
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("%s 的下载器只支持 Windows，其他系统请用官方安装脚本", p.Label)
	}
	return fn(ctx, filepath.Join(root, p.ID), out)
}

// npmInstall 直接用 node 跑 npm-cli.js：npm.cmd 经 cmd.exe 时，带空格的路径
// （C:\Program Files\nodejs、D:\Program Files\CLI）引号会被 cmd 剥掉。
func npmInstall(ctx context.Context, pkg, prefix string, out io.Writer) error {
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("找不到 node（npm 包需要 Node.js 18+）")
	}
	npmCLI := filepath.Join(filepath.Dir(node), "node_modules", "npm", "bin", "npm-cli.js")
	if _, err := os.Stat(npmCLI); err != nil {
		return fmt.Errorf("找不到 npm：%s", npmCLI)
	}
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return err
	}
	// 下载缓存也放进 CLI 根目录，不占 C 盘；http 级日志当进度看
	cache := filepath.Join(filepath.Dir(prefix), ".npm-cache")
	cmd := exec.CommandContext(ctx, node, npmCLI, "install", "-g", "--prefix", prefix, "--cache", cache, "--no-fund", "--no-audit", "--loglevel=http", pkg+"@latest")
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

// archiveInstallers 是不走 npm 的官方安装来源。两家官方脚本都写死装到 %LOCALAPPDATA% 并改用户 PATH，
// 所以只照搬脚本里的下载地址与目录布局，装到 dir。
var archiveInstallers = map[string]func(ctx context.Context, dir string, out io.Writer) error{
	"trae":   installTrae,
	"cursor": installCursor,
}

// 来源：https://lf-cdn.trae.com.cn/obj/trae-com-cn/trae-cli/install.ps1（2026-11-20 核对）
const traeCDN = "https://lf-cdn.trae.com.cn/obj/trae-com-cn/trae-cli/"

func installTrae(ctx context.Context, dir string, out io.Writer) error {
	ver, err := httpText(ctx, traeCDN+"trae-cli_latest_version.txt")
	if err != nil {
		return err
	}
	ver = strings.TrimPrefix(strings.TrimSpace(ver), "v")
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	url := fmt.Sprintf("%strae-cli_%s_windows_%s.zip", traeCDN, ver, arch)
	stage := dir + ".new"
	if err := downloadUnzip(ctx, url, stage, out); err != nil {
		return err
	}
	exe := filepath.Join(stage, "trae-cli.exe")
	for _, alias := range []string{"traecli.exe", "trae-agent.exe"} { // 与官方脚本相同的别名
		if err := copyFile(exe, filepath.Join(stage, alias)); err != nil {
			return err
		}
	}
	return swapDir(stage, dir)
}

// 来源：https://cursor.com/install?win32=true（脚本首行给出版本包前缀）
var cursorScriptURL = "https://cursor.com/install?win32=true"

var (
	cursorURLRe = regexp.MustCompile(`\$downloadUrl\s*=\s*'([^']+)'`)
	cursorVerRe = regexp.MustCompile(`\$version\s*=\s*'([0-9A-Za-z][0-9A-Za-z._-]*)'`)
)

func installCursor(ctx context.Context, dir string, out io.Writer) error {
	script, err := httpText(ctx, cursorScriptURL)
	if err != nil {
		return err
	}
	um, vm := cursorURLRe.FindStringSubmatch(script), cursorVerRe.FindStringSubmatch(script)
	if um == nil || vm == nil || !strings.HasPrefix(um[1], "https://downloads.cursor.com/") {
		return fmt.Errorf("Cursor 安装脚本格式变了，找不到版本包地址")
	}
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	tmp := dir + ".download"
	if err := downloadUnzip(ctx, um[1]+"windows/"+arch+"/agent-cli-package.zip", tmp, out); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// 官方布局：<根>\versions\<版本>\ 放包内容，<根>\cursor-agent.* 是从版本目录复制出来的启动器。
	verDir := filepath.Join(dir, "versions", vm[1])
	if err := os.MkdirAll(filepath.Dir(verDir), 0o755); err != nil {
		return err
	}
	if err := swapDir(filepath.Join(tmp, "dist-package"), verDir); err != nil {
		return err
	}
	launchers, _ := filepath.Glob(filepath.Join(verDir, "cursor-agent*"))
	if len(launchers) == 0 {
		return fmt.Errorf("版本包里没有 cursor-agent 启动器")
	}
	for _, f := range launchers {
		if err := copyFile(f, filepath.Join(dir, filepath.Base(f))); err != nil {
			return err
		}
	}
	return nil
}

func httpText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s：HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

// downloadUnzip 把 zip 下载到临时文件再解压到 dst（dst 先清空）。
func downloadUnzip(ctx context.Context, url, dst string, out io.Writer) error {
	fmt.Fprintf(out, "下载 %s\n", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "aranea-cli-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, err := io.Copy(f, io.TeeReader(resp.Body, &progress{out: out, total: resp.ContentLength}))
	f.Close()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "已下载 %.1f MB，解压到 %s\n", float64(n)/(1<<20), dst)
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return unzip(f.Name(), dst)
}

// progress 每下载 10MB 往安装日志写一行，免得慢网下界面看着像卡住。
type progress struct {
	out         io.Writer
	total, done int64
	next        int64
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.done >= p.next {
		if p.total > 0 {
			fmt.Fprintf(p.out, "  %.0f / %.0f MB\n", float64(p.done)/(1<<20), float64(p.total)/(1<<20))
		} else {
			fmt.Fprintf(p.out, "  %.0f MB\n", float64(p.done)/(1<<20))
		}
		p.next = p.done + 10<<20
	}
	return len(b), nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, zf := range r.File {
		target := filepath.Join(dst, zf.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("压缩包里有越界路径：%s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(zf, target); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(zf *zip.File, target string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, zf.Mode()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, rc); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// swapDir 用 src 替换 dst：旧目录先挪到 .old 再删（在跑的 exe 挪不走时报错，旧版保持可用）。
func swapDir(src, dst string) error {
	old := dst + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return fmt.Errorf("旧版本正在使用，先关掉用它的终端再装：%w", err)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

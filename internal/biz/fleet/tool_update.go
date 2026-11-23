package fleet

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 检查更新：远端最新版本（npm 仓库 / Cursor 安装脚本 / TRAE 发布清单）对比本机 --version。
// 每天自动查一次，卡片据此显示「可更新」；点「更新」时没查过或已是最新就先查一次再决定。

var versionNumRe = regexp.MustCompile(`\d+(?:\.\d+)+(?:-[0-9A-Za-z.]+)?`)

// versionCore 从 --version 输出里取版本号（"2.1.330 (Claude Code)" → "2.1.330"）。
func versionCore(s string) string { return versionNumRe.FindString(s) }

// NewerVersion 给服务层判「可更新」。
func NewerVersion(local, remote string) bool { return newerVersion(local, remote) }

// newerVersion：remote 比 local 新？逐段比数字；数字都一样但后缀不同（如 Cursor 的提交号）也算有更新。
func newerVersion(local, remote string) bool {
	l, r := versionCore(local), versionCore(remote)
	if l == "" || r == "" || l == r {
		return false
	}
	sep := func(c rune) bool { return c == '.' || c == '-' }
	lp, rp := strings.FieldsFunc(l, sep), strings.FieldsFunc(r, sep)
	for i := 0; i < max(len(lp), len(rp)); i++ {
		a, b := "", ""
		if i < len(lp) {
			a = lp[i]
		}
		if i < len(rp) {
			b = rp[i]
		}
		an, aerr := strconv.Atoi(a)
		bn, berr := strconv.Atoi(b)
		switch {
		case aerr == nil && berr == nil && an != bn:
			return bn > an
		case aerr == nil && berr == nil:
			continue
		case a != b:
			return true
		}
	}
	return false
}

// CheckToolUpdate 查一次远端最新版本，记进工具设置（本机版本没记过的顺带补上）。
func (u *Usecase) CheckToolUpdate(ctx context.Context, id string) error {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return apierror.BadRequest(domain, "unknown agent: "+id)
	}
	if p.Install == "" {
		return apierror.FailedPrecondition(domain, p.Label+" 不是一键安装的，查不了更新")
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	remote, err := remoteVersion(cctx, p)
	if err != nil {
		return apierror.FailedPrecondition(domain, "查不到 "+p.Label+" 的最新版本："+err.Error())
	}
	set := u.GetAgentSetting(ctx, id)
	if p.Available && !versionRe.MatchString(set.Version) {
		set.Version = u.toolVersion(p)
	}
	set.LatestVersion, set.LatestCheckedAt = remote, nowString()
	return putDoc(ctx, u.store, kindAgentSetting, id, set)
}

// remoteVersion 是安装来源上的最新版本。
func remoteVersion(ctx context.Context, p AgentPreset) (string, error) {
	if pkgs, ok := strings.CutPrefix(p.Install, "npm:"); ok {
		return npmLatest(ctx, strings.Fields(pkgs)[0])
	}
	switch p.Install {
	case "cursor":
		script, err := httpText(ctx, cursorScriptURL)
		if err != nil {
			return "", err
		}
		if m := cursorVerRe.FindStringSubmatch(script); m != nil {
			return m[1], nil
		}
		return "", fmt.Errorf("Cursor 安装脚本里找不到版本号")
	case "trae":
		var man map[string]any
		if err := fetchJSON(ctx, traeCDN+"/v2/public/stable/latest.json", &man); err != nil {
			return "", err
		}
		if v, ok := man["version"].(string); ok && v != "" {
			return v, nil
		}
		return "", fmt.Errorf("TRAE 发布清单里没有版本号")
	}
	return "", fmt.Errorf("未知安装方式 %q", p.Install)
}

// npmLatest 用 npm view 查 npm 仓库上的最新版本（同 npmInstall，直接跑 npm-cli.js）。
func npmLatest(ctx context.Context, pkg string) (string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("找不到 node")
	}
	npmCLI := filepath.Join(filepath.Dir(node), "node_modules", "npm", "bin", "npm-cli.js")
	args := []string{npmCLI, "view", pkg, "version"}
	if root := CLIRoot(); root != "" {
		args = append(args, "--cache", filepath.Join(root, ".npm-cache"))
	}
	cmd := exec.CommandContext(ctx, node, args...)
	cmd.Env = os.Environ()
	b, err := cmd.Output()
	v := strings.TrimSpace(string(b))
	if err != nil || !versionNumRe.MatchString(v) {
		return "", fmt.Errorf("npm view %s 失败：%v %s", pkg, err, v)
	}
	return v, nil
}

// checkUpdatesDue 给满 24 小时没查过的一键安装工具查一次更新（启动时与每小时一次）。
func (u *Usecase) checkUpdatesDue() {
	for _, p := range ListPresets() {
		if p.ID == "shell" || !p.Available || p.Install == "" || u.ctx.Err() != nil {
			continue
		}
		if set := u.GetAgentSetting(u.ctx, p.ID); updateDue(set.LatestCheckedAt, time.Now()) {
			if err := u.CheckToolUpdate(u.ctx, p.ID); err != nil {
				u.lg.Warn("fleet update check failed", loggateway.Str("agent", p.ID), loggateway.Err(err))
			}
		}
	}
}

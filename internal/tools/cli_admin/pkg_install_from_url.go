package cli_admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"aranea-agents/internal/pkginstall"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type pkgInstallInput struct {
	URL      string `json:"url" jsonschema:"description=aranea package 的 Git 仓库 URL,required"`
	Ref      string `json:"ref" jsonschema:"description=分支或 Tag"`
	Decision string `json:"decision" jsonschema:"description=Conflict resolution strategy: skip (keep existing, skip new), keep (overwrite existing with new), refine (merge new into existing)"`
	DryRun   bool   `json:"dry_run" jsonschema:"description=仅预览不安装"`
}

type pkgInstallOutput struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
	Steps   []string `json:"steps"`
}

// newPkgInstallFromURLTool creates the cli_admin_pkg_install_from_url tool.
// This is the key tool that enables AI to install complete packages via URL.
func newPkgInstallFromURLTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input pkgInstallInput) (pkgInstallOutput, error) {
		if input.URL == "" {
			return pkgInstallOutput{}, fmt.Errorf("url is required")
		}
		if err := validateRepoURL(input.URL); err != nil {
			return pkgInstallOutput{}, fmt.Errorf("invalid url: %w", err)
		}

		pkgDir, cleanup, err := pkginstall.FetchFromURL(input.URL, input.Ref, true)
		if err != nil {
			return pkgInstallOutput{}, fmt.Errorf("fetch package: %w", err)
		}
		defer cleanup()

		// Load and validate manifest.
		manifest, err := pkginstall.LoadManifestFromDir(pkgDir)
		if err != nil {
			return pkgInstallOutput{}, fmt.Errorf("load manifest: %w", err)
		}
		if err := pkginstall.ValidateManifest(manifest); err != nil {
			return pkgInstallOutput{}, fmt.Errorf("invalid manifest: %w", err)
		}

		// Apply decision to skills.
		if input.Decision != "" {
			for i := range manifest.Spec.Skills {
				if manifest.Spec.Skills[i].Decision == "" {
					manifest.Spec.Skills[i].Decision = input.Decision
				}
			}
		}

		var stepLog []string
		ins := &pkginstall.Installer{
			APIURL: deps.APIBaseURL,
			Token:  deps.APIToken,
			DryRun: input.DryRun,
			Quiet:  true,
			OnStep: func(step, total int, name, status string) {
				stepLog = append(stepLog, fmt.Sprintf("[%d/%d] %s: %s", step, total, name, status))
			},
		}

		result, err := ins.Install(pkgDir, manifest)
		if err != nil {
			return pkgInstallOutput{}, err
		}

		// Also append per-step results.
		for _, sr := range result.Steps {
			b, _ := json.Marshal(sr)
			stepLog = append(stepLog, string(b))
		}

		return pkgInstallOutput{
			Created: result.Created,
			Updated: result.Updated,
			Skipped: result.Skipped,
			Errors:  result.Errors,
			Steps:   stepLog,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_pkg_install_from_url"),
		function.WithDescription("从 Git 仓库 URL 安装完整的 aranea package（含 MCP 服务器/Skill/Agent/Team/Graph）。使用 decision 参数控制冲突策略。设置 dry_run=true 可预览安装步骤而不实际执行。"),
	)
}

func validateRepoURL(raw string) error {
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return nil
	}
	// Windows drive letter: C:/path or C:\path
	if len(raw) >= 2 && raw[1] == ':' && isAlpha(rune(raw[0])) {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https", "http":
		host := u.Hostname()
		if host == "" {
			return fmt.Errorf("host is empty")
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("lookup host %q: %w", host, err)
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("host %q resolves to private/internal IP %s; not allowed", host, ip)
			}
		}
		return nil
	case "file":
		return nil
	case "":
		// No scheme: treat as SCP-style Git URL (e.g. github.com/user/repo).
		// Parse the first path segment as the host and validate it.
		host := u.Path
		if slashIdx := strings.Index(host, "/"); slashIdx > 0 {
			host = host[:slashIdx]
		}
		if host == "" {
			return fmt.Errorf("cannot determine host from URL %q", raw)
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("lookup host %q: %w", host, err)
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("host %q resolves to private/internal IP %s; not allowed", host, ip)
			}
		}
		return nil
	default:
		return fmt.Errorf("scheme %q is not allowed; only https, http, and file are permitted", u.Scheme)
	}
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isPrivateIP(ip net.IP) bool {
	privateNets := []net.IPNet{
		{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
		{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
		{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)},
		{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},
		{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return ip.IsUnspecified()
}

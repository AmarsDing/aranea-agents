package cli_admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"aranea-agents/internal/pkginstall"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/outboundguard"
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
			return pkgInstallOutput{}, apierror.BadRequest(apierror.DomainTool, "url is required")
		}
		if err := validateRepoURL(input.URL); err != nil {
			return pkgInstallOutput{}, err
		}

		pkgDir, cleanup, err := pkginstall.FetchFromURL(input.URL, input.Ref, true)
		if err != nil {
			return pkgInstallOutput{}, apierror.Internal(apierror.DomainTool, "fetch package: %v", err)
		}
		defer cleanup()

		// Load and validate manifest.
		manifest, err := pkginstall.LoadManifestFromDir(pkgDir)
		if err != nil {
			return pkgInstallOutput{}, apierror.Internal(apierror.DomainTool, "load manifest: %v", err)
		}
		if err := pkginstall.ValidateManifest(manifest); err != nil {
			return pkgInstallOutput{}, apierror.BadRequest(apierror.DomainTool, "invalid manifest: %v", err)
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
			return pkgInstallOutput{}, apierror.Internal(apierror.DomainTool, "install package: %v", err)
		}

		// Also append per-step results.
		for _, sr := range result.Steps {
			b, marshalErr := json.Marshal(sr)
			if marshalErr != nil {
				stepLog = append(stepLog, fmt.Sprintf("step marshal error: %v", marshalErr))
				continue
			}
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
	// Reject local paths: absolute, relative, and Windows drive letters.
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return apierror.BadRequest(apierror.DomainTool, "local paths are not allowed")
	}
	// Windows drive letter: C:/path or C:\path
	if len(raw) >= 2 && raw[1] == ':' && isAlpha(rune(raw[0])) {
		return apierror.BadRequest(apierror.DomainTool, "local paths are not allowed")
	}
	// Reject paths containing ".." components (path traversal).
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return apierror.BadRequest(apierror.DomainTool, "path traversal (..) is not allowed")
		}
	}
	for _, seg := range strings.Split(raw, `\`) {
		if seg == ".." {
			return apierror.BadRequest(apierror.DomainTool, "path traversal (..) is not allowed")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return apierror.BadRequest(apierror.DomainTool, "parse url: %v", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		host := u.Hostname()
		if host == "" {
			return apierror.BadRequest(apierror.DomainTool, "host is empty")
		}
		if err := outboundguard.ValidatePublicHost(host); err != nil {
			return apierror.BadRequest(apierror.DomainTool, "host %q: %v", host, err)
		}
		return nil
	case "http":
		return apierror.BadRequest(apierror.DomainTool, "http scheme is not allowed; use https instead")
	case "file":
		return apierror.BadRequest(apierror.DomainTool, "file scheme is not allowed")
	case "":
		// No scheme: treat as SCP-style Git URL (e.g. github.com/user/repo).
		// Parse the first path segment as the host and validate it.
		host := u.Path
		if slashIdx := strings.Index(host, "/"); slashIdx > 0 {
			host = host[:slashIdx]
		}
		if host == "" {
			return apierror.BadRequest(apierror.DomainTool, "cannot determine host from URL %q", raw)
		}
		if err := outboundguard.ValidatePublicHost(host); err != nil {
			return apierror.BadRequest(apierror.DomainTool, "host %q: %v", host, err)
		}
		return nil
	default:
		return apierror.BadRequest(apierror.DomainTool, "scheme %q is not allowed; only https and SCP-style Git URLs are permitted", u.Scheme)
	}
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

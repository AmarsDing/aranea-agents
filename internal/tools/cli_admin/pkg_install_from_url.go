package cli_admin

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/pkginstall"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type pkgInstallInput struct {
	URL      string `json:"url" jsonschema:"description=aranea package 的 Git 仓库 URL,required"`
	Ref      string `json:"ref" jsonschema:"description=分支或 Tag"`
	Decision string `json:"decision" jsonschema:"description=冲突策略：skip|keep|refine"`
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

		// Clone the package repository.
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
		function.WithDescription("从 Git 仓库 URL 安装完整的 aranea package（含 MCP 服务器/Skill/Agent/Team/Graph）。"),
	)
}

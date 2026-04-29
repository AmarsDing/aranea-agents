package skill

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
	"arenea/backend/internal/domain"
)

// newInstallCmd 实现 `aranea skill install <url>`。支持 git 仓库
//（github.com / gitlab / 泛型 .git URL）与远程 zip。流程遵循 前端/25 cli.md §6：
//
//  1. 将 URL 解析为源描述（scheme + ref + subpath）
//  2. clone 或下载到临时目录
//  3. 定位 SKILL.md 根（单技能或可挑选子目录）
//  4. 做少量本地预检（要求 front-matter）
//  5. 将选定目录打 zip 并 POST 到 /api/v1/skills/import
//  6. 轮询返回的任务直到离开 `validating` 阶段
//  7. 交互式解决冲突（skip / keep / refine）
//  8. POST /apply 提交保留的候选
func newInstallCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		ref          string
		subpath      string
		dryRun       bool
		conflictMode string
		keep         string
	)
	cmd := &cobra.Command{
		Use:   "install <url>",
		Short: "Install a skill from a git URL or remote zip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := parseSource(args[0], ref, subpath)
			if err != nil {
				return err
			}
			tmp, err := os.MkdirTemp("", "aranea-skill-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)

			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "fetching %s ...\n", src.URL)
			}
			workdir, err := fetchSource(cmd.Context(), src, tmp)
			if err != nil {
				return err
			}
			skillRoot, err := locateSkillRoot(workdir, src.Subpath)
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "skill root: %s\n", skillRoot)
			}
			if err := preValidate(skillRoot); err != nil {
				return fmt.Errorf("local validation failed: %w", err)
			}
			if dryRun {
				output.Success(cmd.OutOrStdout(), "dry-run ok: skill is well-formed")
				return nil
			}
			zipBuf, err := zipDir(skillRoot)
			if err != nil {
				return err
			}
			job, err := uploadAndWait(cmd.Context(), g, zipBuf, filepath.Base(skillRoot)+".zip")
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "import job %s: %s (%d candidates, %d conflicts)\n",
					job.JobID, job.ValidationStatus, len(job.Candidates), len(job.ConflictGroups))
			}
			decisions, err := resolveConflicts(cmd, g, job, conflictMode, keep)
			if err != nil {
				return err
			}
			result, err := applyImport(cmd.Context(), g, job.JobID, decisions)
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "created skills: %v\n", result.CreatedSkillIDs)
				if len(result.SkippedCandidateIDs) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "skipped: %v\n", result.SkippedCandidateIDs)
				}
			}
			output.Success(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to checkout (branch / tag / commit)")
	cmd.Flags().StringVar(&subpath, "subpath", "", "Subdirectory inside the repo containing SKILL.md")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run local validation only; do not upload")
	cmd.Flags().StringVar(&conflictMode, "on-conflict", "ask", "ask|skip|keep|refine — default behavior when a conflict is detected")
	cmd.Flags().StringVar(&keep, "keep", "incoming", "incoming|existing — value used when --on-conflict=keep")
	return cmd
}

func newImportCmd(g *apiclient.GlobalContext) *cobra.Command {
	var conflictMode, keep string
	cmd := &cobra.Command{
		Use:   "import <zip-path>",
		Short: "Import a skill from a local zip file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			job, err := uploadAndWait(cmd.Context(), g, f, filepath.Base(args[0]))
			if err != nil {
				return err
			}
			decisions, err := resolveConflicts(cmd, g, job, conflictMode, keep)
			if err != nil {
				return err
			}
			result, err := applyImport(cmd.Context(), g, job.JobID, decisions)
			if err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&conflictMode, "on-conflict", "ask", "ask|skip|keep|refine")
	cmd.Flags().StringVar(&keep, "keep", "incoming", "incoming|existing")
	return cmd
}

// source 描述归一化后的技能来源。
type source struct {
	Kind    string // git | zip | local
	URL     string
	Ref     string
	Subpath string
}

// parseSource 将常见 URL 形态规范为内部表示：
//
//   - github.com/<owner>/<repo>            → git, https://github.com/<owner>/<repo>.git
//   - github.com/<owner>/<repo>/tree/<ref>/<sub> → git + ref + subpath
//   - https://...                          → 按后缀判为 git 或 zip
//   - file:// 或本地路径                   → local
func parseSource(rawURL, refOverride, subpathOverride string) (source, error) {
	if !strings.Contains(rawURL, "://") && !strings.HasPrefix(rawURL, "github.com") &&
		!strings.HasPrefix(rawURL, "gitlab.com") {
		// 视为本地路径
		abs, err := filepath.Abs(rawURL)
		if err != nil {
			return source{}, err
		}
		return source{Kind: "local", URL: abs, Subpath: subpathOverride}, nil
	}
	if strings.HasPrefix(rawURL, "github.com/") || strings.HasPrefix(rawURL, "gitlab.com/") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return source{}, fmt.Errorf("invalid URL: %w", err)
	}
	if strings.HasSuffix(u.Path, ".zip") {
		return source{Kind: "zip", URL: u.String(), Subpath: subpathOverride}, nil
	}
	src := source{Kind: "git", Ref: refOverride, Subpath: subpathOverride}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if (u.Host == "github.com" || u.Host == "gitlab.com") && len(parts) >= 2 {
		owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
		src.URL = fmt.Sprintf("https://%s/%s/%s.git", u.Host, owner, repo)
		// 从浏览 URL 中识别 /tree/<ref>/<sub...>。
		if len(parts) >= 4 && parts[2] == "tree" {
			if src.Ref == "" {
				src.Ref = parts[3]
			}
			if src.Subpath == "" && len(parts) > 4 {
				src.Subpath = strings.Join(parts[4:], "/")
			}
		}
		return src, nil
	}
	src.URL = u.String()
	return src, nil
}

// fetchSource 将源 clone 或下载到 dir，并返回应搜索 SKILL.md 的目录。
func fetchSource(ctx context.Context, src source, dir string) (string, error) {
	switch src.Kind {
	case "local":
		return src.URL, nil
	case "git":
		args := []string{"clone", "--depth", "1"}
		if src.Ref != "" {
			args = append(args, "--branch", src.Ref)
		}
		args = append(args, src.URL, dir)
		cmd := exec.CommandContext(ctx, "git", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return dir, nil
	case "zip":
		return downloadAndExtractZip(ctx, src.URL, dir)
	}
	return "", fmt.Errorf("unsupported source kind: %s", src.Kind)
}

// downloadAndExtractZip 将远程 zip 流式拉取到 dir/source.zip 并解压到
// dir/extracted。设 50MB 上限以控制最坏情况；更大归档应走 git。
// 容忍 GitHub 风格、将内容包在 `<repo>-<ref>/` 前缀下的压缩包，因 locateSkillRoot
// 会遍历整棵树。
func downloadAndExtractZip(ctx context.Context, rawURL, dir string) (string, error) {
	const maxZipBytes = 50 * 1024 * 1024
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "aranea-cli/skill-install")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download zip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("download zip: HTTP %d", resp.StatusCode)
	}
	zipPath := filepath.Join(dir, "source.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(resp.Body, maxZipBytes+1)); err != nil {
		out.Close()
		return "", fmt.Errorf("write zip: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		return "", err
	}
	if info.Size() > maxZipBytes {
		return "", fmt.Errorf("downloaded archive exceeds %dMB cap", maxZipBytes/(1024*1024))
	}
	extractDir := filepath.Join(dir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	if err := unzipInto(zipPath, extractDir); err != nil {
		return "", err
	}
	return extractDir, nil
}

// unzipInto 将 archivePath 解压到 destDir，并防护路径穿越（Zip Slip），
// 拒绝过大的成员。每项必须解析在 destDir 内；符号链接与设备项跳过。
func unzipInto(archivePath, destDir string) error {
	const maxFileBytes = 20 * 1024 * 1024
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range zr.File {
		if !f.Mode().IsRegular() && !f.FileInfo().IsDir() {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if f.UncompressedSize64 > maxFileBytes {
			return fmt.Errorf("zip entry %q exceeds %dMB", f.Name, maxFileBytes/(1024*1024))
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, io.LimitReader(rc, int64(maxFileBytes)+1)); err != nil {
			rc.Close()
			w.Close()
			return fmt.Errorf("extract %q: %w", f.Name, err)
		}
		rc.Close()
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}

// locateSkillRoot 返回应被打 zip 的目录。若已设置 subpath 则信任调用方。
// 否则遍历树查找 SKILL.md：0 个 → 错误，1 个 → 采用，>1 个 → 错误以迫使用
// --subpath 选择。
func locateSkillRoot(root, subpath string) (string, error) {
	if subpath != "" {
		full := filepath.Join(root, filepath.FromSlash(subpath))
		if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err != nil {
			return "", fmt.Errorf("SKILL.md not found in %s", full)
		}
		return full, nil
	}
	matches := []string{}
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "SKILL.md" {
			matches = append(matches, filepath.Dir(p))
		}
		return nil
	}); err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", errors.New("no SKILL.md found in source; pass --subpath to point at a directory")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple SKILL.md candidates: %v — pick one with --subpath", matches)
	}
}

// preValidate 在网络往返前做廉价本地检查。要求存在 SKILL.md、非空
// front-matter 的 description，且文件小于约 512KB。
func preValidate(root string) error {
	body, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return err
	}
	if len(body) > 512*1024 {
		return fmt.Errorf("SKILL.md too large: %d bytes (limit 512KB)", len(body))
	}
	text := string(body)
	if !strings.HasPrefix(text, "---") {
		return errors.New("SKILL.md must begin with a YAML front-matter delimiter (---)")
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return errors.New("SKILL.md front-matter must be terminated with `---`")
	}
	front := text[3 : 3+end]
	if !strings.Contains(front, "description") {
		return errors.New("SKILL.md front-matter must define `description`")
	}
	return nil
}

// zipDir 将 root（仅相对路径）打包为可流式传到 import 端点的内存 zip。
func zipDir(root string) (io.Reader, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	prefix := filepath.Base(root) + "/"
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(prefix + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// uploadAndWait 对 zip 执行 POST 并轮询任务直到后端声明 ready_to_apply
// 或返回错误状态。
func uploadAndWait(ctx context.Context, g *apiclient.GlobalContext, body io.Reader, fileName string) (domain.SkillImportJob, error) {
	var startResp struct {
		JobID string `json:"job_id"`
	}
	if err := g.Client().PostMultipart(ctx, "/api/v1/skills/import", "file", fileName, body, nil, &startResp); err != nil {
		return domain.SkillImportJob{}, err
	}
	return waitForJob(ctx, g, startResp.JobID)
}

func waitForJob(ctx context.Context, g *apiclient.GlobalContext, jobID string) (domain.SkillImportJob, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		var job domain.SkillImportJob
		if err := g.Client().Get(ctx, "/api/v1/skills/import/"+url.PathEscape(jobID), nil, &job); err != nil {
			return domain.SkillImportJob{}, err
		}
		switch job.Status {
		case "ready_to_apply", "applied", "completed", "needs_review":
			return job, nil
		case "failed", "cancelled":
			return job, fmt.Errorf("import job %s ended with status %s: %s", jobID, job.Status, job.Message)
		}
		if time.Now().After(deadline) {
			return job, fmt.Errorf("timeout waiting for import job %s (last status: %s)", jobID, job.Status)
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// resolveConflicts 构建 apply 决策列表。模式：
//
//   - skip   → 丢弃属于任一并冲突组的所有候选
//   - keep   → 保留已有技能（不创建候选）或保留入站候选（默认子标志值）
//   - refine → 对每个冲突组调用 /refine，接受合并结果作为新候选
//   - ask    → 按组进入交互式 REPL
func resolveConflicts(cmd *cobra.Command, g *apiclient.GlobalContext, job domain.SkillImportJob, mode, keep string) ([]domain.SkillImportDecision, error) {
	decisions := make([]domain.SkillImportDecision, 0, len(job.Candidates))
	conflicted := map[string]string{}
	for _, group := range job.ConflictGroups {
		for _, cid := range group.CandidateIDs {
			conflicted[cid] = group.GroupID
		}
	}
	for _, c := range job.Candidates {
		if _, ok := conflicted[c.CandidateID]; !ok {
			decisions = append(decisions, domain.SkillImportDecision{CandidateID: c.CandidateID, Action: "create"})
		}
	}
	for _, group := range job.ConflictGroups {
		decision, err := decideGroup(cmd, g, job.JobID, group, mode, keep)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func decideGroup(cmd *cobra.Command, g *apiclient.GlobalContext, jobID string, group domain.SkillConflictGroup, mode, keep string) (domain.SkillImportDecision, error) {
	if mode == "ask" {
		mode = askConflictMode(cmd, group)
	}
	switch mode {
	case "skip":
		return domain.SkillImportDecision{GroupID: group.GroupID, Action: "skip"}, nil
	case "keep":
		if keep == "existing" {
			return domain.SkillImportDecision{GroupID: group.GroupID, Action: "keep_existing"}, nil
		}
		return domain.SkillImportDecision{GroupID: group.GroupID, Action: "keep_incoming"}, nil
	case "refine":
		var refined domain.SkillRefineResult
		req := domain.SkillRefineRequest{Instructions: "Merge into a single canonical skill"}
		path := fmt.Sprintf("/api/v1/skills/import/%s/conflict-groups/%s/refine", url.PathEscape(jobID), url.PathEscape(group.GroupID))
		if err := g.Client().Post(cmd.Context(), path, req, &refined); err != nil {
			return domain.SkillImportDecision{}, err
		}
		return domain.SkillImportDecision{
			GroupID:           group.GroupID,
			Action:            "merge",
			MergedName:        refined.MergedName,
			MergedDescription: refined.MergedDescription,
			MergedBody:        refined.MergedBody,
			MergedTags:        refined.MergedTags,
		}, nil
	}
	return domain.SkillImportDecision{GroupID: group.GroupID, Action: "skip"}, nil
}

func askConflictMode(cmd *cobra.Command, group domain.SkillConflictGroup) string {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "\nconflict %s (similarity %.2f, risk %s):\n", group.GroupID, group.HighestSimilarityScore, group.Metrics.ConflictRisk)
	for _, src := range group.ExistingSkills {
		fmt.Fprintf(w, "  existing: %s (%s)\n", src.Name, src.Slug)
	}
	fmt.Fprintf(w, "  reason  : %s\n", group.Reason)
	fmt.Fprint(w, "  choose [k]eep-incoming / [e]xisting / [r]efine / [s]kip: ")
	r := bufio.NewReader(os.Stdin)
	answer, _ := r.ReadString('\n')
	switch strings.TrimSpace(strings.ToLower(answer)) {
	case "k", "keep", "keep-incoming":
		return "keep"
	case "e", "existing", "keep-existing":
		return "keep"
	case "r", "refine":
		return "refine"
	default:
		return "skip"
	}
}

func applyImport(ctx context.Context, g *apiclient.GlobalContext, jobID string, decisions []domain.SkillImportDecision) (domain.SkillImportApplyResult, error) {
	var result domain.SkillImportApplyResult
	body := domain.SkillImportApplyRequest{Decisions: decisions}
	if err := g.Client().Post(ctx, fmt.Sprintf("/api/v1/skills/import/%s/apply", url.PathEscape(jobID)), body, &result); err != nil {
		return domain.SkillImportApplyResult{}, err
	}
	return result, nil
}

// confirm 由所有会修改数据的 skill 命令共享。若设置 --yes / -y 则从不
// 提示；否则自 stdin 读取 y/N。
func confirm(cmd *cobra.Command, g *apiclient.GlobalContext, prompt string) bool {
	if g.Yes {
		return true
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", prompt)
	r := bufio.NewReader(os.Stdin)
	answer, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// safeJoin 将基址 URL 与 path 拼接，并确保无路径穿越。
func safeJoin(base, p string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path.Clean(p), "/")
}

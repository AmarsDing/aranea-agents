package pkginstall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"aranea-agents/internal/orgimport"
)

// Installer installs an aranea package against the backend HTTP API.
// It never imports internal/biz, internal/data, or pkg/trpc-agent-go.
type Installer struct {
	APIURL  string
	Token   string
	DryRun  bool
	Strict  bool
	Quiet   bool
	Timeout time.Duration
	// OnStep is called for each installation step; may be nil.
	OnStep func(step, total int, name, status string)
}

// Result summarises the installation outcome.
type Result struct {
	Steps   []StepResult
	Created int
	Updated int
	Skipped int
	Errors  []string
}

// StepResult records the outcome of one resource installation step.
type StepResult struct {
	Resource string // e.g. "mcp_server:my-mcp"
	Action   string // created|updated|skipped|error
	Message  string
}

const totalSteps = 6

// Install executes the package installation in dependency order:
// 1. MCP Servers → 2. Skills → 3. Org (industry/dept/pos) →
// 4. Agents → 5. Teams → 6. Graphs
func (ins *Installer) Install(pkgDir string, manifest *Manifest) (*Result, error) {
	result := &Result{}
	timeout := ins.Timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	// Step 1: MCP Servers.
	ins.report(1, totalSteps, "MCP 服务器", "installing")
	for _, spec := range manifest.Spec.MCPServers {
		sr := ins.installMCPServer(client, spec)
		result.Steps = append(result.Steps, sr)
		ins.countStep(result, sr)
	}
	ins.report(1, totalSteps, "MCP 服务器", fmt.Sprintf("done (%d)", len(manifest.Spec.MCPServers)))

	// Step 2: Skills.
	ins.report(2, totalSteps, "Skills", "installing")
	for _, spec := range manifest.Spec.Skills {
		sr := ins.installSkill(client, pkgDir, spec)
		result.Steps = append(result.Steps, sr)
		ins.countStep(result, sr)
	}
	ins.report(2, totalSteps, "Skills", fmt.Sprintf("done (%d)", len(manifest.Spec.Skills)))

	// Step 3: Org structure (via orgimport).
	ins.report(3, totalSteps, "行业/部门/岗位", "installing")
	if len(manifest.Spec.Companies) > 0 || len(manifest.Spec.Agents) > 0 || len(manifest.Spec.Teams) > 0 {
		orgSpec := &orgimport.Spec{
			Version: 1,
			Spec: orgimport.SpecBody{
				Companies: manifest.Spec.Companies,
				Agents:    manifest.Spec.Agents,
				Teams:     manifest.Spec.Teams,
			},
		}
		if !ins.DryRun {
			applier := orgimport.NewApplier(orgimport.ApplyOptions{
				APIURL:   ins.APIURL,
				APIToken: ins.Token,
				Timeout:  timeout,
			})
			orgResult, err := applier.Apply(orgSpec)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("org import: %v", err))
			} else {
				result.Created += orgResult.Created
				result.Updated += orgResult.Updated
				result.Skipped += orgResult.Skipped
				result.Errors = append(result.Errors, orgResult.Errors...)
			}
		}
	}
	ins.report(3, totalSteps, "行业/部门/岗位", "done")

	// Steps 4 & 5 are handled by orgimport above (agents + teams).
	ins.report(4, totalSteps, "Agents", "done (via org import)")
	ins.report(5, totalSteps, "Teams", "done (via org import)")

	// Step 6: Graphs.
	ins.report(6, totalSteps, "Graphs", "installing")
	for _, spec := range manifest.Spec.Graphs {
		sr := ins.installGraph(client, pkgDir, spec)
		result.Steps = append(result.Steps, sr)
		ins.countStep(result, sr)
	}
	ins.report(6, totalSteps, "Graphs", fmt.Sprintf("done (%d)", len(manifest.Spec.Graphs)))

	if ins.Strict && !ins.DryRun && len(result.Errors) > 0 {
		return result, fmt.Errorf("package install completed with %d error(s)", len(result.Errors))
	}
	return result, nil
}

func (ins *Installer) report(step, total int, name, status string) {
	if ins.OnStep != nil {
		ins.OnStep(step, total, name, status)
	}
}

func (ins *Installer) countStep(r *Result, sr StepResult) {
	switch sr.Action {
	case "created":
		r.Created++
	case "updated":
		r.Updated++
	case "skipped":
		r.Skipped++
	case "error":
		r.Errors = append(r.Errors, sr.Message)
	}
}

func (ins *Installer) doJSON(client *http.Client, method, path string, body any) ([]byte, int, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ins.APIURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if ins.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ins.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes(), resp.StatusCode, nil
}

// installMCPServer creates or skips an MCP server.
func (ins *Installer) installMCPServer(client *http.Client, spec MCPServerSpec) StepResult {
	resource := "mcp_server:" + spec.Key
	if ins.DryRun {
		return StepResult{Resource: resource, Action: "skipped", Message: "dry-run"}
	}
	// Check if already exists by listing and matching key.
	body, status, err := ins.doJSON(client, http.MethodGet, "/v1/mcp-servers", nil)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	if status == http.StatusOK {
		var listResp struct {
			Items []struct {
				Key string `json:"key"`
				ID  string `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal(body, &listResp) == nil {
			for _, item := range listResp.Items {
				if item.Key == spec.Key {
					return StepResult{Resource: resource, Action: "skipped", Message: "already exists"}
				}
			}
		}
	}
	// Build config JSON.
	configJSON := "{}"
	if spec.Config != nil {
		if b, err := json.Marshal(spec.Config); err == nil {
			configJSON = string(b)
		}
	}
	payload := map[string]any{
		"key":         spec.Key,
		"name":        spec.Name,
		"description": spec.Description,
		"enabled":     spec.Enabled,
		"config_json": configJSON,
	}
	_, status, err = ins.doJSON(client, http.MethodPost, "/v1/mcp-servers", payload)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	if status >= 300 {
		return StepResult{Resource: resource, Action: "error", Message: fmt.Sprintf("HTTP %d", status)}
	}
	return StepResult{Resource: resource, Action: "created"}
}

// installSkill uploads a skill zip via multipart to POST /v1/skills/import.
func (ins *Installer) installSkill(client *http.Client, pkgDir string, spec SkillSpec) StepResult {
	resource := "skill:" + spec.URL
	if spec.Path != "" {
		resource = "skill:" + spec.Path
	}
	if ins.DryRun {
		return StepResult{Resource: resource, Action: "skipped", Message: "dry-run"}
	}

	var zipPath string
	if spec.Path != "" {
		// Local path relative to pkgDir.
		zipPath = filepath.Join(pkgDir, spec.Path)
	} else if spec.URL != "" {
		// Clone the skill from URL to a temp dir, zip the subpath.
		tmpDir, cleanup, err := FetchFromURL(spec.URL, spec.Ref, ins.Quiet)
		if err != nil {
			return StepResult{Resource: resource, Action: "error", Message: err.Error()}
		}
		defer cleanup()
		srcDir := tmpDir
		if spec.Subpath != "" {
			srcDir = filepath.Join(tmpDir, spec.Subpath)
		}
		tmpFile, err := os.CreateTemp("", "skill-*.zip")
		if err != nil {
			return StepResult{Resource: resource, Action: "error", Message: fmt.Sprintf("create temp zip: %v", err)}
		}
		tmpZip := tmpFile.Name()
		_ = tmpFile.Close()
		if err := zipDir(srcDir, tmpZip); err != nil {
			_ = os.Remove(tmpZip)
			return StepResult{Resource: resource, Action: "error", Message: fmt.Sprintf("zip skill: %v", err)}
		}
		defer os.Remove(tmpZip)
		zipPath = tmpZip
	} else {
		return StepResult{Resource: resource, Action: "error", Message: "skill spec requires url or path"}
	}

	f, err := os.Open(zipPath)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil {
		const maxSkillZipBytes = int64(100 << 20)
		if st.Size() > maxSkillZipBytes {
			return StepResult{Resource: resource, Action: "error", Message: "skill zip exceeds 100 MB limit"}
		}
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	if _, err := io.Copy(fw, f); err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	_ = w.WriteField("source", "cli_url")
	_ = w.WriteField("source_url", spec.URL)
	if spec.Ref != "" {
		_ = w.WriteField("source_ref", spec.Ref)
	}
	if spec.Subpath != "" {
		_ = w.WriteField("source_subpath", spec.Subpath)
	}
	if spec.Decision != "" {
		_ = w.WriteField("decision", spec.Decision)
	}
	if err := w.Close(); err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}

	req, err := http.NewRequest(http.MethodPost, ins.APIURL+"/v1/skills/import", &buf)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if ins.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ins.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return StepResult{Resource: resource, Action: "error", Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return StepResult{Resource: resource, Action: "created"}
}

// installGraph reads a local JSON file and POSTs to /v1/graph/import.
func (ins *Installer) installGraph(client *http.Client, pkgDir string, spec GraphSpec) StepResult {
	resource := "graph:" + spec.File
	if spec.Name != "" {
		resource = "graph:" + spec.Name
	}
	if ins.DryRun {
		return StepResult{Resource: resource, Action: "skipped", Message: "dry-run"}
	}
	jsonPath := filepath.Join(pkgDir, spec.File)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	payload := map[string]any{
		"json": string(data),
	}
	if spec.Name != "" {
		payload["name"] = spec.Name
	}
	_, status, err := ins.doJSON(client, http.MethodPost, "/v1/graph/import", payload)
	if err != nil {
		return StepResult{Resource: resource, Action: "error", Message: err.Error()}
	}
	if status >= 300 {
		return StepResult{Resource: resource, Action: "error", Message: fmt.Sprintf("HTTP %d", status)}
	}
	return StepResult{Resource: resource, Action: "created"}
}

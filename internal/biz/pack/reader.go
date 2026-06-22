package pack

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"aranea-agents/pkg/apierror"

	"gopkg.in/yaml.v3"
)

const (
	manifestFile = "manifest.yaml"
	taxonomyFile = "taxonomy.yaml"
	agentsDir    = "agents/"
	teamsDir     = "teams/"
	graphsDir    = "graphs/"

	// 安全限制常量
	MaxTarEntries       = 1000              // 单个 Pack 最多 1000 个 tar 条目
	MaxEntrySize  int64 = 10 * 1024 * 1024  // 单个条目最大 10MB
	MaxTotalSize  int64 = 200 * 1024 * 1024 // Pack 解压后总大小上限 200MB
	MaxPackSize         = 200 * 1024 * 1024 // Pack 原始文件大小上限 200MB
)

// parsePackEntry 解析单个 Pack 条目并写入内存模型。
// relPath 是相对于 Pack 根目录的路径，data 是文件内容。
func parsePackEntry(relPath string, data []byte, p *Pack) error {
	switch {
	case relPath == manifestFile:
		if err := yaml.Unmarshal(data, &p.Manifest); err != nil {
			return apierror.BadRequest(apierror.DomainPack, "parse manifest.yaml failed: %s", err.Error())
		}
	case relPath == taxonomyFile:
		var spec OrganizationPackSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return apierror.BadRequest(apierror.DomainPack, "parse taxonomy.yaml failed: %s", err.Error())
		}
		p.Organization = &spec
	case strings.HasPrefix(relPath, agentsDir) && strings.HasSuffix(relPath, ".yaml"):
		var spec AgentPackSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return apierror.BadRequest(apierror.DomainPack, "parse %s failed: %s", relPath, err.Error())
		}
		p.Agents = append(p.Agents, spec)
	case strings.HasPrefix(relPath, agentsDir) && !strings.HasSuffix(relPath, ".yaml"):
		// Agent 文件（如 agents/go-senior-general/IDENTITY.md）
		parts := strings.SplitN(strings.TrimPrefix(relPath, agentsDir), "/", 2)
		if len(parts) == 2 {
			agentKey := parts[0]
			fileName := parts[1]
			if p.AgentFiles[agentKey] == nil {
				p.AgentFiles[agentKey] = make(map[string]string)
			}
			p.AgentFiles[agentKey][fileName] = string(data)
		}
	case strings.HasPrefix(relPath, teamsDir) && strings.HasSuffix(relPath, ".yaml"):
		var spec TeamPackSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return apierror.BadRequest(apierror.DomainPack, "parse %s failed: %s", relPath, err.Error())
		}
		p.Teams = append(p.Teams, spec)
	case strings.HasPrefix(relPath, graphsDir) && strings.HasSuffix(relPath, ".yaml"):
		var spec GraphPackSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return apierror.BadRequest(apierror.DomainPack, "parse %s failed: %s", relPath, err.Error())
		}
		p.Graphs = append(p.Graphs, spec)
	}
	return nil
}

// validatePackManifest 校验 Pack 的 manifest 必填字段。
func validatePackManifest(p *Pack) error {
	if p.Manifest.APIVersion == "" {
		return apierror.BadRequest(apierror.DomainPack, "manifest.yaml missing api_version")
	}
	if p.Manifest.Kind == "" {
		return apierror.BadRequest(apierror.DomainPack, "manifest.yaml missing kind")
	}
	return nil
}

// ReadPack 从 tar.gz 读取 .arpack 并解析为内存模型。
func ReadPack(r io.Reader) (*Pack, error) {
	// 使用 LimitedReader 限制总解压大小，防止 gzip 炸弹
	lr := io.LimitReader(r, MaxTotalSize)

	gzr, err := gzip.NewReader(lr)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainPack, "gzip decompression failed: %s", err.Error())
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	p := &Pack{
		AgentFiles: make(map[string]map[string]string),
	}

	entryCount := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainPack, "read tar entry failed: %s", err.Error())
		}

		entryCount++
		if entryCount > MaxTarEntries {
			return nil, apierror.BadRequest(apierror.DomainPack, "tar entry count exceeds limit %d", MaxTarEntries)
		}

		// 路径遍历检查：清洗路径后验证
		cleanName := filepath.ToSlash(filepath.Clean(hdr.Name))
		// filepath.Clean 已解析 ..，所以检查清洗前原始路径是否包含 ..
		// 同时验证清洗后路径不以 / 开头（绝对路径）
		if strings.Contains(hdr.Name, "..") || strings.HasPrefix(cleanName, "/") {
			return nil, apierror.BadRequest(apierror.DomainPack, "entry path contains illegal traversal: %s", hdr.Name)
		}

		// 跳过符号链接和硬链接
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}

		// 单条目大小检查
		if hdr.Size > MaxEntrySize {
			return nil, apierror.BadRequest(apierror.DomainPack, "entry %s size %d exceeds limit %d", hdr.Name, hdr.Size, MaxEntrySize)
		}

		// 使用 LimitedReader 读取条目内容
		data, err := io.ReadAll(io.LimitReader(tr, MaxEntrySize+1))
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainPack, "read %s content failed: %s", hdr.Name, err.Error())
		}
		if int64(len(data)) > MaxEntrySize {
			return nil, apierror.BadRequest(apierror.DomainPack, "entry %s content exceeds size limit", hdr.Name)
		}

		if err := parsePackEntry(cleanName, data, p); err != nil {
			return nil, err
		}
	}

	if err := validatePackManifest(p); err != nil {
		return nil, err
	}

	return p, nil
}

// ReadPackFromFS 从 fs.FS（如 embed.FS）读取 .arpack 目录结构并解析为内存模型。
// root 是 fs.FS 中 Pack 目录的根路径（如 "builtin-templates"）。
func ReadPackFromFS(fsys fs.FS, root string) (*Pack, error) {
	p := &Pack{
		AgentFiles: make(map[string]map[string]string),
	}

	entryCount := 0

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		entryCount++
		if entryCount > MaxTarEntries {
			return apierror.Internal(apierror.DomainPack, "fs entry count exceeds limit %d", MaxTarEntries)
		}

		// 获取相对于 root 的路径
		relPath := strings.TrimPrefix(path, root+"/")
		relPath = filepath.ToSlash(relPath)

		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return apierror.Internal(apierror.DomainPack, "read %s failed: %s", path, readErr.Error())
		}
		if int64(len(data)) > MaxEntrySize {
			return apierror.Internal(apierror.DomainPack, "entry %s content exceeds size limit", path)
		}

		if parseErr := parsePackEntry(relPath, data, p); parseErr != nil {
			return parseErr
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := validatePackManifest(p); err != nil {
		return nil, err
	}

	return p, nil
}

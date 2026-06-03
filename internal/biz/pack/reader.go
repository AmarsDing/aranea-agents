package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestFile = "manifest.yaml"
	taxonomyFile = "taxonomy.yaml"
	agentsDir    = "agents/"
	teamsDir     = "teams/"
	graphsDir    = "graphs/"

	// 安全限制常量
	MaxTarEntries     = 1000  // 单个 Pack 最多 1000 个 tar 条目
	MaxEntrySize  int64 = 10 * 1024 * 1024 // 单个条目最大 10MB
	MaxTotalSize  int64 = 200 * 1024 * 1024 // Pack 解压后总大小上限 200MB
	MaxPackSize         = 200 * 1024 * 1024 // Pack 原始文件大小上限 200MB
)

// ReadPack 从 tar.gz 读取 .arpack 并解析为内存模型。
func ReadPack(r io.Reader) (*Pack, error) {
	// 使用 LimitedReader 限制总解压大小，防止 gzip 炸弹
	lr := io.LimitReader(r, MaxTotalSize)

	gzr, err := gzip.NewReader(lr)
	if err != nil {
		return nil, fmt.Errorf("pack: gzip 解压失败: %w", err)
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
			return nil, fmt.Errorf("pack: 读取 tar 条目失败: %w", err)
		}

		entryCount++
		if entryCount > MaxTarEntries {
			return nil, fmt.Errorf("pack: tar 条目数超过上限 %d", MaxTarEntries)
		}

		// 路径遍历检查：清洗路径后验证
		cleanName := filepath.ToSlash(filepath.Clean(hdr.Name))
		// filepath.Clean 已解析 ..，所以检查清洗前原始路径是否包含 ..
		// 同时验证清洗后路径不以 / 开头（绝对路径）
		if strings.Contains(hdr.Name, "..") || strings.HasPrefix(cleanName, "/") {
			return nil, fmt.Errorf("pack: 条目路径包含非法遍历: %s", hdr.Name)
		}

		// 跳过符号链接和硬链接
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}

		// 单条目大小检查
		if hdr.Size > MaxEntrySize {
			return nil, fmt.Errorf("pack: 条目 %s 大小 %d 超过上限 %d", hdr.Name, hdr.Size, MaxEntrySize)
		}

		// 使用 LimitedReader 读取条目内容
		data, err := io.ReadAll(io.LimitReader(tr, MaxEntrySize+1))
		if err != nil {
			return nil, fmt.Errorf("pack: 读取 %s 内容失败: %w", hdr.Name, err)
		}
		if int64(len(data)) > MaxEntrySize {
			return nil, fmt.Errorf("pack: 条目 %s 内容超过大小上限", hdr.Name)
		}

		name := cleanName

		switch {
		case name == manifestFile:
			if err := yaml.Unmarshal(data, &p.Manifest); err != nil {
				return nil, fmt.Errorf("pack: 解析 manifest.yaml 失败: %w", err)
			}
		case name == taxonomyFile:
			var spec TaxonomyPackSpec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return nil, fmt.Errorf("pack: 解析 taxonomy.yaml 失败: %w", err)
			}
			p.Taxonomy = &spec
		case strings.HasPrefix(name, agentsDir) && strings.HasSuffix(name, ".yaml"):
			var spec AgentPackSpec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return nil, fmt.Errorf("pack: 解析 %s 失败: %w", name, err)
			}
			p.Agents = append(p.Agents, spec)
		case strings.HasPrefix(name, agentsDir) && !strings.HasSuffix(name, ".yaml"):
			// Agent 文件（如 agents/go-senior-general/IDENTITY.md）
			parts := strings.SplitN(strings.TrimPrefix(name, agentsDir), "/", 2)
			if len(parts) == 2 {
				agentKey := parts[0]
				fileName := parts[1]
				if p.AgentFiles[agentKey] == nil {
					p.AgentFiles[agentKey] = make(map[string]string)
				}
				p.AgentFiles[agentKey][fileName] = string(data)
			}
		case strings.HasPrefix(name, teamsDir) && strings.HasSuffix(name, ".yaml"):
			var spec TeamPackSpec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return nil, fmt.Errorf("pack: 解析 %s 失败: %w", name, err)
			}
			p.Teams = append(p.Teams, spec)
		case strings.HasPrefix(name, graphsDir) && strings.HasSuffix(name, ".yaml"):
			var spec GraphPackSpec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return nil, fmt.Errorf("pack: 解析 %s 失败: %w", name, err)
			}
			p.Graphs = append(p.Graphs, spec)
		}
	}

	if p.Manifest.APIVersion == "" {
		return nil, fmt.Errorf("pack: manifest.yaml 缺少 api_version")
	}
	if p.Manifest.Kind == "" {
		return nil, fmt.Errorf("pack: manifest.yaml 缺少 kind")
	}

	return p, nil
}

package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestFile  = "manifest.yaml"
	taxonomyFile  = "taxonomy.yaml"
	agentsDir     = "agents/"
	teamsDir      = "teams/"
	graphsDir     = "graphs/"
)

// ReadPack 从 tar.gz 读取 .arpack 并解析为内存模型。
func ReadPack(r io.Reader) (*Pack, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("pack: gzip 解压失败: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	p := &Pack{
		AgentFiles: make(map[string]map[string]string),
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("pack: 读取 tar 条目失败: %w", err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("pack: 读取 %s 内容失败: %w", hdr.Name, err)
		}

		name := hdr.Name

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

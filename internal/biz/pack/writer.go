package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WritePack 将内存模型打包为 tar.gz 写出。
func WritePack(p *Pack, w io.Writer) error {
	gzw := gzip.NewWriter(w)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	now := time.Now().Unix()

	// 1. 写入 manifest.yaml
	if err := writeYAMLFile(tw, manifestFile, p.Manifest, now); err != nil {
		return fmt.Errorf("pack: 写入 manifest.yaml 失败: %w", err)
	}

	// 2. 写入 taxonomy.yaml
	if p.Organization != nil {
		if err := writeYAMLFile(tw, taxonomyFile, p.Organization, now); err != nil {
			return fmt.Errorf("pack: 写入 taxonomy.yaml 失败: %w", err)
		}
	}

	// 3. 写入 agents/ 目录
	sort.Slice(p.Agents, func(i, j int) bool {
		return p.Agents[i].Key < p.Agents[j].Key
	})
	for _, agent := range p.Agents {
		path := agentsDir + agent.Key + ".yaml"
		if err := writeYAMLFile(tw, path, agent, now); err != nil {
			return fmt.Errorf("pack: 写入 %s 失败: %w", path, err)
		}
	}

	// 4. 写入 Agent 文件
	for agentKey, files := range p.AgentFiles {
		// 按文件名排序保证确定性输出
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			path := agentsDir + agentKey + "/" + name
			if err := writeRawFile(tw, path, []byte(files[name]), now); err != nil {
				return fmt.Errorf("pack: 写入 %s 失败: %w", path, err)
			}
		}
	}

	// 5. 写入 teams/ 目录
	sort.Slice(p.Teams, func(i, j int) bool {
		return p.Teams[i].Key < p.Teams[j].Key
	})
	for _, team := range p.Teams {
		path := teamsDir + team.Key + ".yaml"
		if err := writeYAMLFile(tw, path, team, now); err != nil {
			return fmt.Errorf("pack: 写入 %s 失败: %w", path, err)
		}
	}

	// 6. 写入 graphs/ 目录
	sort.Slice(p.Graphs, func(i, j int) bool {
		return p.Graphs[i].ID < p.Graphs[j].ID
	})
	for _, graph := range p.Graphs {
		path := graphsDir + graph.ID + ".yaml"
		if err := writeYAMLFile(tw, path, graph, now); err != nil {
			return fmt.Errorf("pack: 写入 %s 失败: %w", path, err)
		}
	}

	return nil
}

func writeYAMLFile(tw *tar.Writer, name string, v any, modTime int64) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化 %s 失败: %w", name, err)
	}
	return writeRawFile(tw, name, data, modTime)
}

func writeRawFile(tw *tar.Writer, name string, data []byte, modTime int64) error {
	// 确保路径使用正斜杠（tar 规范）
	name = strings.ReplaceAll(name, string(filepath.Separator), "/")

	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
		ModTime: time.Unix(modTime, 0),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

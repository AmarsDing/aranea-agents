package main

// Harness 多任务运行台（fleet，复刻 Orca）装配。
//
// 与 wire_githubpanel.go 同例：改定义只需动本文件 + wire_part2.go 的 provider
// 清单 + wire_gen.go 手工镜像。

import (
	"context"
	"net"
	"os"
	"strings"

	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/fleet"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// fleetProjectSource 把 agentbridge 的项目登记（coding_projects）适配成运行台的项目来源。
type fleetProjectSource struct {
	repo agentbridge.ProjectRepo
}

const fleetProjectWorkspace = "default" // 与 agentbridge API 的项目登记同一租户口径

func (s fleetProjectSource) ListProjects(ctx context.Context) ([]fleet.Project, error) {
	if s.repo == nil {
		return nil, nil
	}
	items, err := s.repo.List(ctx, fleetProjectWorkspace)
	if err != nil {
		return nil, err
	}
	out := make([]fleet.Project, 0, len(items))
	for _, p := range items {
		if p == nil {
			continue
		}
		out = append(out, fleet.Project{ID: p.ID, Name: p.Name, Path: p.Path})
	}
	return out, nil
}

func (s fleetProjectSource) AddProject(ctx context.Context, name, path string) (fleet.Project, error) {
	if err := s.repo.Upsert(ctx, &agentbridge.CodingProject{Workspace: fleetProjectWorkspace, Name: name, Path: path}); err != nil {
		return fleet.Project{}, err
	}
	got, err := s.repo.GetByName(ctx, fleetProjectWorkspace, name)
	if err != nil {
		return fleet.Project{}, err
	}
	return fleet.Project{ID: got.ID, Name: got.Name, Path: got.Path}, nil
}

// fleetServerURL 由 HTTP 监听地址推出 agent 终端回连的本机地址。
// ARANEA_FLEET_SERVER_URL 可覆盖（反代/容器端口映射场景）。
func fleetServerURL(c *conf.Server) string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_FLEET_SERVER_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	addr := ""
	if c != nil && c.Http != nil {
		addr = c.Http.Addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://127.0.0.1:8810"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// provideFleetUsecase 装配运行台用例；数据层不可用时返回 nil（服务回 Unavailable）。
func provideFleetUsecase(d *data.Data, projects agentbridge.ProjectRepo, c *conf.Server, lg loggateway.Logger) *fleet.Usecase {
	store := data.NewFleetStore(d)
	if store == nil {
		return nil
	}
	return fleet.NewUsecase(store, fleetProjectSource{repo: projects}, fleet.Options{
		WorktreeRoot: strings.TrimSpace(os.Getenv("ARANEA_FLEET_WORKTREE_ROOT")),
		ServerURL:    fleetServerURL(c),
		CLIDir:       strings.TrimSpace(os.Getenv("ARANEA_FLEET_CLI_DIR")),
	}, lg)
}

// provideFleetService 装配 fleet.v1 服务。
func provideFleetService(uc *fleet.Usecase) *service.FleetService {
	return service.NewFleetService(uc)
}

package main

// ceoboard 域 providers(整合 P2-7)。
//
// 本文件不带构建标签:wire.go/wire_part2.go(wireinject)与 wire_gen.go
// (!wireinject,手工镜像)共享同一份 provider 定义,循 wire_evolution.go 先例;
// 改动只需维护本文件 + wire_gen.go 的装配调用。

import (
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"aranea-agents/internal/biz/ceoboard"
	cbstore "aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/service"
	loggateway "aranea-agents/pkg/loggateway"
)

// provideCeoboardStore 在 data 层主写柄上构建 ceoboard 事件仓储:
// entsql.OpenDB 仅包装共享连接池(store 不关 client/池,迁移由主迁移链统一负责),
// 不触碰 internal/data/data.go(对等会话高冲突区)。
func provideCeoboardStore(d *data.Data) *cbstore.Store {
	rawDB := providePrimaryRawDB(d)
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, rawDB)))
	return cbstore.New(client, rawDB)
}

// provideCeoboardRuntime 装配驾驶舱运行时;registry 目录加载失败/spool 目录
// 不可建/回源凭据环境变量缺失即返回错误拒绝启动——与原 cmd/server/main.go
// log.Fatalf 的 fail-fast 语义一致。
//
// 灰闸:bootstrap 未声明 ceoboard 段(bc == nil)时返回 (nil, nil)——存量
// 部署无该配置不得启动失败;段一出现(哪怕全空走默认值)即视为显式启用,
// 失败照样拒绝启动。rt 为 nil 时 service 层返回 nil、server 跳过注册、
// workers 跳过调度。
func provideCeoboardRuntime(bc *conf.Ceoboard, st *cbstore.Store, lg loggateway.Logger) (*ceoboard.Runtime, error) {
	if bc == nil {
		return nil, nil
	}
	return ceoboard.NewRuntime(bc, st, lg)
}

// provideCeoboardService 装配驾驶舱 BFF(rt 为 nil 时返回 nil,server 层跳过注册)。
func provideCeoboardService(rt *ceoboard.Runtime, lg loggateway.Logger) *service.CeoboardService {
	return service.NewCeoboardService(rt, lg)
}

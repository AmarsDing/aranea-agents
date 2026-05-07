package main

import (
	"context"
	"flag"
	"os"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	sse "github.com/tx7do/kratos-transport/transport/sse"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	Name     string
	Version  string
	flagconf string
	id, _    = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger log.Logger, gs *grpc.Server, hs *http.Server, ss *sse.Server) *kratos.App {
	srv := []transport.Server{gs, hs}
	if ss != nil {
		srv = append(srv, ss)
	}
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(srv...),
	)
}

func main() {
	flag.Parse()
	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	out, cleanup, err := wireApp(bc.Server, bc.Data, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	cronCtx, cancelCron := context.WithCancel(context.Background())
	defer cancelCron()
	if out.CronRunner != nil {
		interval := cronrunner.DefaultInterval()
		go out.CronRunner.Start(cronCtx, interval)
		logger.Log(log.LevelInfo, "msg", "cron runner started", "interval", interval.String())
	}

	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	if out.SkillWatch != nil {
		go out.SkillWatch.Start(watchCtx)
		logger.Log(log.LevelInfo, "msg", "skill filesystem watcher started")
	}

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}

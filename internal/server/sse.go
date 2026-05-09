package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"

	"github.com/go-kratos/kratos/v2/transport"
	sse "github.com/tx7do/kratos-transport/transport/sse"
)

var (
	_ transport.Server = (*sse.Server)(nil)

	// Default query key used by tx7do sse.Server.ServeHTTP; matches sse.StreamID default in upstream (see WithStreamIdKey).
	streamQueryKey = "stream"
)

const monitorLogsStreamID sse.StreamID = "monitor-logs"

// NewSSEServer 使用 tx7do/kratos-transport/transport/sse，独立监听（与 Kratos HTTP 不同端口）。
// 注册命名流 monitorLogsStreamID，使用 CreateStream + Publish；HandleFunc 包装 /monitor/logs/stream 并注入 stream 查询参数后交给内置 ServeHTTP。
// monitorLogs 绑定后，运维文本行经 biz.MonitorLogBroker.Publish 进入 Logs SSE（event: log）。
// 配置来自 internal/conf/conf.proto 中 Server.sse。
func NewSSEServer(c *conf.Server, teamRunEvents *biz.TeamRunEventBroker, monitorLogs *biz.MonitorLogBroker) *sse.Server {
	if c == nil || c.GetSse() == nil || !c.GetSse().GetEnable() {
		return nil
	}
	s := c.GetSse()
	network := s.GetNetwork()
	if network == "" {
		network = "tcp"
	}
	addr := s.GetAddr()
	if addr == "" {
		addr = ":8001"
	}
	path := s.GetPath()
	if path == "" {
		path = "/"
	}
	timeout := 120 * time.Second
	if s.GetTimeout() != nil {
		timeout = s.GetTimeout().AsDuration()
	}
	srv := sse.NewServer(
		sse.WithNetwork(network),
		sse.WithAddress(addr),
		sse.WithPath(path),
		sse.WithCodec("json"),
		sse.WithTimeout(timeout),
		sse.WithAutoStream(true),
		sse.WithHeaders(map[string]string{
			"X-Accel-Buffering": "no",
		}),
	)

	srv.CreateStream(monitorLogsStreamID)

	registerMonitorLogSSE(srv)
	registerTeamRunSSE(srv, teamRunEvents)

	ctx := context.Background()
	if monitorLogs != nil {
		monitorLogs.SetPublisher(func(c context.Context, line biz.MonitorLogLine) {
			publishBrokerMonitorLine(c, srv, line)
		})
		monitorLogs.Publish(ctx, "INFO", "monitor SSE stream ready (tx7do/kratos-transport); operational lines use MonitorLogBroker.Publish", "monitor-sse")
	}

	go monitorLogsHeartbeatLoop(ctx, srv)

	return srv
}

func registerMonitorLogSSE(srv *sse.Server) {
	srv.HandleFunc("/monitor/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		if !prepareSSEAccessControl(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		q.Set(streamQueryKey, string(monitorLogsStreamID))
		r.URL.RawQuery = q.Encode()
		r.RequestURI = ""
		srv.ServeHTTP(w, r)
	})
}

func publishBrokerMonitorLine(ctx context.Context, srv *sse.Server, line biz.MonitorLogLine) {
	if srv == nil {
		return
	}
	raw, err := json.Marshal(line)
	if err != nil {
		return
	}
	srv.Publish(ctx, monitorLogsStreamID, &sse.Event{
		Data:  raw,
		Event: []byte("log"),
	})
}

func monitorLogsHeartbeatLoop(ctx context.Context, srv *sse.Server) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if srv == nil {
				return
			}
			// SSE comment line keeps proxies/Nginx from buffering idle connections closed (aligned with legacy handleMonitorLogStream).
			srv.Publish(ctx, monitorLogsStreamID, &sse.Event{
				Comment: []byte(" heartbeat"),
			})
		}
	}
}

package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz/fleet"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/websocket"
)

// ============================================================================
// Harness 多任务运行台的两个非 proto 入口（WebSocket/裸 POST 无法用 proto 定义）：
//
//   GET  /v1/fleet/terminal-ws?id=<终端>   浏览器终端字节流。JWT 认证（cookie/
//        Bearer/token 查询参数）+ coding.manage + Origin 白名单（同 /v1/ws）。
//        服务端→浏览器：二进制帧 = 终端输出；文本帧 = {"type":"exit","code":N}。
//        浏览器→服务端：文本帧 {"type":"input","data":"..."} / {"type":"resize","cols":C,"rows":R}。
//   POST /v1/fleet/cli                     agent 终端里 fleetctl 的命令入口。
//        不走 JWT：按每个终端独立的随机令牌（ARANEA_FLEET_TOKEN）识别身份，
//        且只接受本机回环来源——agent 与服务同机运行。
// ============================================================================

// FleetEndpoints 挂运行台的终端流与 fleetctl 入口。
type FleetEndpoints struct {
	uc       *fleet.Usecase
	lg       loggateway.Logger
	upgrader websocket.Upgrader
}

// NewFleetEndpoints 装配；uc 为空时不注册任何路由。
func NewFleetEndpoints(uc *fleet.Usecase, lg loggateway.Logger) *FleetEndpoints {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &FleetEndpoints{
		uc: uc, lg: lg,
		upgrader: websocket.Upgrader{
			ReadBufferSize: 16 << 10, WriteBufferSize: 64 << 10,
			CheckOrigin: func(r *http.Request) bool {
				if WSOriginAllowed(r) {
					return true
				}
				lg.Warn("fleet WS origin rejected", loggateway.Str("origin", strings.TrimSpace(r.Header.Get("Origin"))))
				return false
			},
		},
	}
}

// RegisterOnKratos 注册路由。
func (f *FleetEndpoints) RegisterOnKratos(srv *kratoshttp.Server) {
	if f == nil || f.uc == nil {
		return
	}
	auth.RegisterNoAuthPath("/v1/fleet/cli")
	srv.HandleFunc("/v1/fleet/terminal-ws", f.handleTerminal)
	srv.HandleFunc("/v1/fleet/cli", f.handleCLI)
}

type fleetClientMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (f *FleetEndpoints) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, err := wsAuthenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !claims.HasPermission("coding.manage") {
		http.Error(w, "coding.manage permission required", http.StatusForbidden)
		return
	}
	sess, ok := f.uc.Session(strings.TrimSpace(r.URL.Query().Get("id")))
	if !ok {
		http.Error(w, "terminal not found", http.StatusNotFound)
		return
	}
	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	snapshot, frames, cancel := sess.Subscribe()
	defer cancel()
	done := make(chan struct{})

	// 读：键盘输入与尺寸变化。
	go func() {
		defer close(done)
		conn.SetReadLimit(1 << 20)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m fleetClientMsg
			if json.Unmarshal(raw, &m) != nil {
				continue
			}
			switch m.Type {
			case "input":
				_ = sess.Write([]byte(m.Data))
			case "resize":
				sess.Resize(m.Cols, m.Rows)
			}
		}
	}()

	write := func(kind int, data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(kind, data)
	}
	for len(snapshot) > 0 {
		n := min(len(snapshot), 64<<10)
		if write(websocket.BinaryMessage, snapshot[:n]) != nil {
			return
		}
		snapshot = snapshot[n:]
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case <-ping.C:
			if write(websocket.PingMessage, nil) != nil {
				return
			}
		case fr, ok := <-frames:
			if !ok {
				// 订阅被断开（输出太快跟不上）或进程退出：让前端重连回放。
				_ = write(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "resubscribe"))
				return
			}
			if fr.Exit {
				msg, _ := json.Marshal(map[string]any{"type": "exit", "code": fr.ExitCode})
				_ = write(websocket.TextMessage, msg)
				continue
			}
			if write(websocket.BinaryMessage, fr.Data) != nil {
				return
			}
		}
	}
}

type fleetCLIRequest struct {
	Cmd  string            `json:"cmd"`
	Args map[string]string `json:"args"`
}

type fleetCLIResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (f *FleetEndpoints) handleCLI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	reply := func(status int, resp fleetCLIResponse) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	}
	if r.Method != http.MethodPost {
		reply(http.StatusMethodNotAllowed, fleetCLIResponse{Error: "POST only"})
		return
	}
	if !isLoopback(r.RemoteAddr) {
		reply(http.StatusForbidden, fleetCLIResponse{Error: "fleetctl 只接受本机调用"})
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		reply(http.StatusBadRequest, fleetCLIResponse{Error: "read body failed"})
		return
	}
	var req fleetCLIRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Cmd) == "" {
		reply(http.StatusBadRequest, fleetCLIResponse{Error: "body must be {\"cmd\":...,\"args\":{...}}"})
		return
	}
	out, err := f.uc.HandleCLI(r.Context(), token, strings.TrimSpace(req.Cmd), req.Args)
	if err != nil {
		status := http.StatusBadRequest
		msg := err.Error()
		if ae, ok := apierror.From(err); ok {
			msg = ae.Message
			switch ae.Code {
			case apierror.CodeUnauthorized:
				status = http.StatusUnauthorized
			case apierror.CodeNotFound:
				status = http.StatusNotFound
			case apierror.CodeForbidden:
				status = http.StatusForbidden
			}
		}
		reply(status, fleetCLIResponse{Error: msg})
		return
	}
	reply(http.StatusOK, fleetCLIResponse{OK: true, Output: out})
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

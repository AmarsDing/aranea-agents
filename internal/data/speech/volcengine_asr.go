package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// volcFlagLastPackage 标记服务端最后一帧（0b0010）。帧头其余 flags 见 volc_frame.go。
const volcFlagLastPackage byte = 0x2

// defaultASRAckTimeout 是等待服务端应答 full client request 的默认超时。
// SAUC 协议保证该应答必达；超时即上游静默/协议不匹配（真机事故：误连 v2 端点
// 导致 WS 握手成功但永不应答），必须 fail-fast 而非挂起到语音会话 10min 空闲回收。
const defaultASRAckTimeout = 3 * time.Second

// volcASRProvider 实现火山 SAUC 流式 ASR（双向 WS，服务端 VAD 端点检测）。
// 协议字段按火山公开文档；字节级真机校准归 V1-T10。
type volcASRProvider struct {
	cfg  biz.ASRProviderConfig
	dial wsDialer
	lg   loggateway.Logger
	// ackTimeout 覆盖默认应答超时（测试用）；<=0 回退 defaultASRAckTimeout。
	ackTimeout time.Duration
}

func newVolcASRProvider(cfg biz.ASRProviderConfig, lg loggateway.Logger) biz.StreamingASRProvider {
	return &volcASRProvider{cfg: cfg, dial: gorillaDialer, lg: lg, ackTimeout: defaultASRAckTimeout}
}

func (p *volcASRProvider) Open(ctx context.Context, sc biz.ASRSessionConfig) (biz.ASRSession, error) {
	if sc.SampleRate == 0 {
		sc.SampleRate = 16000
	}
	if sc.Language == "" {
		sc.Language = p.cfg.Language
	}
	header := nethttp.Header{}
	setVolcAuthHeader(header, p.cfg.APIKey, p.cfg.AppKey, p.cfg.AccessKey)
	header.Set("X-Api-Resource-Id", p.cfg.ResourceID)
	header.Set("X-Api-Connect-Id", uuid.NewString())
	conn, err := p.dial(ctx, p.cfg.Endpoint, header)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeUnavailable, "speech")
	}
	s := &volcASRSession{
		conn:   conn,
		events: make(chan biz.ASREvent, 16),
		done:   make(chan struct{}),
		lg:     p.lg,
	}
	// 真机校准（2026-08-08）：bigmodel 端点把 full client request 计入序号空间
	// （autoAssignedSequence 校验严格连续）。full request 显式 seq=1，音频帧从 2 续号。
	s.seq.Store(1)
	if err := s.sendFullClientRequest(sc); err != nil {
		_ = conn.Close()
		return nil, apierror.Wrap(err, apierror.CodeUnavailable, "speech")
	}
	// SAUC 协议保证 full client request 必有 full server response（或 error frame）。
	// 同步消费首帧应答：上游静默/协议不匹配时 fail-fast，而非让语音会话挂起。
	if err := p.waitServerAck(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	go s.readPump()
	return s, nil
}

// ackResult 承载首帧读取结果（带缓冲，Open 超时返回后 goroutine 随 conn.Close 退出，无泄漏）。
type ackResult struct {
	frame volcFrame
	err   error
}

// waitServerAck 同步等待 full client request 的服务端首帧应答。
// 超时 / 读失败 / error frame 均判定 Open 失败（UNAVAILABLE），由调用方关闭连接。
func (p *volcASRProvider) waitServerAck(ctx context.Context, conn wsConn) error {
	timeout := p.ackTimeout
	if timeout <= 0 {
		timeout = defaultASRAckTimeout
	}
	ch := make(chan ackResult, 1)
	go func() {
		f, err := readFirstServerFrame(conn)
		ch <- ackResult{f, err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return apierror.Wrap(ctx.Err(), apierror.CodeUnavailable, "speech")
	case <-timer.C:
		err := apierror.Unavailable("speech", "ASR server did not respond to full client request within %v (upstream silent or protocol mismatch)", timeout)
		p.lg.Warn("volc asr: open failed, server silent after full client request", loggateway.Err(err))
		return err
	case r := <-ch:
		if r.err != nil {
			return apierror.Wrap(r.err, apierror.CodeUnavailable, "speech")
		}
		if r.frame.msgType == volcMsgError {
			return apierror.Unavailable("speech", "ASR server rejected full client request: %s", formatVolcError(r.frame))
		}
		return nil
	}
}

// readFirstServerFrame 读取服务端首帧（须为 SAUC 二进制帧）。
func readFirstServerFrame(conn wsConn) (volcFrame, error) {
	mt, data, err := conn.ReadMessage()
	if err != nil {
		return volcFrame{}, err
	}
	if mt != websocket.BinaryMessage {
		return volcFrame{}, fmt.Errorf("expected binary first frame, got message type %d", mt)
	}
	return unmarshalVolcFrame(bytes.NewReader(data))
}

type volcASRSession struct {
	conn      wsConn
	events    chan biz.ASREvent
	done      chan struct{}
	closeOnce sync.Once
	seq       atomic.Int32
	lg        loggateway.Logger
	// finalCursor 是已发射 Final 的 definite 语句游标（utterances 累积去重）。
	// SAUC 服务端每帧响应都携带全量 utterances 列表，已 definite 的语句会持续
	// 回放；无去重时同一终稿被无限重复发射（真机事故 2026-08-09：听写模式
	// 每个重复 Final 追加进输入框 → 无限输入）。
	finalCursor int
}

func (s *volcASRSession) sendFullClientRequest(sc biz.ASRSessionConfig) error {
	body := map[string]any{
		"user":  map[string]any{"uid": "aranea"},
		"audio": map[string]any{"format": "pcm", "rate": sc.SampleRate, "bits": 16, "channel": 1},
		"request": map[string]any{
			"model_name":      "bigmodel",
			"enable_punc":     true,
			"enable_itn":      true,
			"show_utterances": true,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagPositiveSeq, seq: 1, json: true, payload: raw}, true)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcASRSession) Write(pcm []byte) error {
	n := s.seq.Add(1)
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagPositiveSeq, seq: n, payload: pcm}, false)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// Finish 发送末帧（flags=0b0010：最后一包、无序号字段），标记当前语句结束
// （voice.commit / PTT）。无序号即不参与 autoAssignedSequence 校验，规避末帧
// 绝对值约定（-(N) vs -(N+1)）的端点间差异——真机校准选择。
func (s *volcASRSession) Finish() error {
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagLastPackage, payload: []byte{}}, false)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcASRSession) Events() <-chan biz.ASREvent { return s.events }

func (s *volcASRSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		err = s.conn.Close()
	})
	return err
}

func (s *volcASRSession) emit(ev biz.ASREvent) {
	select {
	case s.events <- ev:
	case <-s.done:
	}
}

func (s *volcASRSession) readPump() {
	defer close(s.events)
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case <-s.done:
				return // 主动 Close 的正常路径
			default:
			}
			// 真机校准（2026-08-08）：火山在末帧应答后以 close 1000 正常关闭
			// （reason "finish last sequence"）。识别结果已交付，正常关闭是
			// 流结束信号而非错误——误报 ASR_ERROR 会把 thinking 态打断回 listening。
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			s.emit(biz.ASREvent{Type: biz.ASREventError, Err: apierror.Wrap(err, apierror.CodeUnavailable, "speech")})
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		if err != nil {
			s.lg.Warn("volc asr: undecodable frame", loggateway.Err(err))
			continue
		}
		switch f.msgType {
		case volcMsgError:
			s.emit(biz.ASREvent{Type: biz.ASREventError, Err: apierror.Internal("speech", "volc asr error: %s", formatVolcError(f))})
		case volcMsgFullServerResponse:
			s.handleResponse(f)
		}
	}
}

// saucResponse 是火山 SAUC 服务端 JSON 响应（真机字段校准归 V1-T10）。
type saucResponse struct {
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Text     string `json:"text"`
			Definite bool   `json:"definite"`
			EndTime  int    `json:"end_time"`
		} `json:"utterances"`
	} `json:"result"`
}

func (s *volcASRSession) handleResponse(f volcFrame) {
	var resp saucResponse
	if err := json.Unmarshal(f.payload, &resp); err != nil {
		s.lg.Warn("volc asr: undecodable response json", loggateway.Err(err))
		return
	}
	// 累积回放去重：definite 总数回退说明服务端开启了新累积窗口，游标归零。
	definite := 0
	for _, u := range resp.Result.Utterances {
		if u.Definite {
			definite++
		}
	}
	if definite < s.finalCursor {
		s.finalCursor = 0
	}
	emitted := false
	idx := 0
	for _, u := range resp.Result.Utterances {
		if !u.Definite {
			continue
		}
		idx++
		if idx <= s.finalCursor {
			continue // 已发射过的累积回放
		}
		s.finalCursor = idx
		if u.Text != "" {
			s.emit(biz.ASREvent{Type: biz.ASREventFinal, Text: u.Text, DurationMs: u.EndTime})
			emitted = true
		}
	}
	// 本帧已产出新终稿时跳过 Partial（result.text 与终稿同源，避免字幕回闪）。
	if !emitted && resp.Result.Text != "" {
		s.emit(biz.ASREvent{Type: biz.ASREventPartial, Text: resp.Result.Text})
	}
	if f.flags == volcFlagLastPackage {
		s.emit(biz.ASREvent{Type: biz.ASREventVadEnd})
	}
}

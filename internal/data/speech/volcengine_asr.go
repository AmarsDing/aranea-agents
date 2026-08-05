package speech

import (
	"bytes"
	"context"
	"encoding/json"
	nethttp "net/http"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// volcFlagLastPackage 标记服务端最后一帧（0b0010）。帧头其余 flags 见 volc_frame.go。
const volcFlagLastPackage byte = 0x2

// volcASRProvider 实现火山 SAUC 流式 ASR（双向 WS，服务端 VAD 端点检测）。
// 协议字段按火山公开文档；字节级真机校准归 V1-T10。
type volcASRProvider struct {
	cfg  biz.ASRProviderConfig
	dial wsDialer
	lg   loggateway.Logger
}

func newVolcASRProvider(cfg biz.ASRProviderConfig, lg loggateway.Logger) biz.StreamingASRProvider {
	return &volcASRProvider{cfg: cfg, dial: gorillaDialer, lg: lg}
}

func (p *volcASRProvider) Open(ctx context.Context, sc biz.ASRSessionConfig) (biz.ASRSession, error) {
	if sc.SampleRate == 0 {
		sc.SampleRate = 16000
	}
	if sc.Language == "" {
		sc.Language = p.cfg.Language
	}
	header := nethttp.Header{}
	header.Set("X-Api-App-Key", p.cfg.AppKey)
	header.Set("X-Api-Access-Key", p.cfg.AccessKey)
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
	if err := s.sendFullClientRequest(sc); err != nil {
		_ = conn.Close()
		return nil, apierror.Wrap(err, apierror.CodeUnavailable, "speech")
	}
	go s.readPump()
	return s, nil
}

type volcASRSession struct {
	conn      wsConn
	events    chan biz.ASREvent
	done      chan struct{}
	closeOnce sync.Once
	seq       atomic.Int32
	lg        loggateway.Logger
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
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: raw}, true)
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

// Finish 发送负序号空音频帧，标记当前语句结束（voice.commit / PTT）。
func (s *volcASRSession) Finish() error {
	n := s.seq.Load()
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagNegativeSeq, seq: -n, payload: []byte{}}, false)
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
			s.emit(biz.ASREvent{Type: biz.ASREventError, Err: apierror.Internal("speech", "volc asr error: %s", string(f.payload))})
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
	for _, u := range resp.Result.Utterances {
		if u.Definite && u.Text != "" {
			s.emit(biz.ASREvent{Type: biz.ASREventFinal, Text: u.Text, DurationMs: u.EndTime})
			return
		}
	}
	if resp.Result.Text != "" {
		s.emit(biz.ASREvent{Type: biz.ASREventPartial, Text: resp.Result.Text})
	}
	if f.flags == volcFlagLastPackage {
		s.emit(biz.ASREvent{Type: biz.ASREventVadEnd})
	}
}

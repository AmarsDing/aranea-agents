package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	nethttp "net/http"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// volcTTSProvider 实现火山 ws_binary 语音合成。
// V1 裁剪 #3：每句一条 WS 连接——TTSSession.Write 恰好调用一次（一句一合成），
// 双工流式/预连接升级归 V3-T5。
type volcTTSProvider struct {
	cfg  biz.TTSProviderConfig
	dial wsDialer
	lg   loggateway.Logger
}

func newVolcTTSProvider(cfg biz.TTSProviderConfig, lg loggateway.Logger) biz.StreamingTTSProvider {
	return &volcTTSProvider{cfg: cfg, dial: gorillaDialer, lg: lg}
}

func (p *volcTTSProvider) Open(_ context.Context, sc biz.TTSSessionConfig) (biz.TTSSession, error) {
	if sc.SampleRate == 0 {
		sc.SampleRate = 16000
	}
	if sc.Voice == "" {
		sc.Voice = p.cfg.Voice
	}
	if sc.SpeedRatio <= 0 {
		sc.SpeedRatio = p.cfg.SpeedRatio
	}
	return &volcTTSSession{
		p:     p,
		sc:    sc,
		audio: make(chan biz.TTSAudioChunk, 32),
	}, nil
}

type volcTTSSession struct {
	p       *volcTTSProvider
	sc      biz.TTSSessionConfig
	audio   chan biz.TTSAudioChunk
	started atomic.Bool
	ended   atomic.Bool
	conn    wsConn
	closeMu sync.Mutex
	closed  bool
}

func (s *volcTTSSession) Write(text string, _ bool) error {
	if s.started.Swap(true) {
		return errors.New("volc tts: Write called twice (V1 一句一连接)")
	}
	header := nethttp.Header{}
	header.Set("X-Api-App-Key", s.p.cfg.AppKey)
	header.Set("X-Api-Access-Key", s.p.cfg.AccessKey)
	header.Set("X-Api-Resource-Id", s.p.cfg.ResourceID)
	header.Set("X-Api-Connect-Id", uuid.NewString())
	conn, err := s.p.dial(context.Background(), s.p.cfg.Endpoint, header)
	if err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil // 错误经 Audio() 通道上报
	}
	s.setConn(conn)
	body := map[string]any{
		"app":  map[string]any{"appid": s.p.cfg.AppKey, "token": s.p.cfg.AccessKey, "cluster": s.p.cfg.ResourceID},
		"user": map[string]any{"uid": "aranea"},
		"audio": map[string]any{
			"voice_type":  s.sc.Voice,
			"encoding":    "pcm",
			"speed_ratio": s.sc.SpeedRatio,
			"rate":        s.sc.SampleRate,
		},
		"request": map[string]any{"reqid": uuid.NewString(), "text": text, "operation": "submit"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: raw}, true)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil
	}
	go s.readPump(conn)
	return nil
}

func (s *volcTTSSession) Audio() <-chan biz.TTSAudioChunk { return s.audio }

func (s *volcTTSSession) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.ended.Store(true)
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *volcTTSSession) setConn(c wsConn) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.conn = c
}

func (s *volcTTSSession) failOnce(err error) {
	if s.ended.CompareAndSwap(false, true) {
		s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: err}
		close(s.audio)
	}
}

func (s *volcTTSSession) finish() {
	if s.ended.CompareAndSwap(false, true) {
		s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkEnd}
	}
}

func (s *volcTTSSession) readPump(conn wsConn) {
	defer func() {
		s.ended.Store(true)
		close(s.audio)
		_ = conn.Close()
	}()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if !s.ended.Load() {
				s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: apierror.Wrap(err, apierror.CodeUnavailable, "speech")}
			}
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		if err != nil {
			s.p.lg.Warn("volc tts: undecodable frame", loggateway.Err(err))
			continue
		}
		switch f.msgType {
		case volcMsgAudioOnlyResponse:
			if len(f.payload) > 0 {
				s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkData, PCM: pcmS16ToF32(f.payload)}
			}
			if f.flags == volcFlagLastPackage {
				s.finish()
			}
		case volcMsgFullServerResponse:
			if f.flags == volcFlagLastPackage {
				s.finish()
			}
		case volcMsgError:
			s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: apierror.Internal("speech", "volc tts error: %s", string(f.payload))}
			return
		}
	}
}

// pcmS16ToF32 将火山 PCM s16le 转换为 biz 端口契约的 f32le。
func pcmS16ToF32(in []byte) []byte {
	n := len(in) / 2
	out := make([]byte, n*4)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(in[i*2:]))
		f := float32(v) / 32768.0
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

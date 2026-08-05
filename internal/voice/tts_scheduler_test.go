package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// fakeTTSSession 按脚本产出音频 chunk；blockCh 非空时 Audio() 阻塞直到其关闭。
type fakeTTSSession struct {
	chunks   []biz.TTSAudioChunk
	writeErr error
	blockCh  chan struct{}

	writeMu sync.Mutex
	writes  []string
	closed  chan struct{}
}

func (f *fakeTTSSession) Write(text string, _ bool) error {
	f.writeMu.Lock()
	f.writes = append(f.writes, text)
	f.writeMu.Unlock()
	return f.writeErr
}

func (f *fakeTTSSession) Audio() <-chan biz.TTSAudioChunk {
	out := make(chan biz.TTSAudioChunk, 8)
	go func() {
		defer close(out)
		if f.blockCh != nil {
			<-f.blockCh
			return
		}
		for _, c := range f.chunks {
			out <- c
		}
	}()
	return out
}

func (f *fakeTTSSession) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

type fakeTTSProvider struct {
	mu       sync.Mutex
	sessions []*fakeTTSSession
	script   func() *fakeTTSSession
}

func (p *fakeTTSProvider) Open(_ context.Context, _ biz.TTSSessionConfig) (biz.TTSSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var s *fakeTTSSession
	if p.script != nil {
		s = p.script()
	} else {
		s = &fakeTTSSession{
			chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1, 2, 3, 4}}, {Type: biz.TTSAudioChunkEnd}},
			closed: make(chan struct{}),
		}
	}
	p.sessions = append(p.sessions, s)
	return s, nil
}

type schedulerProbe struct {
	mu      sync.Mutex
	audios  [][]byte
	drained int
	errs    []error
}

func (p *schedulerProbe) opts(prov biz.StreamingTTSProvider) TTSSchedulerOpts {
	return TTSSchedulerOpts{
		Provider:  prov,
		Config:    biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000},
		OnAudio:   func(pcm []byte) { p.mu.Lock(); p.audios = append(p.audios, pcm); p.mu.Unlock() },
		OnDrained: func() { p.mu.Lock(); p.drained++; p.mu.Unlock() },
		OnError:   func(err error) { p.mu.Lock(); p.errs = append(p.errs, err); p.mu.Unlock() },
		LG:        loggateway.NewNoop(),
	}
}

func TestSchedulerOrderAndDrained(t *testing.T) {
	prov := &fakeTTSProvider{}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	require.NoError(t, s.Enqueue(ctx, "第一句。", false))
	require.NoError(t, s.Enqueue(ctx, "第二句。", false))
	require.NoError(t, s.Enqueue(ctx, "尾句。", true))

	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return probe.drained == 1 && len(probe.audios) == 3
	}, 2*time.Second, 10*time.Millisecond)

	prov.mu.Lock()
	defer prov.mu.Unlock()
	require.Len(t, prov.sessions, 3) // V1 每句一条连接
	require.Equal(t, []string{"第一句。", "第二句。", "尾句。"}, []string{prov.sessions[0].writes[0], prov.sessions[1].writes[0], prov.sessions[2].writes[0]})
}

func TestSchedulerBackpressure(t *testing.T) {
	block := make(chan struct{})
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{blockCh: block, closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer func() { close(block); s.Cancel() }()

	// worker 取走 1 句阻塞在合成上，队列再填满 8 句后第 10 句阻塞（背压）。
	for i := 0; i < ttsQueueCap+1; i++ {
		require.NoError(t, s.Enqueue(ctx, "句", false))
	}
	done := make(chan error, 1)
	go func() { done <- s.Enqueue(ctx, "溢出句", false) }()
	select {
	case <-done:
		t.Fatal("Enqueue should block when queue is full")
	case <-time.After(150 * time.Millisecond):
	}
	// Cancel 后阻塞的 Enqueue 经 ctx 解除
	s.Cancel()
	require.Error(t, <-done)
}

func TestSchedulerSkipFailedSentence(t *testing.T) {
	var failNext bool
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		if failNext {
			failNext = false
			return &fakeTTSSession{writeErr: errors.New("tts boom"), closed: make(chan struct{})}
		}
		return &fakeTTSSession{
			chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{9}}, {Type: biz.TTSAudioChunkEnd}},
			closed: make(chan struct{}),
		}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	failNext = true
	require.NoError(t, s.Enqueue(ctx, "坏句。", false))
	require.NoError(t, s.Enqueue(ctx, "好句。", true))
	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return probe.drained == 1 && len(probe.audios) == 1 && len(probe.errs) == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerAbortsAfterConsecutiveFailures(t *testing.T) {
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{writeErr: errors.New("boom"), closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	for i := 0; i < ttsMaxConsecutiveFailures; i++ {
		_ = s.Enqueue(ctx, "坏句", false)
	}
	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return len(probe.errs) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerCancelClosesCurrentSession(t *testing.T) {
	block := make(chan struct{})
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{blockCh: block, closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)

	require.NoError(t, s.Enqueue(ctx, "句", false))
	require.Eventually(t, func() bool {
		prov.mu.Lock()
		defer prov.mu.Unlock()
		return len(prov.sessions) == 1
	}, 2*time.Second, 10*time.Millisecond)

	s.Cancel()
	close(block)
	select {
	case <-prov.sessions[0].closed:
	case <-time.After(2 * time.Second):
		t.Fatal("current TTS session not closed on Cancel")
	}
}

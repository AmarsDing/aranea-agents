package voice

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	ttsQueueCap               = 8
	ttsMaxConsecutiveFailures = 3
)

// ttsSentenceTimeout 句子级空闲超时：任一 chunk（Data/End/Error）到达即重置。
// 挂死句（火山 End 帧永不到达/连接半死）在超时后被放弃，worker 继续后续句，
// OnDrained 必达（2026-08-09 天气 Turn：worker 挂句上 3 分钟致 tts.end 缺失）。
// 包级变量以便测试缩短。
var ttsSentenceTimeout = 15 * time.Second

// errTTSSentenceTimeout 句子级合成空闲超时（End 帧未达/连接半死）。
var errTTSSentenceTimeout = errors.New("voice: tts sentence idle timeout")

type sentenceJob struct {
	text  string
	flush bool
}

// TTSSchedulerOpts 配置 TTS 调度器。回调均由 worker goroutine 触发。
type TTSSchedulerOpts struct {
	Provider  biz.StreamingTTSProvider
	Config    biz.TTSSessionConfig
	OnAudio   func(pcm []byte) // f32le 16k 音频 chunk
	OnDrained func()           // flush 尾句合成完毕（Turn 级播报结束）
	OnError   func(err error)  // 连续失败中止（K3 降级由会话层处理）
	LG        loggateway.Logger
}

// TTSScheduler 按句调度 TTS 合成（设计 §4.2）。V1 每句一条连接。
type TTSScheduler struct {
	opts  TTSSchedulerOpts
	lg    loggateway.Logger
	queue chan sentenceJob

	cancel context.CancelFunc
	wg     sync.WaitGroup
	done   chan struct{} // Cancel 时关闭，解除阻塞的 Enqueue

	mu      sync.Mutex
	current biz.TTSSession
	stopped bool
}

func NewTTSScheduler(opts TTSSchedulerOpts) *TTSScheduler {
	lg := opts.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TTSScheduler{opts: opts, lg: lg, queue: make(chan sentenceJob, ttsQueueCap), done: make(chan struct{})}
}

func (s *TTSScheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.lg.Info("voice tts scheduler worker started (K7)",
			loggateway.StepID("voice.tts.worker_start"))
		defer func() {
			if r := recover(); r != nil {
				s.lg.Error("voice tts scheduler worker panic recovered (K7)",
					loggateway.StepID("voice.tts.worker_panic"),
					loggateway.Any("panic", r))
			}
			s.lg.Info("voice tts scheduler worker exited (K7)",
				loggateway.StepID("voice.tts.worker_exit"))
		}()
		s.loop(ctx)
	}()
}

// Enqueue 入队一句；队列满时阻塞（背压），ctx 取消时返回错误。
func (s *TTSScheduler) Enqueue(ctx context.Context, text string, flush bool) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("voice: tts scheduler stopped")
	}
	s.mu.Unlock()
	select {
	case s.queue <- sentenceJob{text: text, flush: flush}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errors.New("voice: tts scheduler stopped")
	}
}

// Cancel 停止调度：关闭当前合成会话、停 worker、解除阻塞的 Enqueue。幂等。
func (s *TTSScheduler) Cancel() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.done)
	s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.closeCurrent()
	s.wg.Wait()
}

func (s *TTSScheduler) loop(ctx context.Context) {
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			if job.text == "" {
				// 空文本 flush 哨兵（Turn 尾句已在句边界切出时由会话层补入）：
				// 不触达 provider，仅按序驱动 OnDrained。
				if job.flush && s.opts.OnDrained != nil {
					s.opts.OnDrained()
				}
				continue
			}
			err := s.synthesize(ctx, job)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				failures++
				s.lg.Warn("voice tts: sentence skipped (K3)",
					loggateway.StepID("voice.tts.sentence_skip"),
					loggateway.Err(err),
					loggateway.Any("consecutive_failures", failures))
				if failures >= ttsMaxConsecutiveFailures {
					if s.opts.OnError != nil {
						s.opts.OnError(apierror.Unavailable("speech", "tts provider failing repeatedly"))
					}
					return
				}
				continue
			}
			failures = 0
			if job.flush && s.opts.OnDrained != nil {
				s.opts.OnDrained()
			}
		}
	}
}

// synthesize 合成单句（V1：每句一条连接）。Data chunk 透传 OnAudio；
// 句级 End 内部消化（Turn 级结束由 flush 任务触发 OnDrained）。
func (s *TTSScheduler) synthesize(ctx context.Context, job sentenceJob) error {
	sess, err := s.opts.Provider.Open(ctx, s.opts.Config)
	if err != nil {
		return err
	}
	s.setCurrent(sess)
	defer func() {
		_ = sess.Close()
		s.setCurrent(nil)
	}()
	if err := sess.Write(job.text, true); err != nil {
		return err
	}
	audioCh := sess.Audio()
	idle := time.NewTimer(ttsSentenceTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idle.C:
			return errTTSSentenceTimeout
		case chunk, ok := <-audioCh:
			if !ok {
				return nil
			}
			// 有活动即重置空闲计时：长句持续流式输出不会被误杀。
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(ttsSentenceTimeout)
			switch chunk.Type {
			case biz.TTSAudioChunkData:
				if s.opts.OnAudio != nil {
					s.opts.OnAudio(chunk.PCM)
				}
			case biz.TTSAudioChunkError:
				return chunk.Err
			case biz.TTSAudioChunkEnd:
				// 句级合成完成即返回（V1 一句一连接：Write 一次 ↔ 一次 SessionFinished）。
				// 真机校准（2026-08-08）：火山在 152 后保持 WS 连接，audio 信道不会
				// 随即关闭；继续等待会把 worker 饿死在本句，后续句子与 OnDrained
				// （tts.end）全部阻塞。Turn 级结束仍由 flush 任务触发 OnDrained。
				return nil
			}
		}
	}
}

func (s *TTSScheduler) setCurrent(sess biz.TTSSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = sess
}

func (s *TTSScheduler) closeCurrent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		_ = s.current.Close()
	}
}

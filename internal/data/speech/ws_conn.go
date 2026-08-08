package speech

import (
	"context"
	"fmt"
	"io"
	nethttp "net/http"

	"github.com/gorilla/websocket"
)

// wsConn 抽象 *websocket.Conn，使 ASR/TTS 适配器可用 fake conn 做契约测试。
type wsConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

type wsDialer func(ctx context.Context, url string, header nethttp.Header) (wsConn, error)

// setVolcAuthHeader 按凭据模式写入火山鉴权头（ASR/TTS 共用）：
// APIKey 非空 → X-Api-Key（火山控制台新 API Key 模式）；否则 legacy
// X-Api-App-Key/X-Api-Access-Key 对。凭据值禁止入日志（DB-N8 同语义）。
func setVolcAuthHeader(header nethttp.Header, apiKey, appKey, accessKey string) {
	if apiKey != "" {
		header.Set("X-Api-Key", apiKey)
		return
	}
	header.Set("X-Api-App-Key", appKey)
	header.Set("X-Api-Access-Key", accessKey)
}

func gorillaDialer(ctx context.Context, url string, header nethttp.Header) (wsConn, error) {
	c, resp, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		// 握手失败时 gorilla 只报 bad handshake；带上上游 HTTP 状态与响应摘要，
		// 便于区分凭据拒绝（401/403）与协议/参数错误（400）。（真机排障教训）
		if resp != nil {
			detail := ""
			if resp.Body != nil {
				if b, rerr := io.ReadAll(io.LimitReader(resp.Body, 256)); rerr == nil && len(b) > 0 {
					detail = fmt.Sprintf(", body=%s", string(b))
				}
				_ = resp.Body.Close()
			}
			return nil, fmt.Errorf("%w (upstream HTTP %s%s)", err, resp.Status, detail)
		}
		return nil, err
	}
	return c, nil
}

package speech

import (
	"context"
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

func gorillaDialer(ctx context.Context, url string, header nethttp.Header) (wsConn, error) {
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, err
	}
	return c, nil
}

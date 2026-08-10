package speech

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// 火山 SAUC WebSocket 二进制协议帧编解码（ASR/TTS 适配器共用）。
// 布局见设计文档引用的火山公开协议；字节级真机校验归 V1-T10。

type volcMsgType = byte

const (
	volcMsgFullClientRequest  volcMsgType = 0x1
	volcMsgAudioOnlyRequest   volcMsgType = 0x2
	volcMsgFullServerResponse volcMsgType = 0x9
	volcMsgAudioOnlyResponse  volcMsgType = 0xB
	volcMsgError              volcMsgType = 0xF
)

const (
	volcFlagNone        byte = 0x0
	volcFlagPositiveSeq byte = 0x1
	volcFlagNegativeSeq byte = 0x3
	// volcFlagWithEvent 标记帧头后携带 event number（TTS V3 单向流式下行帧，
	// 布局：event(4B) + sessionID len(4B) + sessionID + payload len(4B) + payload）。
	volcFlagWithEvent byte = 0x4
)

type volcFrame struct {
	msgType volcMsgType
	flags   byte
	seq     int32
	// event：flags 含 volcFlagWithEvent 时有效（TTS V3：350/351/352/152 等）。
	event int32
	// sessionID：TTS V3 event 帧携带的服务端会话 ID（编解码对称保留）。
	sessionID string
	// errCode：msgType==volcMsgError 时的 4 字节错误码（位于 payload 长度之前）。
	errCode uint32
	json    bool // serialization=JSON（可 gzip）；false=裸字节不压缩
	payload []byte
}

func marshalVolcFrame(f volcFrame, gzipJSON bool) ([]byte, error) {
	payload := f.payload
	compression := byte(0x0)
	serialization := byte(0x0)
	if f.json {
		serialization = 0x1
		if gzipJSON {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			if _, err := zw.Write(payload); err != nil {
				return nil, err
			}
			if err := zw.Close(); err != nil {
				return nil, err
			}
			payload = buf.Bytes()
			compression = 0x1
		}
	}
	withSeq := f.flags == volcFlagPositiveSeq || f.flags == volcFlagNegativeSeq
	withEvent := f.flags&volcFlagWithEvent != 0
	isError := f.msgType == volcMsgError
	out := make([]byte, 0, 4+len(payload)+16+len(f.sessionID))
	out = append(out, 0x11, (f.msgType<<4)|(f.flags&0x0F), (serialization<<4)|compression, 0x00)
	var b [4]byte
	switch {
	case isError:
		binary.BigEndian.PutUint32(b[:], f.errCode)
		out = append(out, b[:]...)
	case withEvent:
		binary.BigEndian.PutUint32(b[:], uint32(f.event))
		out = append(out, b[:]...)
		binary.BigEndian.PutUint32(b[:], uint32(len(f.sessionID)))
		out = append(out, b[:]...)
		out = append(out, f.sessionID...)
	case withSeq:
		binary.BigEndian.PutUint32(b[:], uint32(f.seq))
		out = append(out, b[:]...)
	}
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(len(payload)))
	out = append(out, sz[:]...)
	out = append(out, payload...)
	return out, nil
}

func unmarshalVolcFrame(r io.Reader) (volcFrame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return volcFrame{}, err
	}
	if hdr[0]>>4 != 1 {
		return volcFrame{}, fmt.Errorf("unsupported protocol version %d", hdr[0]>>4)
	}
	headerSize := int(hdr[0] & 0x0F)
	f := volcFrame{
		msgType: hdr[1] >> 4,
		flags:   hdr[1] & 0x0F,
		json:    hdr[2]>>4 == 0x1,
	}
	gzipped := hdr[2]&0x0F == 0x1
	if headerSize > 1 { // 跳过额外头字节
		if _, err := io.CopyN(io.Discard, r, int64(4*(headerSize-1))); err != nil {
			return volcFrame{}, err
		}
	}
	switch {
	case f.msgType == volcMsgError:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return volcFrame{}, err
		}
		f.errCode = binary.BigEndian.Uint32(b[:])
	case f.flags&volcFlagWithEvent != 0:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return volcFrame{}, err
		}
		f.event = int32(binary.BigEndian.Uint32(b[:]))
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return volcFrame{}, err
		}
		sidLen := binary.BigEndian.Uint32(b[:])
		if sidLen > 1<<20 { // 1MB 防护（session id 实际几十字节）
			return volcFrame{}, fmt.Errorf("session id too large: %d", sidLen)
		}
		if sidLen > 0 {
			sid := make([]byte, sidLen)
			if _, err := io.ReadFull(r, sid); err != nil {
				return volcFrame{}, err
			}
			f.sessionID = string(sid)
		}
	case f.flags == volcFlagPositiveSeq || f.flags == volcFlagNegativeSeq:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return volcFrame{}, err
		}
		f.seq = int32(binary.BigEndian.Uint32(b[:]))
	}
	var szb [4]byte
	if _, err := io.ReadFull(r, szb[:]); err != nil {
		return volcFrame{}, err
	}
	size := binary.BigEndian.Uint32(szb[:])
	if size > 16<<20 { // 16MB 防护
		return volcFrame{}, fmt.Errorf("payload too large: %d", size)
	}
	f.payload = make([]byte, size)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return volcFrame{}, err
	}
	if gzipped {
		zr, err := gzip.NewReader(bytes.NewReader(f.payload))
		if err != nil {
			return volcFrame{}, err
		}
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		if err != nil {
			return volcFrame{}, err
		}
		f.payload = raw
	}
	return f, nil
}

// formatVolcError 提取错误帧的人类可读描述：优先 payload JSON 的
// code/message，缺省回退帧头 errCode + 原始 payload（ASR/TTS 共用）。
func formatVolcError(f volcFrame) string {
	var e struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(f.payload, &e); err == nil && (e.Code != 0 || e.Message != "") {
		return fmt.Sprintf("code %d: %s", e.Code, e.Message)
	}
	if f.errCode != 0 {
		return fmt.Sprintf("error code %d: %s", f.errCode, string(f.payload))
	}
	return string(f.payload)
}

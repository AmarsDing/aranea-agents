package speech

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
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
)

type volcFrame struct {
	msgType volcMsgType
	flags   byte
	seq     int32
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
	out := make([]byte, 0, 4+len(payload)+8)
	out = append(out, 0x11, (f.msgType<<4)|(f.flags&0x0F), (serialization<<4)|compression, 0x00)
	if withSeq {
		var b [4]byte
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
	if f.flags == volcFlagPositiveSeq || f.flags == volcFlagNegativeSeq {
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

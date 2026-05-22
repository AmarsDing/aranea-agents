package wechat

import (
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// TextInbound is a parsed WeChat official account text message.
type TextInbound struct {
	FromUser string
	ToUser   string
	Content  string
	MsgID    int64
}

type textMessageXML struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgID        int64    `xml:"MsgId"`
}

// VerifyURL handles GET echostr verification (passive mode).
func VerifyURL(token, timestamp, nonce, echostr, signature string) (string, error) {
	if !checkSignature(token, timestamp, nonce, signature) {
		return "", fmt.Errorf("wechat: bad signature")
	}
	echostr = strings.TrimSpace(echostr)
	if echostr == "" {
		return "", fmt.Errorf("wechat: empty echostr")
	}
	return echostr, nil
}

// ParseTextInbound decodes a text message POST body.
func ParseTextInbound(raw []byte) (*TextInbound, error) {
	var msg textMessageXML
	if err := xml.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(strings.ToLower(msg.MsgType)) != "text" {
		return nil, fmt.Errorf("wechat: unsupported msg type")
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		return nil, fmt.Errorf("wechat: empty content")
	}
	return &TextInbound{
		FromUser: strings.TrimSpace(msg.FromUserName),
		ToUser:   strings.TrimSpace(msg.ToUserName),
		Content:  text,
		MsgID:    msg.MsgID,
	}, nil
}

// VerifyPOST validates signature on inbound POST.
func VerifyPOST(token, timestamp, nonce, signature string) error {
	if checkSignature(token, timestamp, nonce, signature) {
		return nil
	}
	return fmt.Errorf("wechat: bad signature")
}

func checkSignature(token, timestamp, nonce, signature string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return true
	}
	parts := []string{token, strings.TrimSpace(timestamp), strings.TrimSpace(nonce)}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	got := fmt.Sprintf("%x", sum)
	return got == strings.TrimSpace(signature)
}

// ReadBody reads request body with size limit.
func ReadBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}

package office

// 邮件解析（邮件 P1 重写，设计 09-邮件.md §4「邮件加列 … body_html（入库时清洗）」）。
//
// 比原 parseMailRaw 多处理的：
//   - multipart 真递归（mixed 里套 alternative / related）；
//   - Content-Transfer-Encoding：base64 与 quoted-printable 都解；
//   - 字符集：GBK / GB2312 / GB18030 / Big5 等经 x/text 转成 UTF-8（正文与头部 encoded-word）；
//   - text/html 单独保留（清洗后入库），只有 HTML 时正文由 HTML 抽文字；
//   - To / Cc / Message-ID / References（回复串线）。

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"golang.org/x/text/encoding/htmlindex"
)

// mailMaxPartDepth multipart 嵌套上限（防恶意深嵌套）。
const mailMaxPartDepth = 12

func mailWordDecoder() *mime.WordDecoder {
	return &mime.WordDecoder{CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		enc, err := htmlindex.Get(charset)
		if err != nil {
			return nil, err
		}
		return enc.NewDecoder().Reader(input), nil
	}}
}

func decodeMailHeader(dec *mime.WordDecoder, v string) string {
	v = strings.TrimSpace(v)
	if out, err := dec.DecodeHeader(v); err == nil {
		return out
	}
	return v
}

// decodeMailCharset 按声明的字符集转 UTF-8；认不出或转失败时原样返回。
func decodeMailCharset(charset string, data []byte) string {
	cs := strings.ToLower(strings.Trim(strings.TrimSpace(charset), `"`))
	switch cs {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return string(data)
	}
	enc, err := htmlindex.Get(cs)
	if err != nil {
		return string(data)
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(out)
}

func decodeMailTransfer(cte string, data []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, string(data))
		if out, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return out
		}
		if out, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
			return out
		}
		return data
	case "quoted-printable":
		if out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err == nil {
			return out
		}
		return data
	}
	return data
}

// ParseMailRaw 解析一封 RFC822 原文：头部、纯文本正文、HTML（已清洗）、附件。
func ParseMailRaw(raw []byte) *MailImapMessage {
	msg := new(MailImapMessage)
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		msg.Body = string(raw)
		msg.Date = time.Now()
		return msg
	}
	dec := mailWordDecoder()
	h := m.Header
	msg.Subject = decodeMailHeader(dec, h.Get("Subject"))
	msg.Sender = decodeMailHeader(dec, h.Get("From"))
	msg.To = decodeMailHeader(dec, h.Get("To"))
	msg.Cc = decodeMailHeader(dec, h.Get("Cc"))
	msg.MessageID = strings.TrimSpace(h.Get("Message-Id"))
	msg.References = strings.Join(strings.Fields(h.Get("References")), " ")
	if msg.References == "" {
		msg.References = strings.TrimSpace(h.Get("In-Reply-To"))
	}
	if t, err := h.Date(); err == nil {
		msg.Date = t
	} else if t, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", strings.TrimSpace(h.Get("Date"))); err == nil {
		msg.Date = t
	} else {
		msg.Date = time.Now()
	}
	walkMailPart(dec, textproto.MIMEHeader(h), m.Body, msg, 0)
	msg.Body = strings.TrimSpace(msg.Body)
	if msg.Body == "" && msg.HTML != "" {
		msg.Body = mailHTMLToText(msg.HTML)
	}
	msg.HTML, msg.Attachments = inlineMailCIDImages(msg.HTML, msg.Attachments)
	msg.HTML = sanitizeMailHTML(msg.HTML)
	return msg
}

// mailInlineImageCap 单张内嵌图片转 data: 的上限；更大的留作附件。
const mailInlineImageCap = 2 << 20

// inlineMailCIDImages 把正文里 cid: 引用的内嵌图片换成 data: URI（清洗器只放行 data:image/ 和外链），
// 换进正文的图片不再算附件；正文没引用的照旧当附件。
func inlineMailCIDImages(html string, atts []MailImapAttachment) (string, []MailImapAttachment) {
	if html == "" || !strings.Contains(strings.ToLower(html), "cid:") {
		return html, atts
	}
	kept := atts[:0]
	for _, a := range atts {
		ref := "cid:" + a.ContentID
		if a.ContentID == "" || !strings.HasPrefix(a.ContentType, "image/") || len(a.Content) > mailInlineImageCap || !strings.Contains(html, ref) {
			kept = append(kept, a)
			continue
		}
		html = strings.ReplaceAll(html, ref, "data:"+a.ContentType+";base64,"+base64.StdEncoding.EncodeToString(a.Content))
	}
	return html, kept
}

func walkMailPart(dec *mime.WordDecoder, header textproto.MIMEHeader, r io.Reader, msg *MailImapMessage, depth int) {
	ct := header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType, params = "text/plain", map[string]string{}
	}
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mediaType, "multipart/") {
		if depth >= mailMaxPartDepth || params["boundary"] == "" {
			return
		}
		mr := multipart.NewReader(r, params["boundary"])
		for {
			part, err := mr.NextRawPart()
			if err != nil {
				return
			}
			walkMailPart(dec, part.Header, part, msg, depth+1)
		}
	}
	data, err := io.ReadAll(io.LimitReader(r, MailAttachmentMaxBytes+1))
	if err != nil {
		return
	}
	data = decodeMailTransfer(header.Get("Content-Transfer-Encoding"), data)

	disp, dparams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dparams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	filename = decodeMailHeader(dec, filename)
	isText := mediaType == "text/plain" || mediaType == "text/html"
	if strings.EqualFold(disp, "attachment") || filename != "" || mediaType == "message/rfc822" || !isText {
		if filename == "" {
			if mediaType == "message/rfc822" {
				filename = "message.eml"
			} else {
				filename = "attachment"
			}
		}
		msg.Attachments = append(msg.Attachments, MailImapAttachment{
			Filename: filename, ContentType: mediaType, Content: data,
			ContentID: strings.Trim(header.Get("Content-Id"), "<> "),
		})
		return
	}
	switch mediaType {
	case "text/plain":
		if msg.Body == "" {
			msg.Body = decodeMailCharset(params["charset"], data)
		}
	case "text/html":
		if msg.HTML == "" {
			msg.HTML = decodeMailCharset(params["charset"], data)
		}
	}
}

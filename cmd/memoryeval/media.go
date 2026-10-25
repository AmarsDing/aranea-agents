package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Multimodal intake limits, per the platform contract verified 2026-10-23:
// <=10 MiB decoded per image, <=30 MiB of image payload per Add request,
// JPEG/PNG/WebP only, inline base64 Data URI only (remote URLs rejected).
// All limits are env-overridable and default to the contract values; the
// whole media path is disabled (zero behavior drift for the textual run)
// unless EVAL_MEDIA_DIR is set.
const (
	defaultMaxImageBytes  int64 = 10 << 20
	defaultMaxImagesBytes int64 = 30 << 20
)

var errEvalImageTooLarge = errors.New("eval image exceeds size limit")

// evalImageMimeExt is the contract mime allowlist with the file extension
// used for the evidence copy.
var evalImageMimeExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// parseEvalDataURI validates and splits the inline image form
// data:<mime>[;params];base64,<payload>. Scheme, mediatype and the base64
// marker are case-insensitive per RFC 2397; mediatype parameters (e.g.
// charset) are ignored. Anything else (remote URLs, missing base64 marker,
// disallowed mime) is rejected without error detail: the caller logs and
// skips, never failing the request (FAQ Q23).
func parseEvalDataURI(raw string) (mime, payload string, ok bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < len("data:") || !strings.EqualFold(raw[:len("data:")], "data:") {
		return "", "", false
	}
	rest := raw[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma <= 0 {
		return "", "", false
	}
	head, payload := rest[:comma], rest[comma+1:]
	tokens := strings.Split(head, ";")
	if len(tokens) < 2 || !strings.EqualFold(strings.TrimSpace(tokens[len(tokens)-1]), "base64") {
		return "", "", false
	}
	mime = strings.ToLower(strings.TrimSpace(tokens[0]))
	if _, ok := evalImageMimeExt[mime]; !ok {
		return "", "", false
	}
	return mime, payload, true
}

// decodeEvalImagePayload tolerates both padded and raw (unpadded) base64.
// The encoded length is checked before decoding so an over-cap payload is
// rejected without allocating the decoded buffer (base64 inflates ~4/3).
func decodeEvalImagePayload(payload string, maxBytes int64) ([]byte, error) {
	if maxBytes > 0 && int64(len(payload)) > (maxBytes+2)/3*4+2 {
		return nil, errEvalImageTooLarge
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(payload)
	}
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(raw)) > maxBytes {
		return nil, errEvalImageTooLarge
	}
	return raw, nil
}

// saveEvalMediaPart writes the decoded image under
// <dir>/<sanitized userID>/<sanitized requestID>-m<msgIdx>-p<partIdx><ext>
// and returns the evidence path. Writes are create-exclusive: a replayed or
// duplicated Add for the same indexes keeps the first copy (idempotent
// evidence), never silently overwriting differing bytes. It is an audit
// copy for the caption-only route disclosure; failures must never fail Add.
func saveEvalMediaPart(dir, userID, requestID string, msgIndex, partIndex int, mime string, raw []byte) (string, error) {
	sub := filepath.Join(dir, sanitizeFileToken(userID))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-m%d-p%d%s",
		sanitizeFileToken(requestID), msgIndex, partIndex, evalImageMimeExt[mime])
	path := filepath.Join(sub, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return path, nil // Evidence for these indexes already exists; keep it.
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(raw); err != nil {
		return "", err
	}
	return path, nil
}

// sanitizeFileToken collapses a free-form identifier (user_id, request_id)
// into a single safe, length-bounded path segment: separators and dots
// (including dot-dot traversal) fold to underscores, and over-long ids are
// truncated with a short deterministic hash suffix so distinct ids cannot
// collide after truncation.
func sanitizeFileToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "anon"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			// Dots are folded too: ".." must never survive as a literal.
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) <= 96 {
		return out
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(out))
	return fmt.Sprintf("%.72s-%08x", out, h.Sum32())
}

package knowledge

import "context"

type documentContentVersionKey struct{}
type documentContentVersion struct{ id, hash string }

// WithDocumentContentVersion stamps the body snapshot used to build a document's chunks.
func WithDocumentContentVersion(ctx context.Context, id, body string) context.Context {
	return context.WithValue(ctx, documentContentVersionKey{}, documentContentVersion{id: id, hash: HashContent(body)})
}

// DocumentContentVersionFromContext returns an expectation only for the matching document.
func DocumentContentVersionFromContext(ctx context.Context, id string) (string, bool) {
	v, ok := ctx.Value(documentContentVersionKey{}).(documentContentVersion)
	if !ok || v.id != id {
		return "", false
	}
	return v.hash, true
}

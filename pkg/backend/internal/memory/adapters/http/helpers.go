package memoryhttp

import "strings"

type listResponse[T any] struct {
	Items []T `json:"items"`
}

func idFromPath(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

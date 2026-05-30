package modelregistry

import "context"

type StoreProvider interface {
	Store(ctx context.Context) (*Store, error)
}

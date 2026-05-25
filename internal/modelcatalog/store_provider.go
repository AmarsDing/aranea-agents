package modelcatalog

import "context"

// StoreProvider resolves the on-disk catalog store (supports dynamic root directory).
type StoreProvider interface {
	Store(ctx context.Context) (*Store, error)
}

// StaticStoreProvider wraps a fixed Store (tests / legacy).
type StaticStoreProvider struct {
	Fixed *Store
}

func (p StaticStoreProvider) Store(context.Context) (*Store, error) {
	return p.Fixed, nil
}

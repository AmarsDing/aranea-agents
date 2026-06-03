# Race Condition Fix Template

## Pattern
- `fatal error: concurrent map writes`
- `DATA RACE` in test output
- `sync: map is not thread-safe`

## Fix Strategy
1. Add `sync.RWMutex` or `sync.Mutex` to the struct containing the map
2. Wrap all map read/write operations with Lock/RLock
3. Consider using `sync.Map` for high-concurrency scenarios
4. For slices, use `sync/atomic` or channel-based patterns

## Example
```go
// Before (race condition)
type Store struct {
    items map[string]string
}

// After (thread-safe)
type Store struct {
    mu    sync.RWMutex
    items map[string]string
}

func (s *Store) Get(key string) string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.items[key]
}

func (s *Store) Set(key, value string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.items[key] = value
}
```

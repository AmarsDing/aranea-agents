# Nil Pointer Dereference Fix Template

## Pattern
- `panic: runtime error: invalid memory address or nil pointer dereference`
- `nil pointer dereference` in test output

## Fix Strategy
1. Add nil check before dereferencing
2. Return error instead of nil pointer
3. Use default/zero value as fallback

## Example
```go
// Before
func (s *Service) GetAgent(id string) *Agent {
    return s.repo.Find(id)
}

// After
func (s *Service) GetAgent(id string) (*Agent, error) {
    agent := s.repo.Find(id)
    if agent == nil {
        return nil, fmt.Errorf("agent not found: %s", id)
    }
    return agent, nil
}
```

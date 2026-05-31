package a2ui

import (
	"sync"
)

type SurfaceState struct {
	SurfaceID  string
	RootID     string
	Components map[string]Component
	DataModel  map[string]any
}

type SurfaceManager struct {
	mu       sync.RWMutex
	surfaces map[string]*SurfaceState
}

func NewSurfaceManager() *SurfaceManager {
	return &SurfaceManager{
		surfaces: make(map[string]*SurfaceState),
	}
}

func (m *SurfaceManager) BeginSurface(surfaceID, rootID string, styles *SurfaceStyles) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.surfaces[surfaceID] = &SurfaceState{
		SurfaceID:  surfaceID,
		RootID:     rootID,
		Components: make(map[string]Component),
		DataModel:  make(map[string]any),
	}
}

func (m *SurfaceManager) ApplySurfaceUpdate(update SurfaceUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.surfaces[update.SurfaceID]
	if !ok {
		s = &SurfaceState{
			SurfaceID:  update.SurfaceID,
			Components: make(map[string]Component),
			DataModel:  make(map[string]any),
		}
		m.surfaces[update.SurfaceID] = s
	}
	for _, comp := range update.Components {
		s.Components[comp.ID] = comp
	}
}

func (m *SurfaceManager) ApplyDataModelUpdate(update DataModelUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.surfaces[update.SurfaceID]
	if !ok {
		return
	}
	if update.Path == "" || update.Path == "/" {
		for _, entry := range update.Contents {
			s.DataModel[entry.Key] = dataEntryValue(entry)
		}
	} else {
		applyNestedPath(s.DataModel, update.Path, update.Contents)
	}
}

func (m *SurfaceManager) DeleteSurface(surfaceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.surfaces, surfaceID)
}

func (m *SurfaceManager) GetSurface(surfaceID string) (*SurfaceState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.surfaces[surfaceID]
	if !ok {
		return nil, false
	}
	cp := &SurfaceState{
		SurfaceID:  s.SurfaceID,
		RootID:     s.RootID,
		Components: make(map[string]Component, len(s.Components)),
		DataModel:  make(map[string]any, len(s.DataModel)),
	}
	for k, v := range s.Components {
		cp.Components[k] = v
	}
	for k, v := range s.DataModel {
		cp.DataModel[k] = v
	}
	return cp, true
}

func (m *SurfaceManager) ListSurfaces() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.surfaces))
	for id := range m.surfaces {
		ids = append(ids, id)
	}
	return ids
}

func dataEntryValue(e DataEntry) any {
	if e.ValueString != nil {
		return *e.ValueString
	}
	if e.ValueNumber != nil {
		return *e.ValueNumber
	}
	if e.ValueBoolean != nil {
		return *e.ValueBoolean
	}
	if len(e.ValueMap) > 0 {
		m := make(map[string]any, len(e.ValueMap))
		for _, child := range e.ValueMap {
			m[child.Key] = dataEntryValue(child)
		}
		return m
	}
	return nil
}

func applyNestedPath(data map[string]any, path string, entries []DataEntry) {
	parts := splitPath(path)
	current := data
	for i, p := range parts {
		if i == len(parts)-1 {
			for _, entry := range entries {
				if m, ok := current[p].(map[string]any); ok {
					m[entry.Key] = dataEntryValue(entry)
				} else {
					current[p] = map[string]any{entry.Key: dataEntryValue(entry)}
				}
			}
			return
		}
		next, ok := current[p].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[p] = next
		}
		current = next
	}
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	if path[0] == '/' {
		path = path[1:]
	}
	if path == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	parts = append(parts, path[start:])
	return parts
}

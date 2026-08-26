package codeexecutor

import "strings"

// Capability describes whether a code executor backend can be selected.
type Capability struct {
	Type      string
	Available bool
	Reason    string
}

// Capabilities reports availability for each supported backend without eagerly creating E2B sandboxes.
func (f *Factory) Capabilities() []Capability {
	out := make([]Capability, 0, len(ValidTypes()))
	for _, typ := range ValidTypes() {
		out = append(out, f.capabilityFor(typ))
	}
	return out
}

func (f *Factory) capabilityFor(typ string) Capability {
	switch typ {
	case TypeLocal:
		if ok, reason := LocalAvailable(); !ok {
			return Capability{Type: TypeLocal, Available: false, Reason: reason}
		}
		return Capability{Type: TypeLocal, Available: true}
	case TypeDocker:
		if DockerAvailable() {
			return Capability{Type: TypeDocker, Available: true}
		}
		return Capability{Type: TypeDocker, Available: false, Reason: "Docker daemon unavailable"}
	case TypeE2B:
		if strings.TrimSpace(f.env.E2BAPIKey) == "" {
			return Capability{Type: TypeE2B, Available: false, Reason: "E2B_API_KEY not set"}
		}
		return Capability{Type: TypeE2B, Available: true}
	case TypeContainer:
		if !ContainerBuildEnabled() {
			return Capability{Type: TypeContainer, Available: false, Reason: "requires build tag codeexec_container"}
		}
		return Capability{Type: TypeContainer, Available: true}
	case TypeSandbox:
		if f.sandboxAvailable() {
			return Capability{Type: TypeSandbox, Available: true}
		}
		return Capability{Type: TypeSandbox, Available: false, Reason: "sandbox pool unavailable (disabled or no docker daemon)"}
	default:
		return Capability{Type: typ, Available: false, Reason: "unsupported backend"}
	}
}

// IsBackendAvailable returns whether typ can be resolved without fallback.
func (f *Factory) IsBackendAvailable(typ string) bool {
	for _, c := range f.Capabilities() {
		if c.Type == typ {
			return c.Available
		}
	}
	return false
}

package event

import (
	"sync"

	"github.com/google/wire"
)

// Infra holds session vs monitor event buses (P0: isolate flow_log from chat envelopes).
type Infra struct {
	SessionBus Bus
	MonitorBus Bus
	Buffer     *Buffer
}

var (
	boundInfra   *Infra
	boundInfraMu sync.RWMutex
)

// BindInfra wires the process-wide event infra for monitor flow logs (replaces SetGlobalBus).
func BindInfra(infra *Infra) {
	boundInfraMu.Lock()
	boundInfra = infra
	boundInfraMu.Unlock()
}

func boundInfraRef() *Infra {
	boundInfraMu.RLock()
	defer boundInfraMu.RUnlock()
	return boundInfra
}

func monitorBusRef() Bus {
	if infra := boundInfraRef(); infra != nil {
		return infra.MonitorBus
	}
	return nil
}

// NewInfra wires dual buses for dependency injection.
func NewInfra() *Infra {
	return &Infra{
		SessionBus: NewBus(),
		MonitorBus: NewBus(),
		Buffer:     NewBuffer(),
	}
}

// ProvideSessionBus exposes the interactive/session bus for wire.
func ProvideSessionBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus()
	}
	return infra.SessionBus
}

// ProvideMonitorBus exposes the monitor/flow bus for wire.
func ProvideMonitorBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus()
	}
	return infra.MonitorBus
}

// ProvideBuffer exposes the session replay buffer for wire.
func ProvideBuffer(infra *Infra) *Buffer {
	if infra == nil {
		return NewBuffer()
	}
	return infra.Buffer
}

// InfraProviderSet is the wire set replacing standalone NewBus/NewBuffer.
// SessionBus is the default event.Bus binding; MonitorBus is accessed via *Infra (WS + flow logs).
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideSessionBus,
	ProvideBuffer,
)

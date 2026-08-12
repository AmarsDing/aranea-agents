package service

import (
	"testing"

	"aranea-agents/internal/event"
)

// 75 M1.4：computeruse.* stepID 前缀须映射到已注册 TraceDomainComputerUse，
// 而非默认回落 system。
func TestDomainForStepID_ComputerUse(t *testing.T) {
	if got := domainForStepID("computeruse.act"); got != event.TraceDomainComputerUse {
		t.Errorf("domainForStepID(computeruse.act) = %q, want %q", got, event.TraceDomainComputerUse)
	}
	if got := domainForStepID("computeruse.killswitch"); got != event.TraceDomainComputerUse {
		t.Errorf("domainForStepID(computeruse.killswitch) = %q, want %q", got, event.TraceDomainComputerUse)
	}
}

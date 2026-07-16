package biz

import (
	"context"
	"testing"
)

func TestChannelPeerUsecase_TryClaimInbound_NilRepoFailClosed(t *testing.T) {
	u := &ChannelPeerUsecase{}
	claimed, err := u.TryClaimInbound(context.Background(), "ch1", "line", "msg1", "peer1", "hi")
	if err == nil {
		t.Fatal("expected error when inbound receipt repo is nil")
	}
	if claimed {
		t.Fatal("expected claimed=false when repo is not configured")
	}
}

func TestChannelPeerUsecase_TryClaimInbound_NilUsecaseFailClosed(t *testing.T) {
	var u *ChannelPeerUsecase
	claimed, err := u.TryClaimInbound(context.Background(), "ch1", "line", "msg1", "peer1", "hi")
	if err == nil {
		t.Fatal("expected error when usecase is nil")
	}
	if claimed {
		t.Fatal("expected claimed=false when usecase is nil")
	}
}

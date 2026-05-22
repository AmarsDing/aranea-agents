package lark

import "testing"

func TestResolveReceiveTargetPrefersChatID(t *testing.T) {
	id, typ := ResolveReceiveTarget("ou_x", "u_x", "oc_x")
	if id != "oc_x" || typ != ReceiveIDTypeChatID {
		t.Fatalf("got %q %q", id, typ)
	}
}

func TestResolveReceiveTargetUserID(t *testing.T) {
	id, typ := ResolveReceiveTarget("", "u_x", "")
	if id != "u_x" || typ != ReceiveIDTypeUserID {
		t.Fatalf("got %q %q", id, typ)
	}
}

func TestReceiveIDTypeFromMeta(t *testing.T) {
	if ReceiveIDTypeFromMeta(map[string]string{"receive_id_type": "user_id"}) != ReceiveIDTypeUserID {
		t.Fatal("expected user_id")
	}
	if ReceiveIDTypeFromMeta(nil) != ReceiveIDTypeOpenID {
		t.Fatal("expected default open_id")
	}
}

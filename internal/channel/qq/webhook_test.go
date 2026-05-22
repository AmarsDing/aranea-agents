package qq

import (
	"net/http"
	"testing"

	"github.com/tencent-connect/botgo/interaction/signature"
)

func TestVerifyRequestUsesBotgoSignature(t *testing.T) {
	secret := "123456abcdef"
	body := []byte(`{"id":"ROBOT1.0_veoihSEXDc8Q.g-6eLpNIa11bH8MisOjn-m-LKxCPntMk6exUXgcWCGpVO7L2QKTNZzjZzFFDSbiOFcqAPWyVA!!","content":"哦一下","timestamp":"2024-10-15T16:33:15+08:00","author":{"id":"675860273","user_openid":"675860273"}}`)
	header := http.Header{}
	header.Set(signature.HeaderSig, "e949b5b94ef4103df903fb031d1d16e358db3db83e79e117edd404c8508be3ce8a76d7bad1bed353194c126a1a5915b4ad8b5288c1191cc53a12acffccd82004")
	header.Set(signature.HeaderTimestamp, "1728981195")
	if err := VerifyRequest(secret, header, body); err != nil {
		t.Fatal(err)
	}
	header.Set(signature.HeaderSig, "deadbeef")
	if err := VerifyRequest(secret, header, body); err == nil {
		t.Fatal("expected bad signature")
	}
}

func TestParseWebhookC2CMessage(t *testing.T) {
	body := []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","id":"evt1","d":{"id":"msg1","content":" hello ","author":{"id":"u1"}}}`)
	res, err := ParseWebhook(body, http.Header{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Message == nil || res.Message.Text != "hello" || res.Message.UserID != "u1" {
		t.Fatalf("%+v", res.Message)
	}
}

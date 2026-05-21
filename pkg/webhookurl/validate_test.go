package webhookurl

import "testing"

func TestValidateNotifyURL_blocksPrivate(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/hook",
		"http://localhost/hook",
		"http://10.0.0.1/hook",
		"ftp://example.com/hook",
		"not-a-url",
	}
	for _, u := range cases {
		if err := ValidateNotifyURL(u); err == nil {
			t.Fatalf("expected error for %q", u)
		}
	}
}

func TestValidateNotifyURL_allowsPublicHTTPS(t *testing.T) {
	if err := ValidateNotifyURL("https://example.com/webhook"); err != nil {
		t.Fatal(err)
	}
}

package shell_exec

import (
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestValidateCommand_allowsIPConfig(t *testing.T) {
	if err := validateCommand("ipconfig /all"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCommand_blocksRMRF(t *testing.T) {
	if err := validateCommand("rm -rf /"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeWindowsCMDBytes_GBK(t *testing.T) {
	enc := simplifiedchinese.GBK.NewEncoder()
	want := "不是内部或外部命令，也不是可运行的程序"
	gbk, err := enc.Bytes([]byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(gbk) {
		t.Fatal("test vector should not be UTF-8")
	}
	got := decodeWindowsCMDBytes(gbk)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDecodeWindowsCMDBytes_UTF8Passthrough(t *testing.T) {
	s := "hello 世界"
	if got := decodeWindowsCMDBytes([]byte(s)); got != s {
		t.Fatalf("got %q want %q", got, s)
	}
}

package shell_exec

import (
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// decodeWindowsCMDBytes maps cmd.exe / Windows console bytes to a Go UTF-8 string.
// Chinese Windows often emits936 (GBK) to stderr even when the process locale is UTF-8; reading that as UTF-8 yields mojibake.
func decodeWindowsCMDBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if utf8.Valid(b) {
		return string(b)
	}
	out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

func bytesToShellText(b []byte) string {
	if runtime.GOOS != "windows" {
		return string(b)
	}
	return decodeWindowsCMDBytes(b)
}

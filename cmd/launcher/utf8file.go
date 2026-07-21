package main

import (
	"os"
	"path/filepath"
)

// UTF-8 BOM so Windows Notepad / 记事本 open Chinese logs correctly
// (without BOM they often decode as system ANSI/GBK → mojibake).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func writeFileUTF8BOM(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(append([]byte{}, utf8BOM...), content...), 0o644)
}

// ensureLogBOM writes a BOM once when the log file is empty/new so appends stay UTF-8.
func ensureLogBOM(f *os.File) {
	if f == nil {
		return
	}
	st, err := f.Stat()
	if err != nil || st.Size() > 0 {
		return
	}
	_, _ = f.Write(utf8BOM)
}

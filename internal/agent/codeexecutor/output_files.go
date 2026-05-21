package codeexecutor

import (
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

const DefaultMaxOutputFileBytes = 10 << 20 // 10 MiB

// CollectOutputDirFiles walks dir and returns framework File entries for non-empty files.
func CollectOutputDirFiles(outDir string, maxBytes int64) []codeexecutor.File {
	if strings.TrimSpace(outDir) == "" {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputFileBytes
	}
	var files []codeexecutor.File
	_ = filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() == 0 || info.Size() > maxBytes {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		name := filepath.Base(path)
		mimeType := mime.TypeByExtension(filepath.Ext(name))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		files = append(files, codeexecutor.File{
			Name:      name,
			Content:   string(data),
			MIMEType:  mimeType,
			SizeBytes: info.Size(),
		})
		return nil
	})
	return files
}

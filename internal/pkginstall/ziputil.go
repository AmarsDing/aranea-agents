package pkginstall

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// zipDir creates a zip archive of srcDir at destPath.
func zipDir(srcDir, destPath string) error {
	// If destPath has a wildcard, create a proper temp file.
	if strings.Contains(destPath, "*") {
		f, err := os.CreateTemp(filepath.Dir(destPath), "skill-*.zip")
		if err != nil {
			return err
		}
		destPath = f.Name()
		f.Close()
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := w.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(f, src)
		return err
	})
}

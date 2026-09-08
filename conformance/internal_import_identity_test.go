package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoLegacyInternalGoImportPrefix(t *testing.T) {
	root := ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), `"a2ui/`) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			t.Errorf("legacy internal import prefix in %s", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RewriteModulePath replaces the Go module path in a CLI directory.
// It rewrites the module declaration in go.mod and all import paths
// in .go files from oldPath to newPath.
func RewriteModulePath(dir, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}

	// Rewrite go.mod module line
	gomodPath := filepath.Join(dir, "go.mod")
	gomod, err := os.ReadFile(gomodPath)
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}

	oldModule := "module " + oldPath
	newModule := "module " + newPath
	updated := strings.Replace(string(gomod), oldModule, newModule, 1)
	if updated == string(gomod) {
		return fmt.Errorf("go.mod does not contain expected module path %q", oldPath)
	}
	if err := os.WriteFile(gomodPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing go.mod: %w", err)
	}

	// Rewrite import paths in all .go files
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		replaced := strings.ReplaceAll(string(content), oldPath, newPath)
		if replaced == string(content) {
			return nil // no changes needed
		}

		return os.WriteFile(path, []byte(replaced), 0o644)
	})
}

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteModulePath(t *testing.T) {
	t.Parallel()

	t.Run("rewrites go.mod and go imports", func(t *testing.T) {
		dir := t.TempDir()

		// Write a go.mod with the old module path
		gomod := "module notion-pp-cli\n\ngo 1.23\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

		// Write a .go file with import paths
		goFile := `package main

import (
	"notion-pp-cli/internal/cli"
	"notion-pp-cli/internal/config"
)

func main() {}
`
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "notion-pp-cli"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd", "notion-pp-cli", "main.go"), []byte(goFile), 0o644))

		err := RewriteModulePath(dir, "notion-pp-cli", "github.com/mvanhorn/printing-press-library/library/productivity/notion-pp-cli")
		require.NoError(t, err)

		// Check go.mod
		updatedMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		require.NoError(t, err)
		assert.Contains(t, string(updatedMod), "module github.com/mvanhorn/printing-press-library/library/productivity/notion-pp-cli")
		assert.NotContains(t, string(updatedMod), "module notion-pp-cli\n")

		// Check .go file imports
		updatedGo, err := os.ReadFile(filepath.Join(dir, "cmd", "notion-pp-cli", "main.go"))
		require.NoError(t, err)
		assert.Contains(t, string(updatedGo), `"github.com/mvanhorn/printing-press-library/library/productivity/notion-pp-cli/internal/cli"`)
		assert.Contains(t, string(updatedGo), `"github.com/mvanhorn/printing-press-library/library/productivity/notion-pp-cli/internal/config"`)
	})

	t.Run("noop when paths are equal", func(t *testing.T) {
		dir := t.TempDir()
		gomod := "module notion-pp-cli\n\ngo 1.23\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

		err := RewriteModulePath(dir, "notion-pp-cli", "notion-pp-cli")
		require.NoError(t, err)
	})

	t.Run("error when go.mod missing old path", func(t *testing.T) {
		dir := t.TempDir()
		gomod := "module other-cli\n\ngo 1.23\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

		err := RewriteModulePath(dir, "notion-pp-cli", "github.com/org/repo/notion-pp-cli")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain expected module path")
	})
}

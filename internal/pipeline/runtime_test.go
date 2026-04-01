package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunVerify_CleansTransientArtifactsButKeepsCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "sample-cli"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "library", "sample-cli"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".cache", "go-build"), 0o755))

	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/sample-cli\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(dir, "cmd", "sample-cli", "main.go"), `package main
func main() {}
`)
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func initRoot() {
	rootCmd.AddCommand(newUsersListCmd())
}
`)
	writeTestFile(t, filepath.Join(dir, "cmd", "library", "sample-cli", "main.go"), "package recursive\n")
	writeTestFile(t, filepath.Join(dir, ".DS_Store"), "finder")
	writeTestFile(t, filepath.Join(dir, ".cache", "go-build", "index"), "cache")

	report, err := RunVerify(VerifyConfig{Dir: dir})
	require.NoError(t, err)
	assert.Equal(t, "PASS", report.Verdict)
	assert.FileExists(t, report.Binary)

	assert.FileExists(t, filepath.Join(dir, "sample-cli"))
	assert.NoDirExists(t, filepath.Join(dir, "cmd", "library"))
	assert.NoFileExists(t, filepath.Join(dir, ".DS_Store"))
	assert.DirExists(t, filepath.Join(dir, ".cache"))
}

func TestRunVerify_KeepsExistingBinaryWhenRebuildFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "sample-cli"), 0o755))

	existingBinary := filepath.Join(dir, "sample-cli")
	require.NoError(t, os.WriteFile(existingBinary, []byte("previous-build"), 0o755))
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/sample-cli\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(dir, "cmd", "sample-cli", "main.go"), `package main
func main() {
	this will not compile
}
`)

	report, err := RunVerify(VerifyConfig{Dir: dir})
	require.Error(t, err)
	assert.Nil(t, report)
	assert.FileExists(t, existingBinary)
}

func TestDiscoverCommands_UsesHelpOutputWhenBinaryAvailable(t *testing.T) {
	// Create a minimal CLI directory with root.go (for fallback path).
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func initRoot() {
	rootCmd.AddCommand(newIEconItems440Cmd())
	rootCmd.AddCommand(newPlayerCmd())
}
`)

	// Build a tiny binary that prints a fake --help with Available Commands.
	binDir := t.TempDir()
	mainFile := filepath.Join(binDir, "main.go")
	writeTestFile(t, mainFile, `package main

import "fmt"

func main() {
	fmt.Println("A test CLI")
	fmt.Println("")
	fmt.Println("Available Commands:")
	fmt.Println("  iecon-items-440  Get economy items for app 440")
	fmt.Println("  player           Get player info")
	fmt.Println("  completion       Generate completion script")
	fmt.Println("  help             Help about any command")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  -h, --help   help for test-cli")
}
`)
	binaryPath := filepath.Join(binDir, "test-cli")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, mainFile)
	out, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "building test binary: %s", string(out))

	commands := discoverCommands(dir, binaryPath)

	// Should use help output: iecon-items-440 (not camelToKebab's iecon-items440).
	assert.Len(t, commands, 2)
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name
	}
	assert.Contains(t, names, "iecon-items-440")
	assert.Contains(t, names, "player")
}

func TestDiscoverCommands_FallsBackToSourceWhenBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func initRoot() {
	rootCmd.AddCommand(newUsersListCmd())
	rootCmd.AddCommand(newProjectsGetCmd())
}
`)

	// Pass a non-existent binary path — should fall back to source parsing.
	commands := discoverCommands(dir, "/nonexistent/binary")

	assert.Len(t, commands, 2)
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name
	}
	assert.Contains(t, names, "users-list")
	assert.Contains(t, names, "projects-get")
}

func TestDiscoverCommands_FallsBackToSourceWhenBinaryPathEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func initRoot() {
	rootCmd.AddCommand(newUsersListCmd())
}
`)

	commands := discoverCommands(dir, "")
	assert.Len(t, commands, 1)
	assert.Equal(t, "users-list", commands[0].Name)
}

func TestParseHelpCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "standard cobra help output",
			input: `A CLI for Steam Web API

Available Commands:
  iecon-items-440  Get economy items for app 440
  player           Get player info
  completion       Generate completion script
  help             Help about any command

Flags:
  -h, --help   help for steam-pp-cli`,
			expected: []string{"iecon-items-440", "player"},
		},
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name: "no available commands section",
			input: `A CLI for something

Flags:
  -h, --help   help for something`,
			expected: nil,
		},
		{
			name: "single command",
			input: `Available Commands:
  users  Manage users

Flags:
  -h, --help   help`,
			expected: []string{"users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := parseHelpCommands(tt.input)
			if tt.expected == nil {
				assert.Empty(t, commands)
				return
			}
			names := make([]string, len(commands))
			for i, c := range commands {
				names[i] = c.Name
			}
			assert.Equal(t, tt.expected, names)
		})
	}
}

func TestBuildCLI_UsesCanonicalCommandDirForClaimedOutput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-pp-cli-2")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "sample-pp-cli"), 0o755))

	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/sample-pp-cli\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(dir, "cmd", "sample-pp-cli", "main.go"), `package main
func main() {}
`)

	binaryPath, err := buildCLI(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "sample-pp-cli-2"), binaryPath)
	assert.FileExists(t, binaryPath)
}

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishValidateMissingManifest(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--dir", cliDir, "--json"})

	output, err := runWithCapturedStdout(t, cmd.Execute)
	// Should fail with ExitPublishError
	require.Error(t, err)

	var result ValidateResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	assert.False(t, result.Passed)

	// Find the manifest check
	var manifestCheck *CheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "manifest" {
			manifestCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, manifestCheck)
	assert.False(t, manifestCheck.Passed)
	assert.Contains(t, manifestCheck.Error, "missing")
}

func TestPublishValidateManifestMissingFields(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	// Write a manifest missing required fields
	writeTestManifest(t, cliDir, pipeline.CLIManifest{SchemaVersion: 1})

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--dir", cliDir, "--json"})

	output, err := runWithCapturedStdout(t, cmd.Execute)
	require.Error(t, err)

	var result ValidateResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	assert.False(t, result.Passed)

	var manifestCheck *CheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "manifest" {
			manifestCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, manifestCheck)
	assert.False(t, manifestCheck.Passed)
	assert.Contains(t, manifestCheck.Error, "required fields")
}

func TestPublishValidateMissingDirFlag(t *testing.T) {
	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--json"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dir is required")
}

func TestPublishValidateManuscriptsWarnOnly(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	writeTestManifest(t, cliDir, pipeline.CLIManifest{
		SchemaVersion: 1,
		APIName:       "test",
		CLIName:       "test-pp-cli",
	})

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--dir", cliDir, "--json"})

	output, _ := runWithCapturedStdout(t, cmd.Execute)

	var result ValidateResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))

	// Find the manuscripts check
	var msCheck *CheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "manuscripts" {
			msCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, msCheck, "manuscripts check should always be present")
	// Manuscripts missing should be a warning, not a failure
	assert.True(t, msCheck.Passed, "manuscripts check should pass (warn-only)")
	assert.NotEmpty(t, msCheck.Warning, "should have a warning about missing manuscripts")
}

func TestPublishValidateJSONHasAllChecks(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	writeTestManifest(t, cliDir, pipeline.CLIManifest{
		SchemaVersion: 1,
		APIName:       "test",
		CLIName:       "test-pp-cli",
	})

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--dir", cliDir, "--json"})

	output, _ := runWithCapturedStdout(t, cmd.Execute)

	var result ValidateResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))

	// Should have all 7 check names
	checkNames := make(map[string]bool)
	for _, c := range result.Checks {
		checkNames[c.Name] = true
	}

	// All 7 checks should be present (they may fail in test env, but must exist)
	expectedChecks := []string{"manifest", "go mod tidy", "go vet", "go build", "--help", "--version", "manuscripts"}
	for _, name := range expectedChecks {
		assert.True(t, checkNames[name], "should have %q check", name)
	}
	assert.Len(t, result.Checks, 7, "should have exactly 7 checks")
}

func TestPublishValidateExitCode(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))
	// No manifest -> validation fails

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"validate", "--dir", cliDir, "--json"})

	_, err := runWithCapturedStdout(t, cmd.Execute)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, ExitPublishError, exitErr.Code, "should use ExitPublishError exit code")
}

func TestPublishPackageMissingDirFlag(t *testing.T) {
	cmd := newPublishCmd()
	cmd.SetArgs([]string{"package", "--json"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dir is required")
}

func TestPublishPackageMissingCategoryFlag(t *testing.T) {
	cmd := newPublishCmd()
	cmd.SetArgs([]string{"package", "--dir", "/tmp/fake", "--json"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--category is required")
}

func TestPublishPackageMissingTargetFlag(t *testing.T) {
	cmd := newPublishCmd()
	cmd.SetArgs([]string{"package", "--dir", "/tmp/fake", "--category", "ai", "--json"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")
}

func TestPublishPackageTargetExists(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	writeTestManifest(t, cliDir, pipeline.CLIManifest{
		SchemaVersion: 1,
		APIName:       "test",
		CLIName:       "test-pp-cli",
	})

	// Create target directory (already exists)
	target := filepath.Join(home, "staging")
	require.NoError(t, os.MkdirAll(target, 0o755))

	cmd := newPublishCmd()
	cmd.SetArgs([]string{"package", "--dir", cliDir, "--category", "developer-tools", "--target", target, "--json"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestPublishPackageCategoryPathTraversal(t *testing.T) {
	home := setLibraryTestEnv(t)
	cliDir := filepath.Join(home, "library", "test-pp-cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	writeTestManifest(t, cliDir, pipeline.CLIManifest{
		SchemaVersion: 1,
		APIName:       "test",
		CLIName:       "test-pp-cli",
	})

	tests := []struct {
		name     string
		category string
		wantErr  string
	}{
		{"dotdot traversal", "../../../escape", "simple slug"},
		{"forward slash", "foo/bar", "simple slug"},
		{"backslash", "foo\\bar", "simple slug"},
		{"dotdot only", "..", "simple slug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "staging")
			cmd := newPublishCmd()
			cmd.SetArgs([]string{"package", "--dir", cliDir, "--category", tt.category, "--target", target, "--json"})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestFindMostRecentRun(t *testing.T) {
	dir := t.TempDir()

	// Create run directories with timestamp-prefixed names and content
	for _, run := range []string{"20260327-100000", "20260328-132022", "20260326-090000"} {
		researchDir := filepath.Join(dir, run, "research")
		require.NoError(t, os.MkdirAll(researchDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(researchDir, "brief.md"), []byte("test"), 0o644))
	}

	runID, err := findMostRecentRun(dir)
	require.NoError(t, err)
	assert.Equal(t, "20260328-132022", runID, "should pick the most recent by lexicographic sort")
}

func TestFindMostRecentRunSkipsEmptyDirectories(t *testing.T) {
	dir := t.TempDir()

	// Most recent run is empty (interrupted archive)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "20260329-100000"), 0o755))

	// Older run has actual content
	researchDir := filepath.Join(dir, "20260328-132022", "research")
	require.NoError(t, os.MkdirAll(researchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(researchDir, "brief.md"), []byte("test"), 0o644))

	runID, err := findMostRecentRun(dir)
	require.NoError(t, err)
	assert.Equal(t, "20260328-132022", runID, "should skip empty run and use older one with content")
}

func TestFindMostRecentRunAllEmpty(t *testing.T) {
	dir := t.TempDir()

	// All runs are empty (no actual manuscript content)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "20260328-132022"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "20260327-100000"), 0o755))

	runID, err := findMostRecentRun(dir)
	require.NoError(t, err)
	assert.Empty(t, runID, "should return empty when all runs are empty directories")
}

func TestFindMostRecentRunEmpty(t *testing.T) {
	dir := t.TempDir()

	runID, err := findMostRecentRun(dir)
	require.NoError(t, err)
	assert.Empty(t, runID)
}

func TestFindMostRecentRunNonexistentDir(t *testing.T) {
	_, err := findMostRecentRun("/nonexistent/path")
	assert.Error(t, err)
}

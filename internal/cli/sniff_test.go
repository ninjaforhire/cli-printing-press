package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSniffCmdRejectsDomainMismatchOnAuthFrom(t *testing.T) {
	t.Parallel()

	cmd := newSniffCmd()
	outputPath := filepath.Join(t.TempDir(), "spec.yaml")
	cmd.SetArgs([]string{
		"--har", filepath.Join("..", "..", "testdata", "sniff", "sample-enriched.json"),
		"--auth-from", filepath.Join("..", "..", "testdata", "sniff", "sample-auth-capture-mismatch.json"),
		"--output", outputPath,
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.EqualError(t, err, "auth captured for other.example.com cannot be used with hn.algolia.com (domain mismatch)")
}

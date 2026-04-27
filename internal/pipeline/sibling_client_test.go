package pipeline

import (
	"strings"
	"testing"
)

// TestHasSiblingInternalImport_RecognizesAlgolia is the regression guard
// for hackernews retro #350 finding F4. A novel-feature command that
// imports a hand-built sibling internal package (e.g. internal/algolia)
// and calls into it must not trip the reimplementation false positive.
func TestHasSiblingInternalImport_RecognizesAlgolia(t *testing.T) {
	t.Parallel()

	content := `package cli

import (
	"hackernews-pp-cli/internal/algolia"
	"github.com/spf13/cobra"
)

func newPulseCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use: "pulse",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac := algolia.New(flags.timeout)
			_, err := ac.Search(args[0], algolia.SearchOpts{})
			return err
		},
	}
}
`
	if !hasSiblingInternalImport(content) {
		t.Fatalf("expected hasSiblingInternalImport to recognize internal/algolia")
	}
	if !hasClientSignal(content) {
		t.Fatalf("expected hasClientSignal to be true for algolia-importing command")
	}
}

// TestHasSiblingInternalImport_IgnoresReservedPackages verifies that
// imports of generator-emitted packages (client, store, cliutil, etc.)
// do NOT trip the new sibling-import signal. Those have their own
// signals (clientImportRe, storeImportRe) and shouldn't double-count.
func TestHasSiblingInternalImport_IgnoresReservedPackages(t *testing.T) {
	t.Parallel()

	for _, pkg := range []string{
		"client", "store", "cliutil", "cache", "config",
		"mcp", "types", "share", "deliver", "profile", "feedback",
		"graphql",
	} {
		content := `package cli

import (
	"my-cli/internal/` + pkg + `"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command { return nil }
`
		if hasSiblingInternalImport(content) {
			t.Errorf("reserved package %q should not trip sibling-import signal", pkg)
		}
	}
}

// TestHasSiblingInternalImport_RecognizesArbitraryNames verifies the
// signal fires for any non-reserved internal package name. Future CLIs
// might use internal/omdb, internal/recipescraper, etc. — the check is
// permissive about names because the alternative (maintaining a
// whitelist of known client-shaped names) doesn't scale.
func TestHasSiblingInternalImport_RecognizesArbitraryNames(t *testing.T) {
	t.Parallel()

	for _, pkg := range []string{
		"algolia", "omdb", "recipescraper", "tmdb",
		"mybank", "fcm", "snake_case_ok", "x9z",
	} {
		content := `package cli

import "my-cli/internal/` + pkg + `"
`
		if !hasSiblingInternalImport(content) {
			t.Errorf("non-reserved package %q should trip sibling-import signal", pkg)
		}
	}
}

// TestHasSiblingInternalImport_NoInternalImport verifies that files
// with no first-party internal/<name> import return false. We don't
// guard against third-party `<vendor>/.../internal/<x>` paths because
// Go's compiler enforces the internal rule — those paths can only
// appear in modules under the same module-path prefix, and never in
// the printed CLI's command files.
func TestHasSiblingInternalImport_NoInternalImport(t *testing.T) {
	t.Parallel()

	cases := []string{
		`package cli`,
		`package cli

import "github.com/spf13/cobra"
`,
		`package cli

import (
	"strings"
	"encoding/json"
	"github.com/spf13/cobra"
)
`,
	}
	for i, content := range cases {
		if hasSiblingInternalImport(content) {
			t.Errorf("case %d should not trip sibling-import signal", i)
		}
	}
}

// TestHasClientSignal_PreservesExistingSignals verifies the OR-with-new-
// regex didn't accidentally weaken the canonical client patterns.
func TestHasClientSignal_PreservesExistingSignals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "flags.newClient",
			content: `c, err := flags.newClient()`,
		},
		{
			name:    "internal/client import",
			content: `import "my-cli/internal/client"`,
		},
		{
			name:    "http.Get",
			content: `resp, err := http.Get("https://example.com")`,
		},
		{
			name:    "c.Get receiver call",
			content: `data, err := c.Get("/path", nil)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !hasClientSignal(tc.content) {
				t.Errorf("hasClientSignal should still match %q after adding sibling-import detection", tc.content)
			}
		})
	}
}

// TestHasSiblingInternalImport_TrivialBodyStillTripsReimplementation
// verifies the negative case: a command file that imports nothing from
// internal/<name> AND has a trivial body still trips reimplementation.
// The new sibling-import signal is additive — it doesn't loosen the
// trivial-body catch.
func TestHasSiblingInternalImport_TrivialBodyStillTripsReimplementation(t *testing.T) {
	t.Parallel()

	content := strings.TrimSpace(`
package cli

import "github.com/spf13/cobra"

func newStubCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stub",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
}
`)
	if hasClientSignal(content) {
		t.Fatalf("trivial-body command should NOT have a client signal")
	}
	if !trivialBodyRe.MatchString(content) {
		t.Fatalf("trivial-body regex should still match the canonical empty stub")
	}
}

package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSkillRendersFrontmatterAndCapabilities verifies that a generated
// SKILL.md carries the expected frontmatter fields and surfaces novel
// features as an inline "Unique Capabilities" block (not requiring agents
// to call --help for discovery).
func TestSkillRendersFrontmatterAndCapabilities(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("finance")
	apiSpec.Category = "commerce"
	outputDir := filepath.Join(t.TempDir(), "finance-pp-cli")
	gen := New(apiSpec, outputDir)
	gen.Narrative = &ReadmeNarrative{
		Headline:       "Quotes, charts, and a local portfolio nothing else has",
		ValueProp:      "Quotes, charts, fundamentals, options chains, and a SQLite-backed portfolio tracker.",
		WhenToUse:      "Reach for this CLI when an agent needs quotes, fundamentals, or persistent portfolio state against Yahoo Finance.",
		TriggerPhrases: []string{"quote AAPL", "check my portfolio", "options for TSLA"},
		Recipes: []Recipe{
			{Title: "Morning digest", Command: "finance-pp-cli digest --watchlist tech", Explanation: "Biggest movers across a named watchlist."},
		},
	}
	gen.NovelFeatures = []NovelFeature{
		{
			Command:      "portfolio perf",
			Description:  "Unrealized P&L across synced lots",
			Example:      "finance-pp-cli portfolio perf --agent",
			WhyItMatters: "Agents answer 'how's my portfolio' in one call",
			Group:        "Local state that compounds",
		},
	}
	require.NoError(t, gen.Generate())

	skill, err := os.ReadFile(filepath.Join(outputDir, "SKILL.md"))
	require.NoError(t, err)
	content := string(skill)

	// Frontmatter
	assert.True(t, strings.Contains(content, "name: pp-finance"),
		"frontmatter name should be pp-<api>")
	assert.True(t, strings.Contains(content, "Quotes, charts, and a local portfolio nothing else has"),
		"frontmatter description should incorporate headline")
	assert.True(t, strings.Contains(content, "'quote AAPL'"),
		"frontmatter description should list domain-specific trigger phrases verbatim")
	assert.True(t, strings.Contains(content, "library/commerce/finance-pp-cli"),
		"openclaw install manifest should use the API's category")

	// Body
	assert.True(t, strings.Contains(content, "## When to Use This CLI"),
		"WhenToUse narrative should render as its own section")
	assert.True(t, strings.Contains(content, "## Unique Capabilities"),
		"Novel features should appear as Unique Capabilities so agents don't need --help discovery")
	assert.True(t, strings.Contains(content, "### Local state that compounds"),
		"grouped novel features should render as subheadings in SKILL too")
	assert.True(t, strings.Contains(content, "finance-pp-cli portfolio perf --agent"),
		"novel-feature example should render as a copy-pasteable invocation")
	assert.True(t, strings.Contains(content, "_Agents answer 'how's my portfolio' in one call_"),
		"WhyItMatters should render as italic")

	// Command reference
	assert.True(t, strings.Contains(content, "**items** — Manage items"),
		"Command Reference should list resources inline so agents skip discovery")

	// Recipes
	assert.True(t, strings.Contains(content, "### Morning digest"),
		"Recipes should render as subsections with titles")
	assert.True(t, strings.Contains(content, "finance-pp-cli digest --watchlist tech"),
		"Recipes should include runnable commands")

	// Installation
	assert.True(t, strings.Contains(content, "## CLI Installation"),
		"SKILL should include CLI install instructions")
	assert.True(t, strings.Contains(content, "## MCP Server Installation"),
		"SKILL should include MCP install instructions")
	assert.True(t, strings.Contains(content, "| 7 | Rate limited"),
		"Exit codes table should render")
}

// TestSkillFallsBackWhenNarrativeAbsent asserts SKILL.md still renders a
// usable skill file when absorb data is missing — fallback uses .Description
// and the deterministic sections only.
func TestSkillFallsBackWhenNarrativeAbsent(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("bare")
	apiSpec.Description = "A basic API."
	outputDir := filepath.Join(t.TempDir(), "bare-pp-cli")
	gen := New(apiSpec, outputDir)
	require.NoError(t, gen.Generate())

	skill, err := os.ReadFile(filepath.Join(outputDir, "SKILL.md"))
	require.NoError(t, err)
	content := string(skill)

	assert.True(t, strings.Contains(content, "name: pp-bare"),
		"frontmatter still renders without narrative")
	assert.True(t, strings.Contains(content, "A basic API."),
		"description falls back to spec description")
	assert.False(t, strings.Contains(content, "## When to Use This CLI"),
		"WhenToUse section should be omitted when narrative is absent")
	assert.False(t, strings.Contains(content, "## Recipes"),
		"Recipes section should be omitted when narrative is absent")
	assert.True(t, strings.Contains(content, "## Auth Setup"),
		"Auth Setup always renders (falls back to auth-type branch)")
	assert.True(t, strings.Contains(content, "## Exit Codes"),
		"Exit codes always render")
	assert.True(t, strings.Contains(content, "## Command Reference"),
		"Command Reference always renders from the spec")
}

// TestSkillRendersAuthBranchPerType asserts the deterministic Auth Setup
// block branches correctly on .Auth.Type when no narrative auth is provided.
func TestSkillRendersAuthBranchPerType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		authType string
		expect   string
	}{
		{"api_key", "api_key", "export"},
		{"oauth2", "oauth2", "auth login"},
		{"bearer_token", "bearer_token", "auth set-token"},
		{"cookie", "cookie", "auth login --chrome"},
		{"none", "none", "No authentication required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			apiSpec := minimalSpec("auth" + tc.name)
			apiSpec.Auth = spec.AuthConfig{
				Type:    tc.authType,
				EnvVars: []string{"AUTH_KEY"},
			}
			outputDir := filepath.Join(t.TempDir(), "auth"+tc.name+"-pp-cli")
			gen := New(apiSpec, outputDir)
			require.NoError(t, gen.Generate())

			skill, err := os.ReadFile(filepath.Join(outputDir, "SKILL.md"))
			require.NoError(t, err)
			content := string(skill)

			assert.True(t, strings.Contains(content, tc.expect),
				"auth-type %q should produce %q in SKILL.md Auth Setup", tc.authType, tc.expect)
		})
	}
}

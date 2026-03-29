package pipeline

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// conventionalCommitPattern validates the format used in PR titles and
// commit messages. This pattern is mirrored by the PR title GitHub Action
// (.github/workflows/pr-title.yml) — keep them in sync.
//
// Scopes: cli, skills, ci (required on all types).
// Breaking changes: ! after scope.
var conventionalCommitPattern = regexp.MustCompile(
	`^(feat|fix|docs|chore|refactor|test|ci|perf|build|style|revert)` +
		`\((cli|skills|ci)\)` +
		`!?` +
		`: .+`)

func TestConventionalCommitPatternAcceptsValid(t *testing.T) {
	valid := []string{
		"feat(cli): add catalog subcommands",
		"fix(skills): remove repo checkout requirement",
		"feat(ci): add release-please",
		"feat(cli)!: rename catalog command to registry",
		"docs(cli): update version flag examples",
		"chore(ci): bump dependencies",
		"refactor(cli): extract helper function",
		"test(cli): add coverage for edge case",
		"fix(cli)!: breaking change with bang",
		"ci(ci): update workflow",
	}

	for _, msg := range valid {
		t.Run(msg, func(t *testing.T) {
			assert.Regexp(t, conventionalCommitPattern, msg)
		})
	}
}

func TestConventionalCommitPatternRejectsInvalid(t *testing.T) {
	invalid := []string{
		"Add new feature",
		"updated the readme",
		"WIP stuff",
		"FEAT(cli): wrong case",
		"feat:missing space",
		"feat(): empty scope",
		"feat: missing scope",
		"docs: missing scope",
		"feat(random): invalid scope",
		"fix(anything): invalid scope",
	}

	for _, msg := range invalid {
		t.Run(msg, func(t *testing.T) {
			assert.NotRegexp(t, conventionalCommitPattern, msg)
		})
	}
}

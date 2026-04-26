package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v2/internal/spec"
	"github.com/stretchr/testify/require"
)

// TestAuthOptional_DoctorReportsOptional verifies that a spec with
// `auth.optional: true` produces a doctor that emits "optional — not configured"
// (rendered as INFO by the indicator switch) instead of "not configured" (FAIL).
// A Grade-A CLI with a completely healthy doctor for its optional-auth state
// is now possible. Regression guard for #211.
func TestAuthOptional_DoctorReportsOptional(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("opt-auth")
	apiSpec.Auth.Optional = true

	outputDir := filepath.Join(t.TempDir(), "opt-auth-pp-cli")
	require.NoError(t, New(apiSpec, outputDir).Generate())

	doctorSrc, err := os.ReadFile(filepath.Join(outputDir, "internal", "cli", "doctor.go"))
	require.NoError(t, err)
	require.Contains(t, string(doctorSrc), `"optional — not configured"`,
		"doctor should emit optional-prefixed status when auth.optional is set")
}

// TestAuthNotOptional_DoctorReportsFailure guards the default branch: when
// auth.optional is unset, doctor emits the plain "not configured" string that
// renders as FAIL. Prevents over-broad application of the optional path.
func TestAuthNotOptional_DoctorReportsFailure(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("req-auth")
	// Optional left at zero-value false.

	outputDir := filepath.Join(t.TempDir(), "req-auth-pp-cli")
	require.NoError(t, New(apiSpec, outputDir).Generate())

	doctorSrc, err := os.ReadFile(filepath.Join(outputDir, "internal", "cli", "doctor.go"))
	require.NoError(t, err)
	require.Contains(t, string(doctorSrc), `"not configured"`,
		"doctor emits the standard not-configured message for required auth")
	require.NotContains(t, string(doctorSrc), `"optional — not configured"`,
		"default spec must not emit the optional-prefixed status")
}

// TestAuthOptional_AuthCmdShortNamesEnvVar verifies the `auth` subcommand's
// help Short description names the specific env var and flags the optionality
// when auth.optional is set.
func TestAuthOptional_AuthCmdShortNamesEnvVar(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("opt-auth-short")
	apiSpec.Auth.Optional = true
	apiSpec.Auth.EnvVars = []string{"OPT_AUTH_KEY"}

	outputDir := filepath.Join(t.TempDir(), "opt-auth-short-pp-cli")
	require.NoError(t, New(apiSpec, outputDir).Generate())

	authSrc, err := os.ReadFile(filepath.Join(outputDir, "internal", "cli", "auth.go"))
	require.NoError(t, err)
	require.Contains(t, string(authSrc), `"Manage the optional OPT_AUTH_KEY`,
		"auth parent command Short must name the env var and flag optionality")
}

// TestAuthRequired_AuthCmdShortNamesEnvVar verifies the default (required)
// branch still names the env var — just without the "optional" flag.
func TestAuthRequired_AuthCmdShortNamesEnvVar(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("req-auth-short")
	apiSpec.Auth.EnvVars = []string{"REQ_AUTH_KEY"}

	outputDir := filepath.Join(t.TempDir(), "req-auth-short-pp-cli")
	require.NoError(t, New(apiSpec, outputDir).Generate())

	authSrc, err := os.ReadFile(filepath.Join(outputDir, "internal", "cli", "auth.go"))
	require.NoError(t, err)
	require.Contains(t, string(authSrc), `"Manage REQ_AUTH_KEY credentials"`,
		"required-auth parent command Short names the env var without optional framing")
}

// TestAuthOptional_ReadmeFramesAsOptional verifies the README template
// uses "Optional: API Key" + the "all core commands work without setup"
// preamble when auth.optional is true and a narrative auth_narrative is set.
func TestAuthOptional_ReadmeFramesAsOptional(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("opt-auth-readme")
	apiSpec.Auth.Optional = true

	outputDir := filepath.Join(t.TempDir(), "opt-auth-readme-pp-cli")
	gen := New(apiSpec, outputDir)
	gen.Narrative = &ReadmeNarrative{
		AuthNarrative: "Use the OPTAUTH_KEY to unlock bonus features.",
	}
	require.NoError(t, gen.Generate())

	readme, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	require.NoError(t, err)
	body := string(readme)
	require.Contains(t, body, "## Optional: API Key",
		"README must use 'Optional: API Key' heading when auth.optional is set")
	require.Contains(t, body, "All core commands work without setup",
		"README must reassure users that core commands need no setup")
}

// TestAuthNotOptional_ReadmeKeepsAuthenticationHeader guards the default branch:
// when auth.optional is unset, README keeps the plain "## Authentication" heading.
func TestAuthNotOptional_ReadmeKeepsAuthenticationHeader(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("req-auth-readme")
	// Optional left false.

	outputDir := filepath.Join(t.TempDir(), "req-auth-readme-pp-cli")
	gen := New(apiSpec, outputDir)
	gen.Narrative = &ReadmeNarrative{
		AuthNarrative: "Export REQ_AUTH_KEY to access the API.",
	}
	require.NoError(t, gen.Generate())

	readme, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	require.NoError(t, err)
	body := string(readme)
	require.Contains(t, body, "## Authentication",
		"README must keep the Authentication heading for required auth")
	require.NotContains(t, body, "## Optional: API Key",
		"required-auth README must not use the optional framing")
	require.NotContains(t, body, "All core commands work without setup",
		"required-auth README must not claim core commands work without setup")
}

// Sanity check that my spec field round-trips. Not really testing anything
// new after the previous tests; here to make the schema intent explicit.
func TestAuthConfig_Optional_ZeroValue(t *testing.T) {
	a := spec.AuthConfig{}
	require.False(t, a.Optional, "Optional must default to false")
	a.Optional = true
	require.True(t, a.Optional)
}

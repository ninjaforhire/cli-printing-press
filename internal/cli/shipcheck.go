package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// shipcheck is the canonical Phase 4 verification umbrella. It runs each
// of the five legs as a subprocess of the same printing-press binary,
// aggregates exit codes, and prints a per-leg summary. Legs remain
// callable standalone — this command is purely additive orchestration.
//
// The subprocess model (rather than calling each leg's RunE in-process)
// gives us:
//   - real-time per-leg output streaming to the operator's terminal,
//   - reliable exit-code propagation through standard *exec.ExitError,
//   - testability via a stub binary that mimics the leg surface.
//
// The legs slice below is the single source of truth for which legs run
// and what argv each gets. Adding a leg = append one entry; the rest of
// the umbrella reads from the slice.

// shipcheckOpts holds every flag the umbrella accepts. Each leg's argv
// builder is a closure over an opts pointer, so adding a flag = adding
// a field here and consulting it from the relevant builder.
type shipcheckOpts struct {
	dir         string
	spec        string
	researchDir string
}

// shipcheckLeg names one verification leg and how to invoke it.
// args builds the leg's argv (without the binary path) from the umbrella's
// resolved options.
type shipcheckLeg struct {
	name string
	args func(*shipcheckOpts) []string
}

// shipcheckLegs enumerates the five legs in canonical execution order.
// Order matters: dogfood writes research.json updates that scorecard
// later consumes, so dogfood must run before scorecard. workflow-verify
// and verify-skill have no inter-leg dependencies; their position is
// driven by the canonical Phase 4 sequence in the /printing-press skill.
var shipcheckLegs = []shipcheckLeg{
	{
		name: "dogfood",
		args: func(o *shipcheckOpts) []string {
			a := []string{"dogfood", "--dir", o.dir}
			if o.spec != "" {
				a = append(a, "--spec", o.spec)
			}
			if o.researchDir != "" {
				a = append(a, "--research-dir", o.researchDir)
			}
			return a
		},
	},
	{
		name: "verify",
		args: func(o *shipcheckOpts) []string {
			a := []string{"verify", "--dir", o.dir}
			if o.spec != "" {
				a = append(a, "--spec", o.spec)
			}
			// --fix is on by default. U2 will add --no-fix to disable.
			a = append(a, "--fix")
			return a
		},
	},
	{
		name: "workflow-verify",
		args: func(o *shipcheckOpts) []string {
			return []string{"workflow-verify", "--dir", o.dir}
		},
	},
	{
		name: "verify-skill",
		args: func(o *shipcheckOpts) []string {
			return []string{"verify-skill", "--dir", o.dir}
		},
	},
	{
		name: "scorecard",
		args: func(o *shipcheckOpts) []string {
			a := []string{"scorecard", "--dir", o.dir}
			if o.researchDir != "" {
				a = append(a, "--research-dir", o.researchDir)
			}
			if o.spec != "" {
				a = append(a, "--spec", o.spec)
			}
			// --live-check is on by default. U2 will add --no-live-check.
			a = append(a, "--live-check")
			return a
		},
	},
}

// shipcheckLegResult is the per-leg outcome of one umbrella run.
type shipcheckLegResult struct {
	Name     string
	Argv     []string
	ExitCode int
	Elapsed  time.Duration
}

// Passed reports whether the leg exited 0.
func (r shipcheckLegResult) Passed() bool { return r.ExitCode == 0 }

// resolveSelfBinary returns the path to the currently-running
// printing-press binary so the umbrella can spawn itself for each leg.
//
// Indirected through a package-level var so tests can substitute a stub
// binary that mimics the leg surface. Production callers always go
// through os.Executable, which gives the actual running executable path
// and avoids any ambiguity from an outdated `printing-press` on $PATH.
var resolveSelfBinary = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving printing-press binary: %w", err)
	}
	// Resolve symlinks so a `printing-press` symlink to the real binary
	// still produces the canonical path subprocesses see.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// runShipcheckLeg spawns one leg as a subprocess, streaming its
// stdout/stderr to the operator's terminal, and captures the exit code.
//
// Returns ExitCode 0 on clean completion, the child's exit code on
// non-zero exit, and an error only when the subprocess could not be
// started (binary missing, permission denied, etc.). A non-zero exit
// from the child is reported via the result, not as an error — the
// umbrella always wants to record what happened and continue.
func runShipcheckLeg(binPath string, leg shipcheckLeg, opts *shipcheckOpts) (shipcheckLegResult, error) {
	argv := leg.args(opts)
	cmd := exec.Command(binPath, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	res := shipcheckLegResult{
		Name:    leg.name,
		Argv:    argv,
		Elapsed: elapsed,
	}
	if runErr == nil {
		res.ExitCode = 0
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	// Subprocess could not be started at all.
	return res, fmt.Errorf("running %s: %w", leg.name, runErr)
}

// renderShipcheckSummary prints a per-leg verdict table to w.
func renderShipcheckSummary(w *os.File, results []shipcheckLegResult) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Shipcheck Summary")
	fmt.Fprintln(w, "=================")
	fmt.Fprintf(w, "  %-16s  %-6s  %-8s  %s\n", "LEG", "RESULT", "EXIT", "ELAPSED")
	for _, r := range results {
		verdict := "PASS"
		if !r.Passed() {
			verdict = "FAIL"
		}
		fmt.Fprintf(w, "  %-16s  %-6s  %-8d  %s\n",
			r.Name,
			verdict,
			r.ExitCode,
			r.Elapsed.Round(time.Millisecond),
		)
	}
	failing := 0
	for _, r := range results {
		if !r.Passed() {
			failing++
		}
	}
	fmt.Fprintln(w, "")
	if failing == 0 {
		fmt.Fprintf(w, "Verdict: PASS (%d/%d legs passed)\n", len(results), len(results))
	} else {
		fmt.Fprintf(w, "Verdict: FAIL (%d/%d legs failed)\n", failing, len(results))
	}
}

// shipcheckUmbrellaCode returns the umbrella's overall exit code:
// 0 if every leg passed, otherwise the largest non-zero exit code
// among failing legs (preserves the most serious failure).
func shipcheckUmbrellaCode(results []shipcheckLegResult) int {
	max := 0
	for _, r := range results {
		if r.ExitCode > max {
			max = r.ExitCode
		}
	}
	return max
}

// validateShipcheckDir confirms --dir points at something that looks
// like a built printing-press CLI: a directory containing go.mod and
// either an internal/cli/ tree or a cmd/<name>-pp-cli/ tree. We are
// intentionally permissive — full structural checks are the legs' job.
func validateShipcheckDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("--dir is required")
	}
	st, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("--dir %q: %w", dir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("--dir %q is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return fmt.Errorf("--dir %q does not contain go.mod (is this a generated CLI directory?)", dir)
	}
	return nil
}

func newShipcheckCmd() *cobra.Command {
	opts := &shipcheckOpts{}

	cmd := &cobra.Command{
		Use:   "shipcheck",
		Short: "Run all five verification legs (dogfood, verify, workflow-verify, verify-skill, scorecard) as one canonical Phase 4 sweep",
		Long: `shipcheck runs every Phase 4 verification leg in sequence and aggregates their
exit codes into a single verdict. It is the canonical local invocation that
matches what the public-library CI runs.

Legs (in canonical order):
  dogfood          — structural validation against the source spec
  verify           — runtime command testing (with --fix to auto-repair common breakage)
  workflow-verify  — primary workflow end-to-end against the verification manifest
  verify-skill     — SKILL.md flag/positional/command consistency with the shipped CLI
  scorecard        — Steinberger quality bar (with --live-check sampling novel features)

Every leg streams its full output to the terminal as it runs; a per-leg verdict
table is printed at the end. The command exits non-zero when any leg fails,
with the exit code reflecting the most serious leg failure.

Each leg remains callable standalone — this command is additive orchestration.`,
		Example: `  # Canonical Phase 4 invocation
  printing-press shipcheck \
    --dir ~/printing-press/library/notion \
    --spec ./openapi.yaml \
    --research-dir ~/printing-press/.runstate/scope/runs/RUN_ID

  # Without a research dir (skips the dogfood/scorecard novel-feature checks)
  printing-press shipcheck --dir ~/printing-press/library/notion --spec ./openapi.yaml`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateShipcheckDir(opts.dir); err != nil {
				return &ExitError{Code: ExitInputError, Err: err}
			}

			binPath, err := resolveSelfBinary()
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err}
			}

			results := make([]shipcheckLegResult, 0, len(shipcheckLegs))
			for _, leg := range shipcheckLegs {
				fmt.Fprintf(os.Stdout, "\n=== %s ===\n", leg.name)
				res, runErr := runShipcheckLeg(binPath, leg, opts)
				if runErr != nil {
					// Subprocess failed to start. Record as a synthetic
					// failure, surface the error to stderr, and continue
					// — operators want a complete summary even if one
					// leg's binary went missing mid-run.
					fmt.Fprintf(os.Stderr, "shipcheck: %v\n", runErr)
					res.ExitCode = ExitUnknownError
				}
				results = append(results, res)
			}

			renderShipcheckSummary(os.Stdout, results)

			code := shipcheckUmbrellaCode(results)
			if code != 0 {
				failing := 0
				for _, r := range results {
					if !r.Passed() {
						failing++
					}
				}
				return &ExitError{
					Code:   code,
					Err:    fmt.Errorf("shipcheck failed: %d/%d legs failed", failing, len(results)),
					Silent: true,
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.dir, "dir", "", "Path to the generated CLI directory (required)")
	cmd.Flags().StringVar(&opts.spec, "spec", "", "Path to the OpenAPI spec file (passed to dogfood, verify, scorecard)")
	cmd.Flags().StringVar(&opts.researchDir, "research-dir", "", "Pipeline directory containing research.json (passed to dogfood and scorecard)")

	return cmd
}

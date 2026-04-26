package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v2/internal/pipeline"
	"github.com/spf13/cobra"
)

func newScorecardCmd() *cobra.Command {
	var dir string
	var specPath string
	var asJSON bool
	var liveCheck bool
	var liveCheckTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "Score a generated CLI against the Steinberger bar",
		Example: `  # Score a generated CLI directory
  printing-press scorecard --dir ./generated/stripe-pp-cli

  # Include a live behavioral sample (runs novel-feature examples against real targets)
  printing-press scorecard --dir ./generated/stripe-pp-cli --live-check

  # Output as JSON
  printing-press scorecard --dir ./generated/stripe-pp-cli --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("--dir is required")}
			}

			// Use a temp pipeline dir for the scorecard output
			pipelineDir, err := os.MkdirTemp("", "scorecard-*")
			if err != nil {
				return fmt.Errorf("creating temp dir: %w", err)
			}
			defer func() { _ = os.RemoveAll(pipelineDir) }()

			sc, err := pipeline.RunScorecard(dir, pipelineDir, specPath, nil)
			if err != nil {
				return &ExitError{Code: ExitGenerationError, Err: fmt.Errorf("running scorecard: %w", err)}
			}

			var live *pipeline.LiveCheckResult
			if liveCheck {
				live = pipeline.RunLiveCheck(pipeline.LiveCheckOptions{
					CLIDir:  dir,
					Timeout: liveCheckTimeout,
				})
				if insightCap := pipeline.InsightCapFromLiveCheck(live); insightCap != nil && sc.Steinberger.Insight > *insightCap {
					sc.Steinberger.Insight = *insightCap
				}
				pipeline.ApplyLiveCheckToScorecard(sc, live)
			}

			if asJSON {
				payload := map[string]any{"scorecard": sc}
				if live != nil {
					payload["live_check"] = live
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			// Human-readable output
			s := sc.Steinberger
			renderScore := func(name string, score, max int) string {
				if sc.IsDimensionUnscored(name) {
					return "N/A"
				}
				return fmt.Sprintf("%d/%d", score, max)
			}

			fmt.Printf("Quality Scorecard: %s\n\n", sc.APIName)
			fmt.Printf("  Output Modes   %d/10\n", s.OutputModes)
			fmt.Printf("  Auth           %d/10\n", s.Auth)
			fmt.Printf("  Error Handling %d/10\n", s.ErrorHandling)
			fmt.Printf("  Terminal UX    %d/10\n", s.TerminalUX)
			fmt.Printf("  README         %d/10\n", s.README)
			fmt.Printf("  Doctor         %d/10\n", s.Doctor)
			fmt.Printf("  Agent Native   %d/10\n", s.AgentNative)
			fmt.Printf("  Local Cache    %d/10\n", s.LocalCache)
			fmt.Printf("  Breadth        %d/10\n", s.Breadth)
			fmt.Printf("  Vision         %d/10\n", s.Vision)
			fmt.Printf("  Workflows      %d/10\n", s.Workflows)
			fmt.Printf("  Insight        %d/10\n", s.Insight)
			fmt.Printf("\n  Domain Correctness\n")
			fmt.Printf("  Path Validity          %s\n", renderScore("path_validity", s.PathValidity, 10))
			fmt.Printf("  Auth Protocol          %s\n", renderScore("auth_protocol", s.AuthProtocol, 10))
			fmt.Printf("  Data Pipeline Integrity %d/10\n", s.DataPipelineIntegrity)
			fmt.Printf("  Sync Correctness       %d/10\n", s.SyncCorrectness)
			fmt.Printf("  Type Fidelity          %d/5\n", s.TypeFidelity)
			fmt.Printf("  Dead Code              %d/5\n", s.DeadCode)
			fmt.Printf("\n  Total: %d/100 - Grade %s\n", s.Total, sc.OverallGrade)
			if len(sc.UnscoredDimensions) > 0 {
				fmt.Printf("  Note: omitted from denominator: %s\n", strings.Join(sc.UnscoredDimensions, ", "))
			}

			if live != nil {
				fmt.Printf("\nLive Check (behavioral sample)\n")
				if live.Unable {
					fmt.Printf("  Unable to run: %s\n", live.Reason)
				} else {
					fmt.Printf("  Passed: %d/%d  (%d%% pass rate)\n", live.Passed, live.Checked(), int(live.PassRate*100+0.5))
					if live.Failed > 0 {
						fmt.Println("  Failures:")
						for _, f := range live.Features {
							if f.Status == "fail" {
								fmt.Printf("    - %s: %s\n", f.Name, f.Reason)
							}
						}
					}
					// Wave B output-quality warnings are surfaced here so a
					// developer running scorecard without --json sees them.
					// Wave A review flagged the "human sees less than agent"
					// gap as an agent-native parity concern.
					warnCount := 0
					for _, f := range live.Features {
						warnCount += len(f.Warnings)
					}
					if warnCount > 0 {
						fmt.Printf("  Warnings (%d): not blocking in Wave B — flip to failures in Wave C after calibration\n", warnCount)
						for _, f := range live.Features {
							for _, w := range f.Warnings {
								fmt.Printf("    - %s: %s\n", f.Name, w)
							}
						}
					}
				}
			}

			if len(sc.GapReport) > 0 {
				fmt.Printf("\nGaps:\n")
				for _, g := range sc.GapReport {
					fmt.Printf("  - %s\n", g)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Path to generated CLI directory")
	cmd.Flags().StringVar(&specPath, "spec", "", "Path to OpenAPI spec JSON for semantic validation")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&liveCheck, "live-check", false, "Sample novel-feature examples against real targets and cap Insight when flagships return broken output")
	cmd.Flags().DurationVar(&liveCheckTimeout, "live-check-timeout", 10*time.Second, "Per-feature timeout for live check invocations")

	return cmd
}

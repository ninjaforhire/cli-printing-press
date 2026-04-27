package cli

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// buildShipcheckStub compiles the shipcheck stub once per test run and
// returns its path. The stub mimics the printing-press leg surface
// (dogfood/verify/workflow-verify/verify-skill/scorecard) and is
// configurable via env vars: see internal/cli/testdata/shipcheck-stub/main.go.
func buildShipcheckStub(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "shipcheck-stub")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/shipcheck-stub")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building shipcheck stub: %v\n%s", err, string(buildOut))
	}
	return out
}

// fakeCLIDir creates a minimal directory that satisfies validateShipcheckDir:
// a directory containing go.mod. Returned path is absolute.
func fakeCLIDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fake\n"), 0o644); err != nil {
		t.Fatalf("writing fake go.mod: %v", err)
	}
	return dir
}

// withStubBinary swaps resolveSelfBinary for the duration of a test so
// the umbrella spawns the stub instead of the real printing-press
// binary. Returns a cleanup function callers must defer.
func withStubBinary(t *testing.T, path string) func() {
	t.Helper()
	prev := resolveSelfBinary
	resolveSelfBinary = func() (string, error) { return path, nil }
	return func() { resolveSelfBinary = prev }
}

// readStubLog parses the stub's per-invocation argv log. Each line is
// tab-separated argv as the stub recorded it.
func readStubLog(t *testing.T, logPath string) [][]string {
	t.Helper()
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("opening stub log: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var out [][]string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		out = append(out, strings.Split(line, "\t"))
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading stub log: %v", err)
	}
	return out
}

// runShipcheckCmd runs newShipcheckCmd().RunE with the given args (no
// "shipcheck" prefix) and returns the resulting error. It does not
// intercept stdout/stderr — they go to the test process's own
// streams, which lets `go test -v` show what the stub printed.
func runShipcheckCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newShipcheckCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}

// TestShipcheck_AllLegsPass: every leg exits 0, umbrella returns nil.
// All five legs must be invoked in canonical order with correct argv.
func TestShipcheck_AllLegsPass(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := fakeCLIDir(t)
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)

	if err := runShipcheckCmd(t, "--dir", dir); err != nil {
		t.Fatalf("expected nil error when all legs pass; got %v", err)
	}

	invocations := readStubLog(t, logFile)
	if len(invocations) != len(shipcheckLegs) {
		t.Fatalf("expected %d leg invocations; got %d: %v", len(shipcheckLegs), len(invocations), invocations)
	}

	// Confirm canonical order: dogfood, verify, workflow-verify, verify-skill, scorecard.
	wantOrder := []string{"dogfood", "verify", "workflow-verify", "verify-skill", "scorecard"}
	for i, want := range wantOrder {
		// argv[0] is the stub binary path; argv[1] is the leg name.
		if len(invocations[i]) < 2 {
			t.Fatalf("invocation %d has fewer than 2 args: %v", i, invocations[i])
		}
		if invocations[i][1] != want {
			t.Errorf("invocation %d: want leg %q, got %q (full argv: %v)", i, want, invocations[i][1], invocations[i])
		}
	}
}

// TestShipcheck_OneLegFails: verify-skill exits 1, umbrella returns
// ExitError with code 1; all five legs still ran (no fail-fast).
func TestShipcheck_OneLegFails(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := fakeCLIDir(t)
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)
	t.Setenv("STUB_EXIT_VERIFY_SKILL", "1")

	err := runShipcheckCmd(t, "--dir", dir)
	if err == nil {
		t.Fatal("expected non-nil error when verify-skill fails; got nil")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError; got %T: %v", err, err)
	}
	if exitErr.Code != 1 {
		t.Errorf("expected umbrella exit code 1; got %d", exitErr.Code)
	}
	if !exitErr.Silent {
		t.Error("expected Silent=true so cobra does not duplicate the error message; got Silent=false")
	}

	invocations := readStubLog(t, logFile)
	if len(invocations) != len(shipcheckLegs) {
		t.Errorf("expected %d invocations even when one fails (no fail-fast); got %d", len(shipcheckLegs), len(invocations))
	}
}

// TestShipcheck_MultipleFailures: dogfood exits 2, scorecard exits 1.
// Umbrella exits with the largest non-zero code (2).
func TestShipcheck_MultipleFailures(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := fakeCLIDir(t)
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)
	t.Setenv("STUB_EXIT_DOGFOOD", "2")
	t.Setenv("STUB_EXIT_SCORECARD", "1")

	err := runShipcheckCmd(t, "--dir", dir)
	if err == nil {
		t.Fatal("expected non-nil error when multiple legs fail")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError; got %T", err)
	}
	if exitErr.Code != 2 {
		t.Errorf("expected umbrella exit code 2 (max of failing leg codes); got %d", exitErr.Code)
	}
}

// TestShipcheck_DefaultArgvIncludesFixAndLiveCheck verifies that without
// any opt-out flags, verify gets --fix and scorecard gets --live-check.
// These are the recommended Phase 4 invocations.
func TestShipcheck_DefaultArgvIncludesFixAndLiveCheck(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := fakeCLIDir(t)
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)

	if err := runShipcheckCmd(t, "--dir", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	invocations := readStubLog(t, logFile)
	verifyArgs := findInvocation(invocations, "verify")
	if !argvHas(verifyArgs, "--fix") {
		t.Errorf("expected verify argv to include --fix by default; got %v", verifyArgs)
	}
	scorecardArgs := findInvocation(invocations, "scorecard")
	if !argvHas(scorecardArgs, "--live-check") {
		t.Errorf("expected scorecard argv to include --live-check by default; got %v", scorecardArgs)
	}
}

// TestShipcheck_PassesSpecAndResearchDir: when --spec and --research-dir
// are set, dogfood and scorecard receive both; verify receives --spec.
func TestShipcheck_PassesSpecAndResearchDir(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := fakeCLIDir(t)
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)

	specPath := "/some/spec.yaml"
	researchDir := "/some/research"
	if err := runShipcheckCmd(t, "--dir", dir, "--spec", specPath, "--research-dir", researchDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	invocations := readStubLog(t, logFile)

	dogfoodArgs := findInvocation(invocations, "dogfood")
	if !argvHas(dogfoodArgs, "--spec") || !argvHas(dogfoodArgs, specPath) {
		t.Errorf("dogfood argv missing --spec: %v", dogfoodArgs)
	}
	if !argvHas(dogfoodArgs, "--research-dir") || !argvHas(dogfoodArgs, researchDir) {
		t.Errorf("dogfood argv missing --research-dir: %v", dogfoodArgs)
	}

	verifyArgs := findInvocation(invocations, "verify")
	if !argvHas(verifyArgs, "--spec") || !argvHas(verifyArgs, specPath) {
		t.Errorf("verify argv missing --spec: %v", verifyArgs)
	}

	scorecardArgs := findInvocation(invocations, "scorecard")
	if !argvHas(scorecardArgs, "--spec") || !argvHas(scorecardArgs, specPath) {
		t.Errorf("scorecard argv missing --spec: %v", scorecardArgs)
	}
	if !argvHas(scorecardArgs, "--research-dir") || !argvHas(scorecardArgs, researchDir) {
		t.Errorf("scorecard argv missing --research-dir: %v", scorecardArgs)
	}

	// workflow-verify and verify-skill don't take --spec or --research-dir;
	// confirm they don't get them.
	wfArgs := findInvocation(invocations, "workflow-verify")
	if argvHas(wfArgs, "--spec") {
		t.Errorf("workflow-verify should not receive --spec; got %v", wfArgs)
	}
	vsArgs := findInvocation(invocations, "verify-skill")
	if argvHas(vsArgs, "--spec") {
		t.Errorf("verify-skill should not receive --spec; got %v", vsArgs)
	}
}

// TestShipcheck_RequiresDir: missing --dir returns ExitInputError before
// any leg runs.
func TestShipcheck_RequiresDir(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()
	logFile := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("STUB_LOG_FILE", logFile)

	err := runShipcheckCmd(t)
	if err == nil {
		t.Fatal("expected error for missing --dir")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError; got %T", err)
	}
	if exitErr.Code != ExitInputError {
		t.Errorf("expected ExitInputError; got %d", exitErr.Code)
	}

	// Stub log should be empty — no legs spawned.
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		invocations := readStubLog(t, logFile)
		if len(invocations) != 0 {
			t.Errorf("expected 0 invocations when --dir missing; got %d", len(invocations))
		}
	}
}

// TestShipcheck_RejectsNonexistentDir: --dir pointing at a missing path
// returns ExitInputError.
func TestShipcheck_RejectsNonexistentDir(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	err := runShipcheckCmd(t, "--dir", "/this/path/does/not/exist/anywhere")
	if err == nil {
		t.Fatal("expected error for nonexistent --dir")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError; got %T", err)
	}
	if exitErr.Code != ExitInputError {
		t.Errorf("expected ExitInputError; got %d", exitErr.Code)
	}
}

// TestShipcheck_RejectsDirWithoutGoMod: --dir pointing at a directory
// without go.mod returns ExitInputError. Guards against accidentally
// running shipcheck against a manuscripts dir or unrelated path.
func TestShipcheck_RejectsDirWithoutGoMod(t *testing.T) {
	stub := buildShipcheckStub(t)
	defer withStubBinary(t, stub)()

	dir := t.TempDir() // empty — no go.mod
	err := runShipcheckCmd(t, "--dir", dir)
	if err == nil {
		t.Fatal("expected error for --dir without go.mod")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError; got %T", err)
	}
	if exitErr.Code != ExitInputError {
		t.Errorf("expected ExitInputError; got %d", exitErr.Code)
	}
}

// TestShipcheckUmbrellaCode_Aggregation tests the pure exit-code aggregator.
func TestShipcheckUmbrellaCode_Aggregation(t *testing.T) {
	cases := []struct {
		name    string
		results []shipcheckLegResult
		want    int
	}{
		{
			name: "all pass",
			results: []shipcheckLegResult{
				{Name: "dogfood", ExitCode: 0},
				{Name: "verify", ExitCode: 0},
			},
			want: 0,
		},
		{
			name: "one fails with code 1",
			results: []shipcheckLegResult{
				{Name: "dogfood", ExitCode: 0},
				{Name: "verify", ExitCode: 1},
			},
			want: 1,
		},
		{
			name: "max wins across multiple failures",
			results: []shipcheckLegResult{
				{Name: "dogfood", ExitCode: 2},
				{Name: "verify", ExitCode: 1},
				{Name: "scorecard", ExitCode: 3},
			},
			want: 3,
		},
		{
			name:    "empty results",
			results: nil,
			want:    0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shipcheckUmbrellaCode(c.results); got != c.want {
				t.Errorf("shipcheckUmbrellaCode = %d; want %d", got, c.want)
			}
		})
	}
}

// findInvocation returns the argv slice (excluding the stub binary path)
// for the given leg name, or nil if not found.
func findInvocation(invocations [][]string, leg string) []string {
	for _, argv := range invocations {
		if len(argv) >= 2 && argv[1] == leg {
			return argv[1:]
		}
	}
	return nil
}

func argvHas(argv []string, needle string) bool {
	return slices.Contains(argv, needle)
}

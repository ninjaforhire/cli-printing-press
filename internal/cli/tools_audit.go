package cli

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	ledgerFilename    = ".printing-press-tools-polish.json"
	ledgerStaleAfter  = 24 * time.Hour
	statusAccepted    = "accepted"
	suspiciousMaxLen  = 30
	suspiciousMinWord = 4
)

// newToolsAuditCmd inspects a single printed CLI's command tree for
// MCP tool quality issues a deterministic check can catch: empty Short,
// suspiciously thin Short, and read-shaped command names that lack the
// mcp:read-only annotation. The output is a JSON list of findings the
// agent then runs through the references/tools-polish.md playbook.
//
// Deterministic only — judgment-grade questions ("is this description
// agent-grade?") belong in the polish skill, not here. Diagnostic
// exit code 0 regardless of findings.
func newToolsAuditCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "tools-audit <cli-dir>",
		Short: "Mechanically audit a printed CLI's MCP tool surface for missing annotations and thin descriptions",
		Long: `Walks <cli-dir>/internal/cli/*.go and reports per-command
findings that signal MCP tool quality issues. Detection is purely
mechanical: empty Short fields, Short text under 30 characters with
fewer than 4 words, and read-shaped command names that lack the
mcp:read-only annotation. The agent layer (references/tools-polish.md)
takes these findings and applies judgment for descriptions and
borderline classifications.

Exit 0 regardless of findings (diagnostic, not gating).`,
		Example: `  printing-press tools-audit ~/printing-press/library/dub
  printing-press tools-audit ~/printing-press/library/dub --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliDir := args[0]
			findings, err := runToolsAudit(cliDir)
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(findings)
			}

			previous := readPreviousLedger(cliDir)
			delta := reconcileWithLedger(previous, findings)

			if err := writeLedger(cliDir, findings); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: writing ledger %s: %v\n", filepath.Join(cliDir, ledgerFilename), err)
			}
			renderToolsAuditTable(cmd.OutOrStdout(), findings, delta)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of a human-readable table")
	return cmd
}

// ToolsAuditFinding is one mechanical issue discovered in a command
// definition. Status and Note are ledger-only; the audit phase emits
// findings without them (omitempty keeps --json clean for downstream
// parsing). The agent edits the persisted ledger to flip Status to
// "accepted" with a Note explaining why a thin-short is fine as-is.
// On re-run the binary preserves these fields for any finding whose
// identity key matches.
type ToolsAuditFinding struct {
	Kind     string `json:"kind"`             // "empty-short", "thin-short", "missing-read-only"
	Command  string `json:"command"`          // value of the Use field, e.g. "tiktok"
	File     string `json:"file"`             // path relative to cli-dir
	Line     int    `json:"line"`             // 1-based source line of the cobra.Command literal
	Evidence string `json:"evidence"`         // the offending text (Short value, etc.)
	Status   string `json:"status,omitempty"` // "" (== pending) or "accepted"; agent writes
	Note     string `json:"note,omitempty"`   // agent-written rationale for an accept decision
}

// readShapedNames is the heuristic for "this command name suggests a
// read operation." We exclude verbs already in cobratree's
// frameworkCommands skip set (search, sql, doctor, version) — the
// runtime walker doesn't register those as MCP tools, so a missing
// read-only annotation is meaningless noise for them.
var readShapedNames = map[string]struct{}{
	"list": {}, "get": {}, "show": {}, "view": {},
	"find": {}, "describe": {}, "context": {}, "stats": {},
	"trending": {}, "trust": {}, "health": {}, "stale": {}, "orphans": {},
	"reconcile": {}, "analytics": {},
}

// runToolsAudit reads every non-test .go file in <cliDir>/internal/cli/
// and accumulates findings across each Command literal. Returns
// findings sorted by file then line so the human-readable table is
// stable and the JSON output is diff-friendly.
func runToolsAudit(cliDir string) ([]ToolsAuditFinding, error) {
	pkgDir := filepath.Join(cliDir, "internal", "cli")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", pkgDir, err)
	}
	var findings []ToolsAuditFinding
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(pkgDir, name)
		// Skip unparseable files — the agent can run go build separately
		// to surface syntax errors without failing the audit.
		file, err := parser.ParseFile(fset, full, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isCobraCommandType(lit.Type) {
				return true
			}
			fields := extractCommandFields(lit)
			if fields.use == "" {
				return true
			}
			line := fset.Position(lit.Pos()).Line
			findings = append(findings, auditCommandFields(name, line, fields)...)
			return true
		})
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

type commandFields struct {
	use         string
	short       string
	hasReadOnly bool
	hasEndpoint bool
}

func isCobraCommandType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "cobra" && sel.Sel.Name == "Command"
}

// extractCommandFields pulls Use/Short/Annotations out of a composite
// literal. Concatenated string literals and unresolvable expressions
// surface as the empty string — acceptable since the audit's job is to
// flag missing or thin content, not enforce that all values be string
// literals.
func extractCommandFields(lit *ast.CompositeLit) commandFields {
	var f commandFields
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Use":
			f.use = stringLit(kv.Value)
		case "Short":
			f.short = stringLit(kv.Value)
		case "Annotations":
			f.hasReadOnly, f.hasEndpoint = inspectAnnotations(kv.Value)
		}
	}
	return f
}

func stringLit(e ast.Expr) string {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}
	if len(bl.Value) >= 2 && (bl.Value[0] == '"' || bl.Value[0] == '`') {
		return bl.Value[1 : len(bl.Value)-1]
	}
	return bl.Value
}

func inspectAnnotations(e ast.Expr) (hasReadOnly, hasEndpoint bool) {
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return false, false
	}
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch stringLit(kv.Key) {
		case "mcp:read-only":
			hasReadOnly = stringLit(kv.Value) == "true"
		case "pp:endpoint":
			hasEndpoint = stringLit(kv.Value) != ""
		}
	}
	return hasReadOnly, hasEndpoint
}

func auditCommandFields(file string, line int, f commandFields) []ToolsAuditFinding {
	cmdName := strings.Fields(f.use)
	if len(cmdName) == 0 {
		return nil
	}
	name := cmdName[0]

	var out []ToolsAuditFinding
	switch {
	case f.short == "":
		out = append(out, ToolsAuditFinding{
			Kind: "empty-short", Command: name, File: file, Line: line,
			Evidence: "(empty)",
		})
	case suspiciousShort(f.short):
		out = append(out, ToolsAuditFinding{
			Kind: "thin-short", Command: name, File: file, Line: line,
			Evidence: f.short,
		})
	}
	// pp:endpoint commands are skipped by the runtime walker (they get
	// typed-tool registration with method-derived classification), so
	// the missing-read-only check doesn't apply to them.
	if !f.hasEndpoint && !f.hasReadOnly && readShapedName(name) {
		out = append(out, ToolsAuditFinding{
			Kind: "missing-read-only", Command: name, File: file, Line: line,
			Evidence: "name matches read heuristic; no mcp:read-only annotation",
		})
	}
	return out
}

// suspiciousShort flags Short text that's both short (under 30 chars)
// and uses fewer than 4 words. Either dimension alone is fine: a long
// 3-word phrase is OK, and a short-but-precise instruction is OK.
// Both together is the "Search Ads" / "Subreddit Posts" anti-pattern.
func suspiciousShort(s string) bool {
	return len(s) < suspiciousMaxLen && len(strings.Fields(s)) < suspiciousMinWord
}

// readShapedName matches the head before the first hyphen against the
// readShapedNames set. Compound names like "get-foo" or "list-bar"
// classify by their leading verb.
func readShapedName(name string) bool {
	head := name
	if i := strings.IndexByte(name, '-'); i > 0 {
		head = name[:i]
	}
	_, ok := readShapedNames[head]
	return ok
}

func renderToolsAuditTable(w io.Writer, findings []ToolsAuditFinding, delta ledgerDelta) {
	var pending, accepted int
	for _, f := range findings {
		if f.Status == statusAccepted {
			accepted++
		} else {
			pending++
		}
	}
	if pending == 0 {
		if accepted > 0 {
			fmt.Fprintf(w, "tools-audit: no pending findings (%d accepted)\n", accepted)
		} else {
			fmt.Fprintln(w, "tools-audit: no findings")
		}
		if delta.hasPrevious && len(delta.resolved) > 0 {
			fmt.Fprintf(w, "since last run: %d resolved, 0 new\n", len(delta.resolved))
		}
		return
	}
	fmt.Fprintf(w, "tools-audit: %d pending finding(s)", pending)
	if accepted > 0 {
		fmt.Fprintf(w, " (%d accepted)", accepted)
	}
	fmt.Fprintln(w)
	if delta.hasPrevious {
		fmt.Fprintf(w, "since last run: %d resolved, %d new\n", len(delta.resolved), len(delta.added))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-20s  %-15s  %-30s  %s\n", "KIND", "COMMAND", "FILE:LINE", "EVIDENCE")
	for _, f := range findings {
		if f.Status == statusAccepted {
			continue
		}
		loc := fmt.Sprintf("%s:%d", f.File, f.Line)
		fmt.Fprintf(w, "%-20s  %-15s  %-30s  %s\n", f.Kind, f.Command, loc, f.Evidence)
	}
}

// ToolsAuditLedger is the on-disk snapshot of the last audit run.
type ToolsAuditLedger struct {
	Timestamp time.Time           `json:"timestamp"`
	CLIDir    string              `json:"cli_dir"`
	Findings  []ToolsAuditFinding `json:"findings"`
}

type ledgerDelta struct {
	hasPrevious bool
	resolved    []ToolsAuditFinding // present in previous, absent in current
	added       []ToolsAuditFinding // present in current, absent in previous
}

// readPreviousLedger loads the ledger at <cli-dir>/<ledgerFilename>.
// Returns nil for missing, corrupt, or stale ledgers — the audit treats
// all three as "no resumable state." Stale and corrupt files are
// deleted so the next write starts clean. Read errors other than "not
// exists" silently fall back to no-ledger; the next write surfaces
// the same error to stderr.
func readPreviousLedger(cliDir string) *ToolsAuditLedger {
	path := filepath.Join(cliDir, ledgerFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var l ToolsAuditLedger
	if err := json.Unmarshal(data, &l); err != nil {
		_ = os.Remove(path)
		return nil
	}
	if time.Since(l.Timestamp) > ledgerStaleAfter {
		_ = os.Remove(path)
		return nil
	}
	return &l
}

func writeLedger(cliDir string, findings []ToolsAuditFinding) error {
	ledger := ToolsAuditLedger{
		Timestamp: time.Now().UTC(),
		CLIDir:    cliDir,
		Findings:  findings,
	}
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding ledger: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(cliDir, ledgerFilename), data, 0644)
}

// reconcileWithLedger carries Status/Note from the previous ledger
// onto matching current findings (so accept decisions survive re-runs)
// and computes the resolved/added delta in a single pass. Identity is
// (file, line, kind, command, evidence); a finding whose Short was
// rewritten reads as "old resolved, new added" rather than mutated.
func reconcileWithLedger(previous *ToolsAuditLedger, current []ToolsAuditFinding) ledgerDelta {
	if previous == nil {
		return ledgerDelta{}
	}
	prev := make(map[string]ToolsAuditFinding, len(previous.Findings))
	for _, f := range previous.Findings {
		prev[findingKey(f)] = f
	}
	delta := ledgerDelta{hasPrevious: true}
	seen := make(map[string]bool, len(current))
	for i := range current {
		k := findingKey(current[i])
		seen[k] = true
		if old, ok := prev[k]; ok {
			current[i].Status = old.Status
			current[i].Note = old.Note
		} else {
			delta.added = append(delta.added, current[i])
		}
	}
	for k, f := range prev {
		if !seen[k] {
			delta.resolved = append(delta.resolved, f)
		}
	}
	return delta
}

func findingKey(f ToolsAuditFinding) string {
	return fmt.Sprintf("%s:%d:%s:%s:%s", f.File, f.Line, f.Kind, f.Command, f.Evidence)
}

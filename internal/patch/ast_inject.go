package patch

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// injectOptions controls which feature mutations are applied. When a feature
// is in Skip, every mutation it owns is omitted: its rootFlags fields, its
// imports, its flag registration, its PersistentPreRunE block, its
// AddCommand call, and (for deliver) its post-Execute flush. This keeps the
// patched CLI buildable when a companion drop-in is skipped due to a
// resource-level collision.
type injectOptions struct {
	// Skip contains feature names: "profile", "deliver", "feedback".
	Skip map[string]bool
}

func (o injectOptions) skip(feature string) bool {
	return o.Skip != nil && o.Skip[feature]
}

// injectRootAST applies PR #218's root.go mutations to src and returns the
// patched source + whether any mutation occurred. Mutations are idempotent:
// a second call against already-patched source returns (src, false, nil).
func injectRootAST(src []byte, opts injectOptions) ([]byte, bool, error) {
	file, err := decorator.Parse(src)
	if err != nil {
		return nil, false, fmt.Errorf("parse root.go: %w", err)
	}

	changed := false
	changed = addRootFlagsFields(file, opts) || changed
	// Imports (bytes/io/os) are only used by deliver's mutations.
	if !opts.skip("deliver") {
		changed = addImports(file, "bytes", "io", "os") || changed
	}
	changed = addPersistentFlags(file, opts) || changed
	changed = addPreRunBlocks(file, opts) || changed
	changed = addCommands(file, opts) || changed
	if !opts.skip("deliver") {
		changed = addPostExecuteFlush(file) || changed
	}

	if !changed {
		return src, false, nil
	}

	var buf bytes.Buffer
	if err := decorator.Fprint(&buf, file); err != nil {
		return nil, false, fmt.Errorf("format root.go: %w", err)
	}
	return buf.Bytes(), true, nil
}

// addRootFlagsFields appends profileName, deliverSpec, deliverBuf, deliverSink
// to the rootFlags struct. Fields owned by skipped features are omitted.
func addRootFlagsFields(file *dst.File, opts injectOptions) bool {
	changed := false
	dst.Inspect(file, func(n dst.Node) bool {
		ts, ok := n.(*dst.TypeSpec)
		if !ok || ts.Name.Name != "rootFlags" {
			return true
		}
		st, ok := ts.Type.(*dst.StructType)
		if !ok {
			return true
		}
		if !opts.skip("profile") && !structHasField(st, "profileName") {
			st.Fields.List = append(st.Fields.List, &dst.Field{
				Names: []*dst.Ident{{Name: "profileName"}},
				Type:  &dst.Ident{Name: "string"},
			})
			changed = true
		}
		if !opts.skip("deliver") {
			if !structHasField(st, "deliverSpec") {
				st.Fields.List = append(st.Fields.List, &dst.Field{
					Names: []*dst.Ident{{Name: "deliverSpec"}},
					Type:  &dst.Ident{Name: "string"},
				})
				changed = true
			}
			if !structHasField(st, "deliverBuf") {
				st.Fields.List = append(st.Fields.List, &dst.Field{
					Names: []*dst.Ident{{Name: "deliverBuf"}},
					Type: &dst.StarExpr{X: &dst.SelectorExpr{
						X:   &dst.Ident{Name: "bytes"},
						Sel: &dst.Ident{Name: "Buffer"},
					}},
				})
				changed = true
			}
			if !structHasField(st, "deliverSink") {
				st.Fields.List = append(st.Fields.List, &dst.Field{
					Names: []*dst.Ident{{Name: "deliverSink"}},
					Type:  &dst.Ident{Name: "DeliverSink"},
				})
				changed = true
			}
		}
		return false
	})
	return changed
}

func structHasField(st *dst.StructType, name string) bool {
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

// addImports adds packages to the file's import list if absent. Returns true
// if any were added.
func addImports(file *dst.File, pkgs ...string) bool {
	existing := map[string]bool{}
	for _, imp := range file.Imports {
		existing[strings.Trim(imp.Path.Value, `"`)] = true
	}
	if len(file.Decls) == 0 {
		return false
	}
	importDecl, ok := file.Decls[0].(*dst.GenDecl)
	if !ok || importDecl.Tok.String() != "import" {
		return false
	}
	changed := false
	for _, pkg := range pkgs {
		if existing[pkg] {
			continue
		}
		importDecl.Specs = append(importDecl.Specs, &dst.ImportSpec{
			Path: &dst.BasicLit{Value: fmt.Sprintf(`"%s"`, pkg)},
		})
		changed = true
	}
	return changed
}

// addPersistentFlags inserts --profile and --deliver after the last existing
// PersistentFlags() registration inside Execute(). Skipped features are
// omitted; if both profile and deliver are skipped, no flag is added.
func addPersistentFlags(file *dst.File, opts injectOptions) bool {
	changed := false
	dst.Inspect(file, func(n dst.Node) bool {
		fn, ok := n.(*dst.FuncDecl)
		if !ok || fn.Name.Name != "Execute" {
			return true
		}
		var newStmts []dst.Stmt
		if !opts.skip("profile") {
			// Idempotency: skip if --profile already registered.
			already := false
			for _, stmt := range fn.Body.List {
				if persistentFlagsRegisters(stmt, "profile") {
					already = true
					break
				}
			}
			if !already {
				newStmts = append(newStmts, parseStmt(`rootCmd.PersistentFlags().StringVar(&flags.profileName, "profile", "", "Apply values from a saved profile")`))
			}
		}
		if !opts.skip("deliver") {
			already := false
			for _, stmt := range fn.Body.List {
				if persistentFlagsRegisters(stmt, "deliver") {
					already = true
					break
				}
			}
			if !already {
				newStmts = append(newStmts, parseStmt(`rootCmd.PersistentFlags().StringVar(&flags.deliverSpec, "deliver", "", "Route output to a sink: stdout (default), file:<path>, webhook:<url>")`))
			}
		}
		if len(newStmts) == 0 {
			return false
		}
		lastFlagIdx := -1
		for i, stmt := range fn.Body.List {
			if isPersistentFlagsCall(stmt) {
				lastFlagIdx = i
			}
		}
		if lastFlagIdx < 0 {
			return false
		}
		fn.Body.List = append(fn.Body.List[:lastFlagIdx+1], append(newStmts, fn.Body.List[lastFlagIdx+1:]...)...)
		changed = true
		return false
	})
	return changed
}

// addPreRunBlocks inserts the deliver-setup and profile-lookup blocks at the
// top of the PersistentPreRunE function body. Skipped features are omitted.
func addPreRunBlocks(file *dst.File, opts injectOptions) bool {
	changed := false
	dst.Inspect(file, func(n dst.Node) bool {
		assign, ok := n.(*dst.AssignStmt)
		if !ok || len(assign.Lhs) != 1 {
			return true
		}
		sel, ok := assign.Lhs[0].(*dst.SelectorExpr)
		if !ok || sel.Sel.Name != "PersistentPreRunE" {
			return true
		}
		if len(assign.Rhs) != 1 {
			return false
		}
		fn, ok := assign.Rhs[0].(*dst.FuncLit)
		if !ok {
			return false
		}
		var prepend []dst.Stmt
		if !opts.skip("deliver") && !nodeReferences(fn, "deliverSpec") {
			prepend = append(prepend, parseStmt(`if flags.deliverSpec != "" {
				sink, err := ParseDeliverSink(flags.deliverSpec)
				if err != nil {
					return err
				}
				flags.deliverSink = sink
				if sink.Scheme != "stdout" && sink.Scheme != "" {
					flags.deliverBuf = &bytes.Buffer{}
					cmd.SetOut(io.MultiWriter(os.Stdout, flags.deliverBuf))
				}
			}`))
		}
		if !opts.skip("profile") && !nodeReferences(fn, "profileName") {
			prepend = append(prepend, parseStmt(`if flags.profileName != "" {
				profile, err := GetProfile(flags.profileName)
				if err != nil {
					return err
				}
				if profile == nil {
					return fmt.Errorf("profile %q not found", flags.profileName)
				}
				if err := ApplyProfileToFlags(cmd, profile); err != nil {
					return err
				}
			}`))
		}
		if len(prepend) == 0 {
			return false
		}
		fn.Body.List = append(prepend, fn.Body.List...)
		changed = true
		return false
	})
	return changed
}

// addCommands appends newProfileCmd and newFeedbackCmd AddCommand calls after
// the last existing rootCmd.AddCommand entry. Skipped features are omitted.
func addCommands(file *dst.File, opts injectOptions) bool {
	changed := false
	dst.Inspect(file, func(n dst.Node) bool {
		fn, ok := n.(*dst.FuncDecl)
		if !ok || fn.Name.Name != "Execute" {
			return true
		}
		var newStmts []dst.Stmt
		if !opts.skip("profile") {
			already := false
			for _, stmt := range fn.Body.List {
				if rootAddsCommand(stmt, "newProfileCmd") {
					already = true
					break
				}
			}
			if !already {
				newStmts = append(newStmts, parseStmt(`rootCmd.AddCommand(newProfileCmd(&flags))`))
			}
		}
		if !opts.skip("feedback") {
			already := false
			for _, stmt := range fn.Body.List {
				if rootAddsCommand(stmt, "newFeedbackCmd") {
					already = true
					break
				}
			}
			if !already {
				newStmts = append(newStmts, parseStmt(`rootCmd.AddCommand(newFeedbackCmd(&flags))`))
			}
		}
		if len(newStmts) == 0 {
			return false
		}
		lastAddIdx := -1
		for i, stmt := range fn.Body.List {
			if isRootAddCommand(stmt) {
				lastAddIdx = i
			}
		}
		if lastAddIdx < 0 {
			return false
		}
		fn.Body.List = append(fn.Body.List[:lastAddIdx+1], append(newStmts, fn.Body.List[lastAddIdx+1:]...)...)
		changed = true
		return false
	})
	return changed
}

// addPostExecuteFlush inserts the deliverBuf flush block between the last
// rootCmd.Execute() result handling and the final `return err` of Execute().
func addPostExecuteFlush(file *dst.File) bool {
	changed := false
	dst.Inspect(file, func(n dst.Node) bool {
		fn, ok := n.(*dst.FuncDecl)
		if !ok || fn.Name.Name != "Execute" {
			return true
		}
		// Idempotency: skip if Deliver() already referenced.
		if nodeReferences(fn, "Deliver") {
			return false
		}
		// Find the final `return err` statement.
		returnIdx := -1
		for i, stmt := range fn.Body.List {
			ret, ok := stmt.(*dst.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			if id, ok := ret.Results[0].(*dst.Ident); ok && id.Name == "err" {
				returnIdx = i
			}
		}
		if returnIdx < 0 {
			return false
		}
		flushBlock := parseStmt(`if err == nil && flags.deliverBuf != nil {
			if derr := Deliver(flags.deliverSink, flags.deliverBuf.Bytes(), flags.compact); derr != nil {
				fmt.Fprintf(os.Stderr, "warning: deliver to %s:%s failed: %v\n", flags.deliverSink.Scheme, flags.deliverSink.Target, derr)
				return derr
			}
		}`)
		fn.Body.List = append(fn.Body.List[:returnIdx], append([]dst.Stmt{flushBlock}, fn.Body.List[returnIdx:]...)...)
		changed = true
		return false
	})
	return changed
}

// --- AST helpers ---

func isPersistentFlagsCall(stmt dst.Stmt) bool {
	exprStmt, ok := stmt.(*dst.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*dst.CallExpr)
	if !ok {
		return false
	}
	outerSel, ok := call.Fun.(*dst.SelectorExpr)
	if !ok {
		return false
	}
	innerCall, ok := outerSel.X.(*dst.CallExpr)
	if !ok {
		return false
	}
	innerSel, ok := innerCall.Fun.(*dst.SelectorExpr)
	if !ok {
		return false
	}
	return innerSel.Sel.Name == "PersistentFlags"
}

func persistentFlagsRegisters(stmt dst.Stmt, flagName string) bool {
	exprStmt, ok := stmt.(*dst.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*dst.CallExpr)
	if !ok || len(call.Args) < 2 {
		return false
	}
	lit, ok := call.Args[1].(*dst.BasicLit)
	if !ok {
		return false
	}
	return strings.Trim(lit.Value, `"`) == flagName
}

func isRootAddCommand(stmt dst.Stmt) bool {
	exprStmt, ok := stmt.(*dst.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*dst.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*dst.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*dst.Ident)
	return ok && id.Name == "rootCmd" && sel.Sel.Name == "AddCommand"
}

func rootAddsCommand(stmt dst.Stmt, ctorName string) bool {
	exprStmt, ok := stmt.(*dst.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*dst.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	inner, ok := call.Args[0].(*dst.CallExpr)
	if !ok {
		return false
	}
	id, ok := inner.Fun.(*dst.Ident)
	return ok && id.Name == ctorName
}

// nodeReferences walks a dst node and returns true if any Ident or
// SelectorExpr has the given name. Used for idempotency checks.
func nodeReferences(node dst.Node, name string) bool {
	found := false
	dst.Inspect(node, func(n dst.Node) bool {
		if found {
			return false
		}
		switch v := n.(type) {
		case *dst.Ident:
			if v.Name == name {
				found = true
				return false
			}
		case *dst.SelectorExpr:
			if v.Sel.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// checkRootShape verifies the target root.go matches the printing-press
// generator's shape that the patcher's AST matchers assume. Returns an
// empty string on match, or a human-readable mismatch reason. The drop-in
// files reference `rootFlags` and expect AST injection to land new wiring
// inside the `rootCmd.PersistentFlags()` / `rootCmd.AddCommand()` blocks;
// a non-matching shape (e.g. package-global `root` with no rootFlags
// struct, as some older synthetic CLIs have) would produce a compile
// failure if we wrote drop-ins anyway.
func checkRootShape(src []byte) string {
	file, err := decorator.Parse(src)
	if err != nil {
		return fmt.Sprintf("root.go does not parse: %v", err)
	}
	hasRootFlags := false
	hasRootCmdFlags := false
	hasRootCmdAddCommand := false
	dst.Inspect(file, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.TypeSpec:
			if v.Name.Name == "rootFlags" {
				if _, ok := v.Type.(*dst.StructType); ok {
					hasRootFlags = true
				}
			}
		case *dst.ExprStmt:
			if isPersistentFlagsCall(v) {
				hasRootCmdFlags = true
			}
			if isRootAddCommand(v) {
				hasRootCmdAddCommand = true
			}
		}
		return true
	})
	var missing []string
	if !hasRootFlags {
		missing = append(missing, "rootFlags struct")
	}
	if !hasRootCmdFlags {
		missing = append(missing, "rootCmd.PersistentFlags() call")
	}
	if !hasRootCmdAddCommand {
		missing = append(missing, "rootCmd.AddCommand call")
	}
	if len(missing) == 0 {
		return ""
	}
	return "root.go missing " + strings.Join(missing, ", ")
}

// parseStmt parses a single Go statement and returns it as a dst.Stmt.
// Wraps the snippet in a synthetic function so go/parser accepts it.
func parseStmt(src string) dst.Stmt {
	wrapper := "package _p\nfunc _() {\n" + src + "\n}\n"
	file, err := decorator.Parse(wrapper)
	if err != nil {
		panic(fmt.Errorf("patch.parseStmt: %w (src=%q)", err, src))
	}
	fn := file.Decls[0].(*dst.FuncDecl)
	return fn.Body.List[0]
}

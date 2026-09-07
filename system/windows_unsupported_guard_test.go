package system

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// FEAT-010 Task 7. Ten of the fourteen findings in Sharon Carter's Windows
// capability audit share one root cause: a *_windows.go function that
// unconditionally returns a hardcoded placeholder value ("-1", -1, false,
// nil, "", 0) together with a literal nil error -- the exact shape
// system/file_windows.go's Mode/Owner/Uid/Group/Gid had before Task 2. Fixing
// each instance one at a time guarantees a fifteenth. This is the durable
// answer: a source-level guard that catches the CLASS, from Linux, with no
// Windows host and no Actions minutes.
//
// Modelled directly on system/waitdelay_guard_test.go -- read that file's
// header comment before changing this one; the same reasoning applies here
// verbatim. In particular: this reads the directory itself and parses each
// file with parser.ParseFile rather than calling parser.ParseDir or using
// golang.org/x/tools/go/packages, because both of those resolve per-GOOS and
// would hide every *_windows.go file from a Linux run, defeating the guard
// entirely. Parsing file-by-file keeps the build-tag blindness without a
// deprecated call.
//
// Rule enforced: a function or method in a *_windows.go file (non-test) whose
// last declared return value is `error` must not have a return statement
// where every non-error return value is a hardcoded sentinel ("-1", -1,
// false, nil, "", 0, resolved through simple aliasing -- see below) AND the
// error value is a literal nil.
//
// Scope and known limits, stated explicitly rather than left as a silent
// hole (same discipline as setsWaitDelay's comment in the waitdelay guard):
//
//   - Only checks a function's TOP-LEVEL statements (fn.Body.List), not
//     statements nested inside an if/for/switch/select/range. A function
//     that does real conditional work anywhere is presumed non-trivial and
//     is skipped entirely, even if one of its branches happens to return a
//     hardcoded sentinel + nil. This is deliberate: distinguishing a
//     genuine "lookup ran and found nothing" branch (correct, see
//     system/registry_windows.go's Exists) from a disguised stub inside a
//     branch is a type-checking problem, not a syntactic one, and this
//     guard is syntactic by design (see the waitdelay guard's own header
//     for why: it has to run without build constraints to reach the
//     Windows file at all).
//   - Sentinel resolution follows three levels of indirection, matching the
//     three non-literal bypass shapes named in FEAT-010 (a fourth, a
//     directly-inlined literal, needs no resolution at all):
//     1. a local variable assigned a literal in an earlier top-level
//     statement of the same function ("a variable assigned a literal
//     one line earlier"),
//     2. a package-level const or var initialised from a literal in the
//     same file ("a named constant whose value is -1"),
//     3. a single delegate call `return f.unsupported()` where the callee
//     is itself classified as returning a sentinel ("a helper function
//     that returns the sentinel"), resolved to a fixed point over
//     multiple passes so a chain of delegates is also caught.
func TestWindowsUnsupportedFunctionsDoNotFabricateSentinels(t *testing.T) {
	dirs := []string{".", "../util"}

	type fnInfo struct {
		file     string
		funcName string
		fn       *ast.FuncDecl
		results  int // number of return values in the signature
	}

	fset := token.NewFileSet()
	allFuncs := map[string]fnInfo{} // key: "file.go:FuncName" or "file.go:(*Recv).Method"
	literalConsts := map[string]ast.Expr{}
	checked := 0

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading directory %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, "_windows.go") ||
				strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}

			// Pass 1 (per file): collect package-level const/var bound to a
			// literal, for bypass shape 1 ("a named constant whose value is -1").
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
					continue
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, ident := range vs.Names {
						if i < len(vs.Values) && isSentinelLiteral(vs.Values[i]) {
							literalConsts[ident.Name] = vs.Values[i]
						}
					}
				}
			}

			// Collect every func/method whose last result is `error`.
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Type.Results == nil {
					continue
				}
				results := fn.Type.Results.List
				if len(results) == 0 {
					continue
				}
				last := results[len(results)-1]
				lastIdent, ok := last.Type.(*ast.Ident)
				if !ok || lastIdent.Name != "error" {
					continue
				}
				n := 0
				for _, f := range results {
					if len(f.Names) > 0 {
						n += len(f.Names)
					} else {
						n++
					}
				}
				if n < 2 {
					// A bare `(error)` return has no value slot to fabricate.
					continue
				}
				key := funcKey(name, fn)
				allFuncs[key] = fnInfo{file: name, funcName: fn.Name.Name, fn: fn, results: n}
				checked++
			}
		}
	}

	// Classify every collected function, fixed-point over a few passes so
	// delegate chains (bypass shape 3) resolve. Delegate lookups
	// (calleeKey) are by bare function/method NAME only -- this guard has
	// no type information (deliberately syntactic, see the header comment),
	// so it cannot resolve `f.zzzUnsupported()`'s receiver to a concrete
	// type. Matching by name is the same trade-off the rest of this guard
	// makes: narrower precision in exchange for working without a type
	// checker.
	violations := map[string]bool{}
	for pass := 0; pass < 4; pass++ {
		violatedNames := map[string]bool{}
		for key := range violations {
			violatedNames[allFuncs[key].funcName] = true
		}
		changed := false
		for key, info := range allFuncs {
			if violations[key] {
				continue
			}
			if isSentinelStub(info.fn, info.results, literalConsts, violatedNames) {
				violations[key] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	for key := range violations {
		info := allFuncs[key]
		t.Errorf("%s: %s unconditionally returns a hardcoded sentinel value "+
			"together with a literal nil error. On Windows this reports a "+
			"plausible-looking wrong answer instead of an honest 'this is not "+
			"supported' error -- see FEAT-010 SW-1..SW-4 for the pattern this "+
			"guard exists to catch, and system/registry_notwindows.go for the "+
			"sentinel-error idiom to use instead.", info.file, info.funcName)
	}

	// A guard that silently matches nothing is worse than no guard: if the
	// *_windows.go file set shrank, or every function stopped returning
	// (..., error), this guard would vacuously pass. Assert it still has
	// reach. 18 is the count of (..., error)-returning *_windows.go
	// functions measured at the time this guard was written (2026-09-01);
	// this is a reach check, not a code-quality target -- if this trips,
	// check whether the guard broke before assuming the code did.
	if checked < 15 {
		t.Errorf("found only %d candidate *_windows.go functions, expected at least 15 -- "+
			"this guard has lost its reach, not the code its bound", checked)
	}
}

// funcKey builds a stable identity for a func/method declaration, including
// the receiver type for methods so two identically-named methods on
// different types don't collide.
func funcKey(file string, fn *ast.FuncDecl) string {
	recv := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv = exprString(fn.Recv.List[0].Type) + "."
	}
	return file + ":" + recv + fn.Name.Name
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	default:
		return "?"
	}
}

// isSentinelStub reports whether fn's top-level statements amount to nothing
// more than a hardcoded-sentinel-plus-nil-error return, per the rule and
// scope documented on TestWindowsUnsupportedFunctionsDoNotFabricateSentinels.
func isSentinelStub(fn *ast.FuncDecl, wantResults int, consts map[string]ast.Expr, knownStubs map[string]bool) bool {
	locals := map[string]ast.Expr{}
	var finalReturn *ast.ReturnStmt

	for _, stmt := range fn.Body.List {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			// Only a single, final return statement is trivial. A function
			// with an earlier return (implying it was reached via some
			// already-excluded control flow, or multiple top-level returns)
			// is not in scope for this narrow guard.
			finalReturn = s
		case *ast.AssignStmt:
			// x := <literal> (or var x = <literal>) -- bypass shape 1.
			if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				return false
			}
			ident, ok := s.Lhs[0].(*ast.Ident)
			if !ok {
				return false
			}
			if !isSentinelLiteral(s.Rhs[0]) {
				return false
			}
			locals[ident.Name] = s.Rhs[0]
		case *ast.DeclStmt:
			gen, ok := s.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return false
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					return false
				}
				for i, name := range vs.Names {
					if i < len(vs.Values) && isSentinelLiteral(vs.Values[i]) {
						locals[name.Name] = vs.Values[i]
					}
				}
			}
		default:
			// Any other top-level statement (if, for, switch, select,
			// range, go, defer, expr-statement, ...) means real work is
			// happening. Not trivial -- see the guard's documented scope.
			return false
		}
	}

	if finalReturn == nil {
		return false
	}

	// Shape A: `return f.unsupported()` -- one call expression standing in
	// for every result at once (bypass shape 3).
	if len(finalReturn.Results) == 1 && wantResults > 1 {
		call, ok := finalReturn.Results[0].(*ast.CallExpr)
		if !ok {
			return false
		}
		callee := calleeKey(call)
		if callee == "" {
			return false
		}
		return knownStubs[callee]
	}

	if len(finalReturn.Results) != wantResults {
		return false
	}

	// The error slot (last value) must be a literal/aliased nil.
	if !resolvesToNilLiteral(finalReturn.Results[wantResults-1], locals, consts) {
		return false
	}
	// Every other slot must resolve to a sentinel literal.
	for i := 0; i < wantResults-1; i++ {
		if !resolvesToSentinelLiteral(finalReturn.Results[i], locals, consts) {
			return false
		}
	}
	return true
}

// calleeKey extracts the bare function/method name being called, so a
// delegate return (`return f.unsupported()`) can be checked against the
// set of already-classified stub names. This guard has no type
// information (deliberately syntactic -- see the header comment), so it
// cannot resolve a method call's receiver to a concrete type and match on
// (type, name); bare-name matching is the precision this trade-off buys.
// Returns "" if the call shape isn't a plain `x.Name(...)` or `Name(...)`
// (e.g. a call through a more complex expression) -- in which case the
// guard does not flag it, consistent with staying narrow rather than
// guessing.
func calleeKey(call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	if ident, ok := call.Fun.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// resolvesToSentinelLiteral reports whether expr is, directly or through one
// level of local-variable or package-level-const aliasing, one of the
// sentinel literals.
func resolvesToSentinelLiteral(expr ast.Expr, locals, consts map[string]ast.Expr) bool {
	if isSentinelLiteral(expr) {
		return true
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if v, ok := locals[ident.Name]; ok {
			return isSentinelLiteral(v)
		}
		if v, ok := consts[ident.Name]; ok {
			return isSentinelLiteral(v)
		}
	}
	return false
}

// resolvesToNilLiteral is resolvesToSentinelLiteral narrowed to nil
// specifically, for the error slot.
func resolvesToNilLiteral(expr ast.Expr, locals, consts map[string]ast.Expr) bool {
	if isNilIdent(expr) {
		return true
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if v, ok := locals[ident.Name]; ok {
			return isNilIdent(v)
		}
		if v, ok := consts[ident.Name]; ok {
			return isNilIdent(v)
		}
	}
	return false
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// isSentinelLiteral matches exactly the five literal shapes FEAT-010 names:
// "-1", -1, false, nil, "", 0.
func isSentinelLiteral(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name == "false" || v.Name == "nil"
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			unquoted, err := strconv.Unquote(v.Value)
			return err == nil && (unquoted == "-1" || unquoted == "")
		case token.INT:
			return v.Value == "0"
		}
		return false
	case *ast.UnaryExpr:
		// -1 as an int literal is UnaryExpr{Op: SUB, X: BasicLit{"1"}}.
		if v.Op != token.SUB {
			return false
		}
		lit, ok := v.X.(*ast.BasicLit)
		return ok && lit.Kind == token.INT && lit.Value == "1"
	}
	return false
}

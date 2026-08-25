package system

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every function that runs a util.Command must bound its I/O.
//
// This is a SOURCE-level check, deliberately, because the defect it guards
// against is invisible to a normal test on this host. system/service_windows.go
// carries runHelperPowershell, a near-duplicate of runHelperCommand that exists
// because the powershell wrapper builds its Cmd differently. When the WaitDelay
// bound was added to runHelperCommand, that copy was missed -- and being
// //go:build windows, no test on Linux or on go-builder (AlmaLinux) could
// notice. No Windows host exists in this sandbox or on the remote either, so
// "run it on Windows CI" is not a substitute.
//
// go/parser ignores build constraints, so parsing the directory reaches every
// file for every GOOS from wherever this runs. That is the whole point: this
// asserts a property of code it cannot execute.
//
// The rule is narrow on purpose. A function that CONSTRUCTS a command without
// running it (commandWrapper in command_posix.go and command_windows.go) is not
// in scope -- its caller owns the bound. Only the function that calls Run does.
func TestEveryCommandRunnerBoundsItsIO(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package system: %v", err)
	}

	checked := 0
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if !callsRun(fn.Body) {
					continue
				}
				checked++
				if !setsWaitDelay(fn.Body) {
					t.Errorf("%s: %s calls Run() on a command but never sets "+
						"WaitDelay. The context bounds the PROCESS; only WaitDelay "+
						"bounds Wait, which blocks for as long as anything holds the "+
						"stdout pipe -- an escaped grandchild holds it for its own "+
						"lifetime. Under `serve` that wedges syverMu for the life of "+
						"the process. Derive it from the caller's budget, never a "+
						"constant.", filepath.Base(fset.Position(fn.Pos()).Filename), fn.Name.Name)
					_ = path
				}
			}
		}
	}

	// A guard that silently matches nothing is worse than no guard: if Run were
	// renamed or wrapped, every check above would vacuously pass. Assert the
	// guard still has reach. Three runners today -- runHelperCommand,
	// runHelperPowershell, runCommand.
	if checked < 3 {
		t.Errorf("found only %d command runners, expected at least 3 -- this guard "+
			"has lost its reach, not the code its bound", checked)
	}
}

func callsRun(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" || len(call.Args) != 0 {
			return true
		}
		found = true
		return false
	})
	return found
}

// setsWaitDelay reports whether the body assigns a NON-ZERO WaitDelay.
//
// The value check is the point. An earlier version of this guard asserted only
// that some assignment to a field named WaitDelay existed, which `WaitDelay = 0`
// satisfies while being functionally identical to never setting it -- exec
// treats zero as "no delay", so the wedge this whole guard exists to prevent
// would sail through green. Found by Hawkeye reviewing the guard itself.
//
// This is syntactic, not type-checked: go/parser carries no type information, so
// it cannot tell util.Command's field from any other and cannot see dead code.
// That is an accepted limit -- the guard has to run without build constraints to
// reach the Windows file at all, which is its whole reason to exist. Rejecting
// the cheap no-op closes the bypass anyone would actually reach for.
func setsWaitDelay(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WaitDelay" {
				continue
			}
			if i < len(assign.Rhs) && isZeroDuration(assign.Rhs[i]) {
				continue
			}
			found = true
			return false
		}
		return true
	})
	return found
}

// isZeroDuration matches the shapes that mean "no bound": a bare 0, and a
// conversion of one such as time.Duration(0).
func isZeroDuration(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.INT && v.Value == "0"
	case *ast.CallExpr:
		return len(v.Args) == 1 && isZeroDuration(v.Args[0])
	case *ast.ParenExpr:
		return isZeroDuration(v.X)
	}
	return false
}

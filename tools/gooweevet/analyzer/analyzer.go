// Package analyzer is gooweevet's go/analysis pass (#147): static checks for
// goowee's reactivity rules, the ones the compiler can't see. Each
// diagnostic names the AGENTS.md golden rule it enforces.
//
//   - snapshot: sig.Get() passed to a static helper (Text, Class, …) in a
//     component body, where it is read once and never updates (rule 1).
//   - deps: Computed, UseEffect, Watch or UseScope reading a signal with Get
//     that its deps don't list, so it won't re-run when that signal changes
//     (rule 3). Peek is the opt-out for an intended snapshot.
//   - offloop: a signal Set, or a UseState setter, called in a goroutine or
//     a time.AfterFunc callback outside core.Schedule (rule 4).
package analyzer

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const (
	corePkg  = "github.com/yogisalomo/goowee/core"
	hPkg     = "github.com/yogisalomo/goowee/h"
	hooksPkg = "github.com/yogisalomo/goowee/hooks"
)

// Analyzer is the gooweevet pass.
var Analyzer = &analysis.Analyzer{
	Name:     "gooweevet",
	Doc:      "check goowee's reactivity rules: snapshot reads in components, undeclared dependencies, signal writes off the render loop",
	URL:      "https://github.com/yogisalomo/goowee/blob/main/AGENTS.md#golden-rules-each-prevents-a-common-mistake",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// staticHelpers are h's constructors that take a plain value, each with its
// reactive counterpart.
var staticHelpers = map[string]string{
	"Text":     "TextS(sig) or Textf",
	"Class":    "ClassS(sig) or ClassIf",
	"Style":    "StyleS(sig)",
	"Value":    "ValueS(sig) or BindValue(sig)",
	"Href":     "HrefS(sig)",
	"Src":      "SrcS(sig)",
	"Checked":  "CheckedS(sig) or BindChecked(sig)",
	"Disabled": "DisabledS(sig)",
}

// depFuncs are the functions whose callback must list what it reads, with
// the index of the deps argument and of the callback.
var depFuncs = map[string]struct{ deps, fn int }{
	corePkg + ".Computed":   {0, 1},
	hooksPkg + ".UseEffect": {0, 1},
	hooksPkg + ".Watch":     {0, 1},
}

func run(pass *analysis.Pass) (any, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	ins.Preorder([]ast.Node{(*ast.CallExpr)(nil), (*ast.GoStmt)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.CallExpr:
			fn := calledFunc(pass.TypesInfo, n)
			if fn == nil || fn.Pkg() == nil {
				return
			}
			full := fn.Pkg().Path() + "." + fn.Name()
			switch {
			case full == corePkg+".Component" && len(n.Args) == 2:
				checkSnapshots(pass, n.Args[1])
			case full == corePkg+".ComponentWithProps" && len(n.Args) >= 2:
				checkSnapshots(pass, n.Args[len(n.Args)-1])
			case full == hooksPkg+".UseScope" && len(n.Args) >= 1:
				checkDeps(pass, fn.Name(), n.Args[0], n.Args[1:])
			case full == "time.AfterFunc" && len(n.Args) == 2:
				checkOffLoop(pass, n.Args[1], "a time.AfterFunc callback")
			default:
				if d, ok := depFuncs[full]; ok && len(n.Args) > d.fn {
					if deps, ok := depList(pass.TypesInfo, n.Args[d.deps]); ok {
						checkDeps(pass, fn.Name(), n.Args[d.fn], deps)
					}
				}
			}
		case *ast.GoStmt:
			checkOffLoop(pass, n.Call, "a goroutine")
		}
	})
	return nil, nil
}

// calledFunc is the function a call statically calls, generic or not.
func calledFunc(info *types.Info, call *ast.CallExpr) *types.Func {
	fun := ast.Unparen(call.Fun)
	if ix, ok := fun.(*ast.IndexExpr); ok {
		fun = ix.X
	} else if ix, ok := fun.(*ast.IndexListExpr); ok {
		fun = ix.X
	}
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return nil
	}
	fn, _ := info.Uses[id].(*types.Func)
	return fn
}

// isSignal reports whether t is *core.Signal[T].
func isSignal(t types.Type) bool {
	p, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := p.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Origin().Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == corePkg && obj.Name() == "Signal"
}

// signalCall reports the receiver of x.Get() (or Set, Peek) when x is a
// signal: the call's receiver expression.
func signalMethod(info *types.Info, call *ast.CallExpr, method string) (ast.Expr, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method {
		return nil, false
	}
	t := info.TypeOf(sel.X)
	if t == nil || !isSignal(t) {
		return nil, false
	}
	return sel.X, true
}

// checkSnapshots reports h.Text(sig.Get()) and the like in a component's
// render function. Nested function literals (handlers, Show content) are
// left alone: they run later, or re-run.
func checkSnapshots(pass *analysis.Pass, render ast.Expr) {
	lit, ok := ast.Unparen(render).(*ast.FuncLit)
	if !ok {
		return
	}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := calledFunc(pass.TypesInfo, call)
		if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != hPkg {
			return true
		}
		suggest, ok := staticHelpers[fn.Name()]
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			ac, ok := ast.Unparen(arg).(*ast.CallExpr)
			if !ok {
				continue
			}
			if recv, ok := signalMethod(pass.TypesInfo, ac, "Get"); ok {
				pass.Reportf(arg.Pos(), "%s(%s.Get()) reads the signal once: the component body runs once, so it never updates; use %s (AGENTS.md rule 1)",
					fn.Name(), exprString(recv), suggest)
			}
		}
		return true
	})
}

// depList returns the expressions of a deps argument written as a slice
// literal or core.Deps(...) call; ok is false for anything else (a variable),
// which the check can't see into.
func depList(info *types.Info, arg ast.Expr) ([]ast.Expr, bool) {
	switch a := ast.Unparen(arg).(type) {
	case *ast.CompositeLit:
		return a.Elts, true
	case *ast.Ident:
		if a.Name == "nil" {
			return nil, true
		}
	case *ast.CallExpr:
		if fn := calledFunc(info, a); fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == corePkg && fn.Name() == "Deps" {
			return a.Args, true
		}
	}
	return nil, false
}

// checkDeps reports Get calls in fn's body on signals deps doesn't list. A
// signal declared inside fn (a local Computed) isn't a dependency.
func checkDeps(pass *analysis.Pass, name string, fn ast.Expr, deps []ast.Expr) {
	lit, ok := ast.Unparen(fn).(*ast.FuncLit)
	if !ok {
		return
	}
	declared := map[string]bool{}
	for _, d := range deps {
		declared[exprString(d)] = true
	}
	reported := map[string]bool{}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		// A function literal inside runs later (a handler, a frame callback, a
		// goroutine), not as part of this evaluation.
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// A nested Computed/Watch/UseEffect has deps of its own.
		if f := calledFunc(pass.TypesInfo, call); f != nil && f.Pkg() != nil {
			if _, nested := depFuncs[f.Pkg().Path()+"."+f.Name()]; nested {
				return false
			}
		}
		recv, ok := signalMethod(pass.TypesInfo, call, "Get")
		if !ok {
			return true
		}
		key := exprString(recv)
		if declared[key] || reported[key] || declaredInside(pass.TypesInfo, recv, lit) {
			return true
		}
		reported[key] = true
		pass.Reportf(call.Pos(), "%s reads %s.Get() but doesn't list %s in its deps, so it won't re-run when it changes; add it, or use Peek for a snapshot (AGENTS.md rule 3)",
			name, key, key)
		return true
	})
}

// declaredInside reports whether the signal expression's root variable is
// declared within lit (a local signal, or a range/closure variable).
func declaredInside(info *types.Info, e ast.Expr, lit *ast.FuncLit) bool {
	for {
		switch x := ast.Unparen(e).(type) {
		case *ast.SelectorExpr:
			e = x.X
			continue
		case *ast.IndexExpr:
			e = x.X
			continue
		case *ast.Ident:
			obj := info.ObjectOf(x)
			return obj != nil && obj.Pos() >= lit.Pos() && obj.Pos() < lit.End()
		}
		return false
	}
}

// checkOffLoop reports signal writes in a function that runs off the render
// loop, unless they're inside a core.Schedule callback.
func checkOffLoop(pass *analysis.Pass, fn ast.Expr, where string) {
	var body *ast.BlockStmt
	switch f := ast.Unparen(fn).(type) {
	case *ast.FuncLit:
		body = f.Body
	case *ast.CallExpr: // go func() { … }()
		if lit, ok := ast.Unparen(f.Fun).(*ast.FuncLit); ok {
			body = lit.Body
		}
	}
	if body == nil {
		return
	}
	setters := setterVars(pass)
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if f := calledFunc(pass.TypesInfo, call); f != nil && f.Pkg() != nil && f.Pkg().Path() == corePkg && f.Name() == "Schedule" {
			return false // runs on the render loop
		}
		if recv, ok := signalMethod(pass.TypesInfo, call, "Set"); ok {
			pass.Reportf(call.Pos(), "%s.Set in %s races the render loop; wrap it in core.Schedule(func() { … }) (AGENTS.md rule 4)", exprString(recv), where)
			return true
		}
		if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && setters[pass.TypesInfo.ObjectOf(id)] {
			pass.Reportf(call.Pos(), "%s, a UseState setter, called in %s races the render loop; wrap it in core.Schedule(func() { … }) (AGENTS.md rule 4)", id.Name, where)
		}
		return true
	})
}

// setterVars finds the setters of `x, setX := hooks.UseState(…)` in the
// package.
func setterVars(pass *analysis.Pass) map[types.Object]bool {
	out := map[types.Object]bool{}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
				return true
			}
			call, ok := ast.Unparen(as.Rhs[0]).(*ast.CallExpr)
			if !ok {
				return true
			}
			if fn := calledFunc(pass.TypesInfo, call); fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == hooksPkg && fn.Name() == "UseState" {
				if id, ok := as.Lhs[1].(*ast.Ident); ok {
					if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
						out[obj] = true
					}
				}
			}
			return true
		})
	}
	return out
}

func exprString(e ast.Expr) string {
	var b strings.Builder
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch x := ast.Unparen(e).(type) {
		case *ast.Ident:
			b.WriteString(x.Name)
		case *ast.SelectorExpr:
			walk(x.X)
			b.WriteString(".")
			b.WriteString(x.Sel.Name)
		case *ast.CallExpr:
			walk(x.Fun)
			b.WriteString("()")
		case *ast.IndexExpr:
			walk(x.X)
			b.WriteString("[…]")
		case *ast.StarExpr:
			b.WriteString("*")
			walk(x.X)
		default:
			b.WriteString("…")
		}
	}
	walk(e)
	return b.String()
}

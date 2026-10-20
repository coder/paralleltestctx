package paralleltestctx

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// localClosures records the function literals assigned to local variables in
// a test function, so a call to one can be checked for the timeout contexts
// it captures.
type localClosures struct {
	pass     *analysis.Pass
	lits     map[types.Object][]*ast.FuncLit
	captures map[types.Object]map[types.Object]struct{}
}

func collectLocalClosures(pass *analysis.Pass, body *ast.BlockStmt) *localClosures {
	c := &localClosures{
		pass:     pass,
		lits:     map[types.Object][]*ast.FuncLit{},
		captures: map[types.Object]map[types.Object]struct{}{},
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) == len(n.Rhs) {
				for i := range n.Rhs {
					c.add(n.Lhs[i], n.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for i := range n.Values {
					c.add(n.Names[i], n.Values[i])
				}
			}
		}
		return true
	})
	return c
}

func (c *localClosures) add(lhs, rhs ast.Expr) {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return
	}
	fl, ok := ast.Unparen(rhs).(*ast.FuncLit)
	if !ok {
		return
	}
	v, ok := c.pass.TypesInfo.ObjectOf(id).(*types.Var)
	if !ok {
		return
	}
	c.lits[v] = append(c.lits[v], fl)
}

// callee returns the local closure variable that call invokes, or nil.
func (c *localClosures) callee(call *ast.CallExpr) types.Object {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	obj := c.pass.TypesInfo.Uses[id]
	if _, ok := c.lits[obj]; !ok {
		return nil
	}
	return obj
}

// capturedVars returns the variables declared outside the closure that its
// body references, directly or through other local closures.
func (c *localClosures) capturedVars(obj types.Object) map[types.Object]struct{} {
	if got, ok := c.captures[obj]; ok {
		return got
	}
	out := map[types.Object]struct{}{}
	c.collectCaptures(obj, map[types.Object]bool{}, out)
	c.captures[obj] = out
	return out
}

func (c *localClosures) collectCaptures(obj types.Object, seen map[types.Object]bool, out map[types.Object]struct{}) {
	if seen[obj] {
		return
	}
	seen[obj] = true
	for _, fl := range c.lits[obj] {
		ast.Inspect(fl.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			v, ok := c.pass.TypesInfo.Uses[id].(*types.Var)
			if !ok || v.IsField() || (v.Pos() >= fl.Pos() && v.Pos() < fl.End()) {
				return true
			}
			out[v] = struct{}{}
			if _, ok := c.lits[v]; ok {
				c.collectCaptures(v, seen, out)
			}
			return true
		})
	}
}

// checkClosureCall reports a call after a t.Parallel call to a local closure
// that captures a timeout context created before the t.Parallel call.
func (a *ctxAnalyzer) checkClosureCall(pass *analysis.Pass, call *ast.CallExpr, parallelCalls []ast.Node, timeoutCtxs []timeoutContext, closures *localClosures) {
	if len(parallelCalls) == 0 {
		return
	}
	obj := closures.callee(call)
	if obj == nil {
		return
	}
	captured := closures.capturedVars(obj)
	callPos := pass.Fset.Position(call.Pos()).Offset

	reported := map[types.Object]bool{}
	for _, ctx := range timeoutCtxs {
		if _, ok := captured[ctx.obj]; !ok || reported[ctx.obj] {
			continue
		}
		if ctx.invalidPos != 0 && pass.Fset.Position(ctx.invalidPos).Offset < callPos {
			continue
		}
		contextPos := pass.Fset.Position(ctx.pos).Offset
		for _, parallelCall := range parallelCalls {
			parallelPos := pass.Fset.Position(parallelCall.Pos()).Offset
			if contextPos < parallelPos && callPos > parallelPos {
				reported[ctx.obj] = true
				pass.Reportf(call.Pos(), "timeout context %s used after a t.Parallel call via closure %s", ctx.obj.Name(), obj.Name())
				break
			}
		}
	}
}

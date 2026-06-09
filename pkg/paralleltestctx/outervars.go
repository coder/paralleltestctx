package paralleltestctx

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// analyzeOuterVarReassignments flags an outer-scope variable that is
// reassigned with `=` inside a parallel subtest scope. Sibling parallel
// subtests can race on the shared variable, and the fix is almost always
// to shadow with `:=`.
//
// Constraints (deliberately tight to keep the FP rate low):
//
//  1. The enclosing scope must contain a t.Parallel() call.
//  2. The assignment must use `=`, not `:=`.
//  3. The LHS must be an *ast.Ident (not a SelectorExpr, IndexExpr, etc.).
//  4. The Ident must resolve to a *types.Var declared OUTSIDE this scope.
//  5. The object must not already be tracked as a timeout context — the
//     existing context-specific diagnostic covers that case.
func (a *ctxAnalyzer) analyzeOuterVarReassignments(
	pass *analysis.Pass,
	scope ast.Node,
	parallelCalls []ast.Node,
	timeoutCtxObjs map[types.Object]struct{},
) {
	if len(parallelCalls) == 0 {
		return
	}
	scopeStart := scope.Pos()
	scopeEnd := scope.End()

	ast.Inspect(scope, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok && ast.Node(fl) != scope {
			// Nested subtest body has its own scope; analyzed separately.
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.ASSIGN {
			return true
		}
		for _, lhs := range as.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			obj := pass.TypesInfo.Uses[id]
			if obj == nil {
				continue
			}
			v, ok := obj.(*types.Var)
			if !ok {
				continue
			}
			if v.Pos() >= scopeStart && v.Pos() <= scopeEnd {
				continue
			}
			if _, isCtx := timeoutCtxObjs[obj]; isCtx {
				continue
			}
			pass.Reportf(
				as.Pos(),
				"outer-scope variable %s reassigned inside a parallel subtest; did you mean to shadow with := ?",
				id.Name,
			)
		}
		return true
	})
}

package paralleltestctx

import (
	"flag"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

const Doc = `warn when a context with a timeout/deadline is used after a t.Parallel call`

func Analyzer() *analysis.Analyzer {
	return newCtxAnalyzer().analyzer
}

type timeoutFunc struct {
	receiver string // empty string means no receiver specified
	name     string
}

type ctxAnalyzer struct {
	analyzer        *analysis.Analyzer
	timeoutFuncFlag string
	timeoutFuncs    []timeoutFunc // cache for flag parsed w/ defaults
}

func newCtxAnalyzer() *ctxAnalyzer {
	a := &ctxAnalyzer{}
	var flags flag.FlagSet
	flags.StringVar(&a.timeoutFuncFlag, "custom-funcs", "", "comma-separated list of additional function names that create timeout/deadline contexts")
	a.analyzer = &analysis.Analyzer{
		Name:  "paralleltestctx",
		Doc:   Doc,
		Run:   a.run,
		Flags: flags,
	}
	return a
}

// doesCreateTimeoutContext checks if the given call expression creates a
// context with a deadline or timeout
func (a *ctxAnalyzer) doesCreateTimeoutContext(call *ast.CallExpr) bool {
	timeoutFuncs := a.getTimeoutFuncs()

	// Check for receiver calls (e.g. context.WithTimeout, testutil.Context)
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		funcName := se.Sel.Name
		receiverName := extractReceiverName(se.X)

		for _, fn := range timeoutFuncs {
			if fn.name == funcName {
				if fn.receiver == "" || fn.receiver == receiverName {
					return true
				}
			}
		}
	}

	// Check for direct function calls (e.g., Context when no receiver specified)
	if id, ok := call.Fun.(*ast.Ident); ok {
		funcName := id.Name
		for _, fn := range timeoutFuncs {
			if fn.receiver == "" && fn.name == funcName {
				return true
			}
		}
	}

	return false
}

type timeoutHelperSet map[types.Object]struct{}

func (a *ctxAnalyzer) collectTimeoutHelpers(pass *analysis.Pass, testFiles []*ast.File) timeoutHelperSet {
	candidates := map[*ast.FuncDecl]types.Object{}
	for _, file := range testFiles {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !funcReturnsContext(pass, fd) {
				continue
			}

			obj := pass.TypesInfo.Defs[fd.Name]
			if obj == nil {
				continue
			}
			candidates[fd] = obj
		}
	}

	helpers := timeoutHelperSet{}
	for changed := true; changed; {
		changed = false
		for fd, obj := range candidates {
			if _, ok := helpers[obj]; ok {
				continue
			}
			if a.functionReturnsTimeoutContext(pass, fd, helpers) {
				helpers[obj] = struct{}{}
				changed = true
			}
		}
	}
	return helpers
}

func funcReturnsContext(pass *analysis.Pass, fd *ast.FuncDecl) bool {
	if fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
		return false
	}

	return isContextType(pass, pass.TypesInfo.TypeOf(fd.Type.Results.List[0].Type))
}

func (a *ctxAnalyzer) functionReturnsTimeoutContext(pass *analysis.Pass, fd *ast.FuncDecl, helpers timeoutHelperSet) bool {
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if found {
			return false
		}

		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(n.Results) == 0 {
				return true
			}
			found = a.exprContainsTimeoutContext(pass, n.Results[0], helpers)
			return !found
		default:
			return true
		}
	})
	return found
}

func (a *ctxAnalyzer) exprContainsTimeoutContext(pass *analysis.Pass, expr ast.Expr, helpers timeoutHelperSet) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		found = a.doesCreateTimeoutContext(call) || callsTimeoutHelper(pass, call, helpers)
		return !found
	})
	return found
}

func callsTimeoutHelper(pass *analysis.Pass, call *ast.CallExpr, helpers timeoutHelperSet) bool {
	if len(helpers) == 0 {
		return false
	}

	obj := callObject(pass, call)
	if obj == nil {
		return false
	}
	_, ok := helpers[obj]
	return ok
}

func callObject(pass *analysis.Pass, call *ast.CallExpr) types.Object {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[fun]
	case *ast.SelectorExpr:
		return pass.TypesInfo.Uses[fun.Sel]
	default:
		return nil
	}
}

func isContextType(pass *analysis.Pass, typ types.Type) bool {
	if typ == nil {
		return false
	}

	contextIface := contextInterface(pass)
	if contextIface == nil {
		return isStdlibContextType(typ)
	}

	return types.Implements(types.Unalias(typ), contextIface)
}

func contextInterface(pass *analysis.Pass) *types.Interface {
	if pass.Pkg == nil {
		return nil
	}

	for _, pkg := range pass.Pkg.Imports() {
		if pkg.Path() != "context" {
			continue
		}

		obj := pkg.Scope().Lookup("Context")
		if obj == nil {
			return nil
		}

		named, ok := obj.Type().(*types.Named)
		if !ok {
			return nil
		}

		iface, ok := named.Underlying().(*types.Interface)
		if !ok {
			return nil
		}

		return iface
	}

	return nil
}

func isStdlibContextType(typ types.Type) bool {
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()
	return obj.Name() == "Context" && obj.Pkg() != nil && obj.Pkg().Path() == "context"
}

func (a *ctxAnalyzer) getTimeoutFuncs() []timeoutFunc {
	if a.timeoutFuncs != nil {
		return a.timeoutFuncs
	}

	// Always include standard timeout functions (context.WithTimeout, context.WithDeadline)
	result := []timeoutFunc{
		{receiver: "context", name: "WithTimeout"},
		{receiver: "context", name: "WithDeadline"},
	}

	if a.timeoutFuncFlag != "" {
		for f := range strings.SplitSeq(a.timeoutFuncFlag, ",") {
			if trimmed := strings.TrimSpace(f); trimmed != "" {
				if parts := strings.Split(trimmed, "."); len(parts) == 2 {
					// receiver.function format (e.g., "testutil.Context")
					result = append(result, timeoutFunc{receiver: parts[0], name: parts[1]})
				} else {
					// bare function name (e.g., "Context")
					result = append(result, timeoutFunc{receiver: "", name: trimmed})
				}
			}
		}
	}

	a.timeoutFuncs = result // cache the result
	return result
}

// filterTestFiles returns only the test files from the pass
func filterTestFiles(pass *analysis.Pass) []*ast.File {
	var testFiles []*ast.File
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			testFiles = append(testFiles, file)
		}
	}
	return testFiles
}

func (a *ctxAnalyzer) run(pass *analysis.Pass) (any, error) {
	testFiles := filterTestFiles(pass)
	if len(testFiles) == 0 {
		return nil, nil
	}

	timeoutHelpers := a.collectTimeoutHelpers(pass, testFiles)

	insp := inspector.New(testFiles)
	nodeFilter := []ast.Node{(*ast.FuncDecl)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if !isTestFunction(fd) {
			return
		}

		a.analyzeTestFunction(pass, fd, timeoutHelpers)
	})
	return nil, nil
}

// analyzeTestFunction analyzes a test function to find timeout context usage after t.Parallel calls
func (a *ctxAnalyzer) analyzeTestFunction(pass *analysis.Pass, fd *ast.FuncDecl, helpers timeoutHelperSet) {
	testVarName := testParamName(fd)
	if testVarName == "" {
		return
	}

	// Collect all timeout contexts and their positions
	timeoutCtxs := a.collectTimeoutContexts(pass, fd, helpers)

	timeoutCtxObjs := make(map[types.Object]struct{}, len(timeoutCtxs))
	for _, c := range timeoutCtxs {
		timeoutCtxObjs[c.obj] = struct{}{}
	}

	// Find all t.Parallel() calls and check for context usage after them
	a.analyzeScope(pass, fd.Body, testVarName, timeoutCtxs, timeoutCtxObjs)
}

func isTestFunction(fd *ast.FuncDecl) bool {
	if !strings.HasPrefix(fd.Name.Name, "Test") {
		return false
	}
	if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
		return false
	}
	p := fd.Type.Params.List[0]
	se, ok := p.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := se.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "testing" {
		return false
	}
	return true
}

func testParamName(fd *ast.FuncDecl) string {
	p := fd.Type.Params.List[0]
	if len(p.Names) == 0 {
		return ""
	}
	return p.Names[0].Name
}

// timeoutContext represents a context variable created with a timeout/deadline
type timeoutContext struct {
	obj        types.Object
	pos        token.Pos // position where it was created
	invalidPos token.Pos // position where it was overwritten with non-timeout context (0 if still valid)
}

// collectTimeoutContexts finds all context variables created with a timeout/deadline functions
func (a *ctxAnalyzer) collectTimeoutContexts(pass *analysis.Pass, fd *ast.FuncDecl, helpers timeoutHelperSet) []timeoutContext {
	var contexts []timeoutContext
	contextMap := make(map[types.Object]*timeoutContext)

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(as.Lhs) == 0 || len(as.Rhs) == 0 {
			return true
		}

		id, ok := as.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.ObjectOf(id)
		if obj == nil {
			return true
		}

		if !isContextType(pass, obj.Type()) {
			return true
		}

		if a.exprContainsTimeoutContext(pass, as.Rhs[0], helpers) {
			// This is a timeout context creation
			ctx := timeoutContext{
				obj: obj,
				pos: as.Pos(),
			}
			contexts = append(contexts, ctx)
			contextMap[obj] = &contexts[len(contexts)-1]
		} else if existingCtx, exists := contextMap[obj]; exists {
			// This is an overwrite of an existing timeout context with non-timeout
			existingCtx.invalidPos = as.Pos()
		}

		return true
	})
	return contexts
}

// analyzeScope analyzes a specific scope (test function or subtest) for the pattern
func (a *ctxAnalyzer) analyzeScope(pass *analysis.Pass, scope ast.Node, testVarName string, timeoutCtxs []timeoutContext, timeoutCtxObjs map[types.Object]struct{}) {
	var parallelCalls []ast.Node
	reportedNodes := make(map[ast.Node]bool) // Track nodes we've already reported

	ast.Inspect(scope, func(n ast.Node) bool {
		// Don't descend into nested function literals (they have their own scopes)
		if fl, ok := n.(*ast.FuncLit); ok && fl != scope {
			// Analyze the subtest scope separately with its own test variable
			subtestVarName := a.getSubtestParamName(fl)
			if subtestVarName != "" {
				a.analyzeScope(pass, fl, subtestVarName, timeoutCtxs, timeoutCtxObjs)
			}
			return false // Don't continue into this scope
		}

		// Find t.Parallel() calls in this scope
		if isTestMethodCall(n, testVarName, "Parallel") {
			parallelCalls = append(parallelCalls, n)
		}

		// Check for context reassignment (overwriting)
		if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.ASSIGN {
			if len(as.Lhs) > 0 {
				if id, ok := as.Lhs[0].(*ast.Ident); ok {
					if a.checkContextViolation(pass, id, n, parallelCalls, timeoutCtxs, true) {
						reportedNodes[id] = true
						return true
					}
				}
			}
		}

		// Check if any timeout context is used in this scope (regular usage, not assignment)
		if id, ok := n.(*ast.Ident); ok {
			// Skip if we've already reported this identifier
			if reportedNodes[id] {
				return true
			}

			if a.checkContextViolation(pass, id, n, parallelCalls, timeoutCtxs, false) {
				return true
			}
		}

		return true
	})

	a.analyzeOuterVarReassignments(pass, scope, parallelCalls, timeoutCtxObjs)
}

// checkContextViolation checks if a context identifier violates the t.Parallel usage rules
func (a *ctxAnalyzer) checkContextViolation(pass *analysis.Pass, id *ast.Ident, node ast.Node, parallelCalls []ast.Node, timeoutCtxs []timeoutContext, isAssignment bool) bool {
	obj := pass.TypesInfo.ObjectOf(id)
	if obj == nil {
		return false
	}

	// Check if this identifier references a timeout context
	for _, ctx := range timeoutCtxs {
		if ctx.obj == obj {
			// Check if this usage/assignment is after any parallel call in the same scope
			for _, parallelCall := range parallelCalls {
				parallelPos := pass.Fset.Position(parallelCall.Pos()).Offset
				nodePos := pass.Fset.Position(node.Pos()).Offset
				contextPos := pass.Fset.Position(ctx.pos).Offset

				// Context created before parallel call, but used/assigned after parallel call
				if contextPos < parallelPos && nodePos > parallelPos {
					// If context was invalidated (overwritten with non-timeout), check if that happened before this usage
					if ctx.invalidPos != 0 {
						invalidOffset := pass.Fset.Position(ctx.invalidPos).Offset
						if invalidOffset < nodePos {
							// Context was invalidated before this usage, so don't warn
							return false
						}
					}

					if isAssignment {
						pass.Reportf(node.Pos(), "timeout context %s overwritten after a t.Parallel call; did you mean to shadow the variable?", id.Name)
					} else {
						pass.Reportf(node.Pos(), "timeout context %s used after a t.Parallel call", id.Name)
					}
					return true
				}
			}
		}
	}
	return false
}

// getSubtestParamName extracts the test parameter name from a function literal
func (a *ctxAnalyzer) getSubtestParamName(fl *ast.FuncLit) string {
	if fl.Type.Params == nil || len(fl.Type.Params.List) == 0 {
		return ""
	}
	p := fl.Type.Params.List[0]
	if len(p.Names) == 0 {
		return ""
	}
	return p.Names[0].Name
}

// extractReceiverName extracts the receiver name from different AST expressions
func extractReceiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.CompositeLit:
		// Handle foo{}.Method() pattern - get the type name
		if id, ok := x.Type.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.ParenExpr:
		// Handle (foo{}).Method() pattern - unwrap parentheses
		if cl, ok := x.X.(*ast.CompositeLit); ok {
			if id, ok := cl.Type.(*ast.Ident); ok {
				return id.Name
			}
		}
	}
	return ""
}

// isTestMethodCall checks if node is a method call on testVar with the given methodName
func isTestMethodCall(node ast.Node, testVar, methodName string) bool {
	ce, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	recv, ok := fun.X.(*ast.Ident)
	if !ok {
		return false
	}
	return recv.Name == testVar && fun.Sel.Name == methodName
}

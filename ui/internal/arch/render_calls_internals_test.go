package arch

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
)

// Values are unions across assignments, so branches, loops and alias cycles cannot
// hide a possible call. Only reachable calls supply parameters: an event's use of a
// shared helper must not become an effect of that helper's projection use.
type renderReachability struct {
	model      renderPackageModel
	parameters map[*types.Var]renderTargets
	paths      map[*renderFunction][]string
	functions  []*renderFunction
	changed    bool
}

func (m renderPackageModel) reachableFrom(entry *renderFunction) *renderReachability {
	r := &renderReachability{
		model:      m,
		parameters: make(map[*types.Var]renderTargets),
		paths:      make(map[*renderFunction][]string),
	}
	r.reach(entry, nil)
	for r.changed {
		r.changed = false
		for _, function := range r.functions {
			inspectRenderBody(function.body, func(node ast.Node) {
				if call := m.invocation(node); call != nil {
					r.followCall(call, r.paths[function])
				}
			})
		}
	}
	return r
}

func (r *renderReachability) reach(function *renderFunction, path []string) {
	if function == nil || r.paths[function] != nil {
		return
	}
	r.paths[function] = append(slices.Clone(path), function.identity())
	r.functions = append(r.functions, function)
	r.changed = true
}

func (r *renderReachability) resolve(source renderSource) renderTargets {
	resolver := renderValueResolver{flow: r, active: make(map[renderSource]bool)}
	return resolver.resolve(source)
}

func (r *renderReachability) callTargets(call *ast.CallExpr) (renderTargets, renderTargets) {
	direct := r.resolve(renderSource{expression: call.Fun})
	callbacks := make(renderTargets)
	opaque := false
	for target := range direct {
		if r.model.localFunction(target) == nil && (target.function != nil || target.callback != nil) {
			opaque = true
		}
	}
	if !opaque {
		return direct, callbacks
	}
	// An imported body is unavailable, so its function-typed parameters may invoke
	// a local closure. Passing a function as ordinary data (for example to Sprintf)
	// does not make that function executable.
	signature, ok := r.model.types.TypeOf(call.Fun).Underlying().(*types.Signature)
	if !ok {
		return direct, callbacks
	}
	for index, argument := range call.Args {
		if callableParameter(signature, index) {
			callbacks.include(r.resolve(renderSource{expression: argument}))
		}
	}
	return direct, callbacks
}

func callableParameter(signature *types.Signature, index int) bool {
	parameters := signature.Params()
	if parameters.Len() == 0 {
		return false
	}
	index = min(index, parameters.Len()-1)
	typ := parameters.At(index).Type()
	if signature.Variadic() && index == parameters.Len()-1 {
		typ = typ.(*types.Slice).Elem()
	}
	_, callable := typ.Underlying().(*types.Signature)
	return callable
}

func (r *renderReachability) followCall(call *ast.CallExpr, path []string) {
	direct, callbacks := r.callTargets(call)
	for _, target := range orderedRenderTargets(direct) {
		if local := r.model.localFunction(target); local != nil {
			r.reach(local, path)
			r.bindArguments(local, target, call, r.resolve)
		}
	}
	for _, target := range orderedRenderTargets(callbacks) {
		r.reach(r.model.localFunction(target), path)
	}
}

func (r *renderReachability) bindArguments(function *renderFunction, target renderTarget, call *ast.CallExpr, resolve func(renderSource) renderTargets) {
	arguments := r.argumentSources(call.Args)
	if target.methodExpression && len(arguments) > 0 {
		arguments = arguments[1:]
	}
	parameters := function.signature.Params()
	for index := range parameters.Len() {
		if index >= len(arguments) {
			break
		}
		parameter := parameters.At(index).Origin()
		if r.parameters[parameter] == nil {
			r.parameters[parameter] = make(renderTargets)
		}
		sources := arguments[index : index+1]
		if function.signature.Variadic() && index == parameters.Len()-1 {
			sources = arguments[index:]
		}
		for _, source := range sources {
			if r.parameters[parameter].include(resolve(source)) {
				r.changed = true
			}
		}
	}
}

func (r *renderReachability) argumentSources(arguments []ast.Expr) []renderSource {
	if len(arguments) == 1 {
		if tuple, ok := r.model.types.TypeOf(arguments[0]).(*types.Tuple); ok {
			sources := make([]renderSource, tuple.Len())
			for index := range sources {
				sources[index] = renderSource{expression: arguments[0], result: index}
			}
			return sources
		}
	}
	sources := make([]renderSource, len(arguments))
	for index, argument := range arguments {
		sources[index] = renderSource{expression: argument}
	}
	return sources
}

func (m renderPackageModel) invocation(node ast.Node) *ast.CallExpr {
	switch node := node.(type) {
	case *ast.CallExpr:
		return node
	case *ast.RangeStmt:
		if _, callable := m.types.TypeOf(node.X).Underlying().(*types.Signature); callable {
			return &ast.CallExpr{Fun: node.X}
		}
	}
	return nil
}

func orderedRenderTargets(targets renderTargets) []renderTarget {
	ordered := make([]renderTarget, 0, len(targets))
	for target := range targets {
		ordered = append(ordered, target)
	}
	slices.SortFunc(ordered, func(a, b renderTarget) int { return strings.Compare(a.identity(), b.identity()) })
	return ordered
}

func (target renderTarget) identity() string {
	if target.function != nil {
		return target.function.FullName()
	}
	if target.closure != nil {
		return target.closure.identity()
	}
	return target.callback.String()
}

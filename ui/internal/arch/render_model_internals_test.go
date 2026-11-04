package arch

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type renderFunction struct {
	name      string
	receiver  string
	body      *ast.BlockStmt
	signature *types.Signature
	returns   [][]renderSource
	set       *token.FileSet
}

func (f *renderFunction) identity() string {
	if f.receiver == "" {
		return f.name
	}
	return f.receiver + "." + f.name
}

type renderPackageModel struct {
	functions         []*renderFunction
	functionByObject  map[*types.Func]*renderFunction
	functionByLiteral map[*ast.FuncLit]*renderFunction
	callbackByObject  map[types.Object]callbackRule
	sources           map[*types.Var][]renderSource
	checkedPackage    *types.Package
	types             *types.Info
}

type renderSource struct {
	expression ast.Expr
	variable   *types.Var
	result     int
}

type renderTarget struct {
	function         *types.Func
	closure          *renderFunction
	callback         types.Object
	methodExpression bool
}

type renderTargets map[renderTarget]bool

func (targets renderTargets) include(other renderTargets) bool {
	changed := false
	for target := range other {
		if !targets[target] {
			targets[target] = true
			changed = true
		}
	}
	return changed
}

func (m renderPackageModel) localFunction(target renderTarget) *renderFunction {
	if target.closure != nil {
		return target.closure
	}
	return m.functionByObject[target.function]
}

func newRenderModel(set *token.FileSet, files []*ast.File, checked *types.Package, info *types.Info, packageKey string) renderPackageModel {
	m := renderPackageModel{
		functionByObject:  make(map[*types.Func]*renderFunction),
		functionByLiteral: make(map[*ast.FuncLit]*renderFunction),
		callbackByObject:  make(map[types.Object]callbackRule),
		sources:           make(map[*types.Var][]renderSource),
		checkedPackage:    checked,
		types:             info,
	}
	for _, file := range files {
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Body != nil {
				object := info.Defs[function.Name].(*types.Func).Origin()
				renderer := &renderFunction{
					name: function.Name.Name, body: function.Body,
					signature: object.Type().(*types.Signature), set: set,
				}
				if receiver := renderer.signature.Recv(); receiver != nil {
					renderer.receiver = types.TypeString(receiver.Type(), func(*types.Package) string { return "" })
				}
				m.functions = append(m.functions, renderer)
				m.functionByObject[object] = renderer
			}
			m.collectCallbackRules(declaration, packageKey)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.FuncLit:
				function := &renderFunction{
					name: fmt.Sprintf("closure at %s", set.Position(node.Pos())),
					body: node.Body, signature: info.TypeOf(node).(*types.Signature), set: set,
				}
				m.functionByLiteral[node] = function
				m.functions = append(m.functions, function)
			case *ast.AssignStmt:
				for index, left := range node.Lhs {
					m.addSource(m.variableOf(left), renderResult(node.Rhs, index))
				}
			case *ast.ValueSpec:
				for index, name := range node.Names {
					m.addSource(m.variableOf(name), renderResult(node.Values, index))
				}
			case *ast.RangeStmt:
				m.addSource(m.variableOf(node.Value), renderSource{expression: node.X})
			case *ast.CompositeLit:
				m.collectFieldSources(node)
			}
			return true
		})
	}
	for _, function := range m.functions {
		function.collectReturns()
	}
	return m
}

func (m renderPackageModel) collectCallbackRules(declaration ast.Decl, packageKey string) {
	general, ok := declaration.(*ast.GenDecl)
	if !ok || general.Tok != token.TYPE {
		return
	}
	for _, specification := range general.Specs {
		typeSpec := specification.(*ast.TypeSpec)
		key := packageKey + ":" + typeSpec.Name.Name
		switch declared := typeSpec.Type.(type) {
		case *ast.FuncType:
			if rule, exists := renderingCallbacks[key]; exists {
				m.callbackByObject[m.types.Defs[typeSpec.Name]] = rule
			}
		case *ast.StructType:
			for _, field := range declared.Fields.List {
				for _, name := range field.Names {
					if rule, exists := renderingCallbacks[key+"."+name.Name]; exists {
						m.callbackByObject[m.types.Defs[name]] = rule
					}
				}
			}
		}
	}
}

func (m renderPackageModel) addSource(variable *types.Var, source renderSource) {
	if variable != nil && (source.expression != nil || source.variable != nil) {
		variable = variable.Origin()
		m.sources[variable] = append(m.sources[variable], source)
	}
}

func (m renderPackageModel) variableOf(expression ast.Expr) *types.Var {
	var object types.Object
	switch expression := expression.(type) {
	case *ast.Ident:
		object = m.types.Defs[expression]
		if object == nil {
			object = m.types.Uses[expression]
		}
	case *ast.SelectorExpr:
		object = m.selectedObject(expression)
	case *ast.ParenExpr:
		return m.variableOf(expression.X)
	case *ast.StarExpr:
		return m.variableOf(expression.X)
	}
	variable, _ := object.(*types.Var)
	return variable
}

func (m renderPackageModel) selectedObject(selector *ast.SelectorExpr) types.Object {
	if selection := m.types.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return m.types.Uses[selector.Sel]
}

func (m renderPackageModel) collectFieldSources(literal *ast.CompositeLit) {
	structure, ok := m.types.TypeOf(literal).Underlying().(*types.Struct)
	if !ok {
		return
	}
	for index, element := range literal.Elts {
		expression := element
		field := structure.Field(index)
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			expression = keyed.Value
			field = m.variableOf(keyed.Key)
		}
		m.addSource(field, renderSource{expression: expression})
	}
}

func renderResult(expressions []ast.Expr, index int) renderSource {
	if len(expressions) == 1 {
		return renderSource{expression: expressions[0], result: index}
	}
	if index < len(expressions) {
		return renderSource{expression: expressions[index]}
	}
	return renderSource{}
}

func (f *renderFunction) collectReturns() {
	f.returns = make([][]renderSource, f.signature.Results().Len())
	inspectRenderBody(f.body, func(node ast.Node) {
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return
		}
		for index := range f.returns {
			source := renderResult(returned.Results, index)
			if len(returned.Results) == 0 {
				source.variable = f.signature.Results().At(index)
			}
			f.returns[index] = append(f.returns[index], source)
		}
	})
}

func inspectRenderBody(body *ast.BlockStmt, inspect func(ast.Node)) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		inspect(node)
		return true
	})
}

type renderValueResolver struct {
	flow   *renderReachability
	active map[renderSource]bool
}

func (r *renderValueResolver) resolve(source renderSource) renderTargets {
	targets := make(renderTargets)
	if r.active[source] {
		return targets
	}
	r.active[source] = true
	defer delete(r.active, source)
	if source.variable != nil {
		variable := source.variable.Origin()
		targets.include(r.flow.parameters[variable])
		for _, assignment := range r.flow.model.sources[variable] {
			targets.include(r.resolve(assignment))
		}
		r.addCallback(targets, variable)
		r.addTypedCallback(targets, variable.Type())
		return targets
	}
	r.resolveExpression(targets, source)
	if source.expression != nil {
		r.addTypedCallback(targets, r.flow.model.types.TypeOf(source.expression))
	}
	return targets
}

func (r *renderValueResolver) resolveExpression(targets renderTargets, source renderSource) {
	forward := func(expression ast.Expr) {
		targets.include(r.resolve(renderSource{expression: expression, result: source.result}))
	}
	switch expression := source.expression.(type) {
	case *ast.Ident:
		r.addObject(targets, r.flow.model.types.Uses[expression], false)
	case *ast.SelectorExpr:
		selection := r.flow.model.types.Selections[expression]
		methodExpression := selection != nil && selection.Kind() == types.MethodExpr
		r.addObject(targets, r.flow.model.selectedObject(expression), methodExpression)
	case *ast.ParenExpr:
		forward(expression.X)
	case *ast.IndexExpr:
		forward(expression.X)
	case *ast.IndexListExpr:
		forward(expression.X)
	case *ast.TypeAssertExpr:
		forward(expression.X)
	case *ast.UnaryExpr:
		forward(expression.X)
	case *ast.StarExpr:
		forward(expression.X)
	case *ast.FuncLit:
		targets[renderTarget{closure: r.flow.model.functionByLiteral[expression]}] = true
	case *ast.CompositeLit:
		for _, element := range expression.Elts {
			if keyed, ok := element.(*ast.KeyValueExpr); ok {
				element = keyed.Value
			}
			forward(element)
		}
	case *ast.CallExpr:
		if r.flow.model.types.Types[expression.Fun].IsType() && len(expression.Args) == 1 {
			forward(expression.Args[0])
			return
		}
		for target := range r.resolve(renderSource{expression: expression.Fun}) {
			function := r.flow.model.localFunction(target)
			if function == nil || source.result >= len(function.returns) {
				continue
			}
			r.flow.bindArguments(function, target, expression, r.resolve)
			for _, returned := range function.returns[source.result] {
				targets.include(r.resolve(returned))
			}
		}
	}
}

func (r *renderValueResolver) addObject(targets renderTargets, object types.Object, methodExpression bool) {
	switch object := object.(type) {
	case *types.Func:
		targets[renderTarget{function: object.Origin(), methodExpression: methodExpression}] = true
	case *types.Var:
		targets.include(r.resolve(renderSource{variable: object.Origin()}))
	}
}

func (r *renderValueResolver) addCallback(targets renderTargets, object types.Object) {
	if rule, exists := r.flow.model.callbackByObject[object]; exists && rule.phase != callbackProjection {
		targets[renderTarget{callback: object}] = true
	}
}

func (r *renderValueResolver) addTypedCallback(targets renderTargets, typ types.Type) {
	if typ == nil {
		return
	}
	if named, ok := types.Unalias(typ).(*types.Named); ok {
		r.addCallback(targets, named.Origin().Obj())
	}
}

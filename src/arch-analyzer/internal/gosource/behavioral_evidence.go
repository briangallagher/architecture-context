package gosource

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/jctanner/arch-analyzer/internal/model"
)

const (
	conditionalMetricsKind = "conditional-metrics-enforcement"
	namedWatchKind         = "named-watch-predicate"
	behaviorObserved       = "observed"
	behaviorUnresolved     = "unresolved"
)

func extractBehavioralEvidence(
	files []sourceFile,
	repository repositoryOptionBindings,
) []model.BehavioralEvidence {
	index := buildRepositoryGoIndex(files)
	apiPackages := repositoryAPIPackages(files)
	var result []model.BehavioralEvidence
	for _, file := range files {
		result = append(result, extractConditionalMetricsEvidence(file, repository)...)
		result = append(result, extractNamedWatchEvidence(file, index, apiPackages)...)
	}
	return dedupeBehavioralEvidence(result)
}

func extractConditionalMetricsEvidence(
	file sourceFile,
	repository repositoryOptionBindings,
) []model.BehavioralEvidence {
	var result []model.BehavioralEvidence
	ast.Inspect(file.file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 ||
			!isImportedSelector(file, call.Fun, "sigs.k8s.io/controller-runtime", "NewManager") {
			return true
		}
		managerOptions, ok := call.Args[1].(*ast.CompositeLit)
		if !ok || !isImportedSelector(file, managerOptions.Type, "sigs.k8s.io/controller-runtime", "Options") {
			return true
		}
		metricsEntry := compositeFieldEntry(managerOptions, "Metrics")
		if metricsEntry == nil {
			return true
		}

		evidence := model.BehavioralEvidence{
			Kind:           conditionalMetricsKind,
			Status:         behaviorUnresolved,
			Identity:       "controller-runtime metrics",
			ServingSurface: "controller-runtime metrics serving surface",
			Source:         sourceRangeAt(file, metricsEntry.Key.Pos(), metricsEntry.Value.End()),
		}
		literal, condition, assignment, valid := managerMetricsProof(file, metricsEntry.Value)
		if !valid {
			evidence.Limitations = []string{
				"The controller-runtime manager Metrics binding does not use one direct lexical options object with a stable SecureServing condition",
			}
			result = append(result, evidence)
			return true
		}
		if !isImportedSelector(file, assignment.Rhs[0], metricsFiltersPackage, "WithAuthenticationAndAuthorization") {
			evidence.Limitations = []string{
				"FilterProvider uses an unsupported or dynamic provider; authentication and authorization behavior is unresolved",
			}
			result = append(result, evidence)
			return true
		}

		evidence.EnforcementProvider = expressionSource(file, assignment.Rhs[0])
		secureExpression := compositeFieldExpression(literal, "SecureServing")
		conditionText := expressionSource(file, condition)

		// Defaults and flag names enrich the stable configuration identity but do
		// not decide whether the branch itself was observed.
		_, boolFlags := staticFlagBindings(file)
		for name, binding := range repository.bools {
			boolFlags[name] = binding
		}
		_, secureFlag, secureResolved := resolveStaticBoolOption(secureExpression, boolFlags)
		evidence.ConfigurationBranch = conditionText + " is true"
		if secureResolved && secureFlag != "" {
			evidence.ConfigurationBranch += " (--" + secureFlag + ")"
		}
		evidence.Status = behaviorObserved
		result = append(result, evidence)
		return true
	})
	return result
}

func compositeFieldEntry(literal *ast.CompositeLit, field string) *ast.KeyValueExpr {
	for _, raw := range literal.Elts {
		entry, ok := raw.(*ast.KeyValueExpr)
		if ok && expressionIdentifier(entry.Key) == field {
			return entry
		}
	}
	return nil
}

func managerMetricsProof(
	file sourceFile,
	expression ast.Expr,
) (*ast.CompositeLit, ast.Expr, *ast.AssignStmt, bool) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil, nil, nil, false
	}
	function, ok := call.Fun.(*ast.FuncLit)
	if !ok || function.Body == nil || function.Type.Results == nil ||
		len(function.Type.Results.List) != 1 || !isMetricsServerOptions(file, function.Type.Results.List[0].Type) {
		return nil, nil, nil, false
	}

	var literal *ast.CompositeLit
	variable := ""
	declarationIndex := -1
	filterIndex := -1
	var condition ast.Expr
	var filterAssignment *ast.AssignStmt
	returnIndex := -1
	ambiguous := false
	for index, statement := range function.Body.List {
		switch typed := statement.(type) {
		case *ast.AssignStmt:
			if typed.Tok != token.DEFINE || len(typed.Lhs) != 1 || len(typed.Rhs) != 1 || literal != nil {
				continue
			}
			name, nameOK := typed.Lhs[0].(*ast.Ident)
			candidate, literalOK := typed.Rhs[0].(*ast.CompositeLit)
			if nameOK && literalOK && isMetricsServerOptions(file, candidate.Type) {
				literal, variable, declarationIndex = candidate, name.Name, index
			}
		case *ast.IfStmt:
			if typed.Init != nil {
				ambiguous = true
				continue
			}
			assignment := directFilterProviderAssignment(typed.Body, variable)
			if assignment != nil {
				if filterAssignment != nil {
					ambiguous = true
				} else {
					filterAssignment, condition, filterIndex = assignment, typed.Cond, index
				}
			}
		case *ast.ReturnStmt:
			if len(typed.Results) != 1 {
				ambiguous = true
				continue
			}
			identifier, identifierOK := typed.Results[0].(*ast.Ident)
			if !identifierOK || identifier.Name != variable || returnIndex >= 0 {
				ambiguous = true
				continue
			}
			returnIndex = index
		}
	}
	if ambiguous || literal == nil || declarationIndex < 0 || filterAssignment == nil ||
		filterIndex <= declarationIndex || returnIndex <= filterIndex ||
		!hasSingleMetricsReturn(function.Body, variable, filterAssignment.End()) {
		return nil, nil, nil, false
	}

	secure := compositeFieldExpression(literal, "SecureServing")
	if !sameStableConfigurationExpression(file, secure, condition) {
		return nil, nil, nil, false
	}
	for _, statement := range function.Body.List[declarationIndex+1 : filterIndex] {
		if mutatesMetricsDecisionState(statement, variable) || mutatesIdentifier(statement, rootIdentifier(secure)) {
			return nil, nil, nil, false
		}
	}
	for _, statement := range function.Body.List[filterIndex+1 : returnIndex] {
		if mutatesMetricsDecisionState(statement, variable) {
			return nil, nil, nil, false
		}
	}
	return literal, condition, filterAssignment, true
}

func hasSingleMetricsReturn(body *ast.BlockStmt, variable string, after token.Pos) bool {
	count := 0
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		count++
		if len(statement.Results) != 1 || statement.Pos() <= after {
			valid = false
			return false
		}
		identifier, ok := statement.Results[0].(*ast.Ident)
		if !ok || identifier.Name != variable {
			valid = false
		}
		return false
	})
	return valid && count == 1
}

func mutatesMetricsDecisionState(node ast.Node, variable string) bool {
	if variable == "" {
		return false
	}
	mutated := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		var targets []ast.Expr
		switch typed := candidate.(type) {
		case *ast.AssignStmt:
			targets = typed.Lhs
		case *ast.IncDecStmt:
			targets = []ast.Expr{typed.X}
		}
		for _, target := range targets {
			if identifier, ok := target.(*ast.Ident); ok && identifier.Name == variable {
				mutated = true
				return false
			}
			if field := rootSelectorField(target, variable); field == "FilterProvider" || field == "SecureServing" {
				mutated = true
				return false
			}
		}
		return !mutated
	})
	return mutated
}

func rootSelectorField(expression ast.Expr, root string) string {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return rootSelectorField(typed.X, root)
	case *ast.SelectorExpr:
		if identifier, ok := typed.X.(*ast.Ident); ok && identifier.Name == root {
			return typed.Sel.Name
		}
		return rootSelectorField(typed.X, root)
	case *ast.IndexExpr:
		return rootSelectorField(typed.X, root)
	default:
		return ""
	}
}

func directFilterProviderAssignment(body *ast.BlockStmt, variable string) *ast.AssignStmt {
	if body == nil || variable == "" {
		return nil
	}
	var result *ast.AssignStmt
	shadowed := false
	for _, statement := range body.List {
		if declaresIdentifier(statement, variable) {
			shadowed = true
			continue
		}
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		field, ok := assignment.Lhs[0].(*ast.SelectorExpr)
		identifier, identifierOK := field.X.(*ast.Ident)
		if ok && identifierOK && !shadowed && field.Sel.Name == "FilterProvider" && identifier.Name == variable {
			if result != nil {
				return nil
			}
			result = assignment
		}
	}
	return result
}

func declaresIdentifier(statement ast.Stmt, name string) bool {
	switch typed := statement.(type) {
	case *ast.AssignStmt:
		if typed.Tok != token.DEFINE {
			return false
		}
		for _, lhs := range typed.Lhs {
			if identifier, ok := lhs.(*ast.Ident); ok && identifier.Name == name {
				return true
			}
		}
	case *ast.DeclStmt:
		declaration, ok := typed.Decl.(*ast.GenDecl)
		if !ok || declaration.Tok != token.VAR {
			return false
		}
		for _, spec := range declaration.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, identifier := range value.Names {
				if identifier.Name == name {
					return true
				}
			}
		}
	}
	return false
}

func sameStableConfigurationExpression(file sourceFile, left, right ast.Expr) bool {
	return stableConfigurationExpression(left) && stableConfigurationExpression(right) &&
		expressionSource(file, left) == expressionSource(file, right)
}

func stableConfigurationExpression(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return stableConfigurationExpression(typed.X)
	case *ast.Ident:
		return typed.Name != "" && typed.Name != "true" && typed.Name != "false" && typed.Name != "nil"
	case *ast.SelectorExpr:
		return stableConfigurationExpression(typed.X)
	default:
		return false
	}
}

func rootIdentifier(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return rootIdentifier(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return rootIdentifier(typed.X)
	default:
		return ""
	}
}

func mutatesIdentifier(node ast.Node, name string) bool {
	if name == "" {
		return false
	}
	mutated := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		switch typed := candidate.(type) {
		case *ast.AssignStmt:
			for _, lhs := range typed.Lhs {
				if rootIdentifier(lhs) == name {
					mutated = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if rootIdentifier(typed.X) == name {
				mutated = true
				return false
			}
		}
		return !mutated
	})
	return mutated
}

func extractNamedWatchEvidence(
	file sourceFile,
	index repositoryGoIndex,
	apiPackages map[string]apiPackage,
) []model.BehavioralEvidence {
	var result []model.BehavioralEvidence
	for _, declaration := range file.file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		identity := qualifiedFunctionIdentity(file, function)
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Watches" || containsCall(selector.X, "NewWebhookManagedBy") {
				return true
			}
			packagePath, typeName := watchedType(call.Args[0], file)
			if typeName == "" {
				return true
			}

			withPredicates, predicateArguments := watchPredicateArguments(file, call.Args[1:])
			if !withPredicates {
				return true
			}
			evidence := model.BehavioralEvidence{
				Kind:       namedWatchKind,
				Status:     behaviorUnresolved,
				Identity:   identity,
				WatchedGVK: behaviorGVK(packagePath, typeName, file.modulePath, apiPackages),
				Source:     sourceRangeAt(file, selector.Sel.Pos(), call.End()),
			}
			literalNames, complete := literalNamedPredicates(file, predicateArguments)
			evidence.LiteralValues = literalNames
			if !complete || len(literalNames) == 0 {
				evidence.Limitations = []string{
					"Watch predicates use a dynamic value or unsupported wrapper; named-resource filtering is unresolved",
				}
				result = append(result, evidence)
				return true
			}

			target, targetPresent, targetResolved := watchEventTarget(file, index, apiPackages, call)
			evidence.EventTarget = target
			if !targetPresent || !targetResolved {
				evidence.Limitations = []string{
					"The named watch predicate is literal, but a direct supported event-handler target is absent, dynamic, or unresolved",
				}
				result = append(result, evidence)
				return true
			}
			evidence.Status = behaviorObserved
			result = append(result, evidence)
			return true
		})
	}
	return result
}

func watchPredicateArguments(file sourceFile, arguments []ast.Expr) (bool, []ast.Expr) {
	var result []ast.Expr
	found := false
	for _, argument := range arguments {
		call, ok := argument.(*ast.CallExpr)
		if !ok || calledFunctionName(call.Fun) != "WithPredicates" {
			continue
		}
		found = true
		if !isSupportedWithPredicates(file, call.Fun) {
			// Preserve the unsupported wrapper expression so the literal pass
			// fails closed and emits an explicit unresolved record.
			result = append(result, argument)
			continue
		}
		result = append(result, call.Args...)
	}
	return found, result
}

func isSupportedWithPredicates(file sourceFile, expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "WithPredicates" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	path := file.imports[identifier.Name]
	return path == "sigs.k8s.io/controller-runtime/pkg/builder" ||
		path == moduleHelperPath(file, "pkg/controller/reconciler")
}

func literalNamedPredicates(file sourceFile, expressions []ast.Expr) ([]string, bool) {
	var result []string
	for _, expression := range expressions {
		call, ok := expression.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return uniqueSortedStrings(result), false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "CreatedOrUpdatedOrDeletedNamed" {
			return uniqueSortedStrings(result), false
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok || file.imports[identifier.Name] != moduleHelperPath(file, "pkg/controller/predicates/resources") {
			return uniqueSortedStrings(result), false
		}
		value, ok := staticStringLiteral(call.Args[0])
		if !ok || value == "" {
			return uniqueSortedStrings(result), false
		}
		result = append(result, value)
	}
	return uniqueSortedStrings(result), true
}

func watchEventTarget(
	file sourceFile,
	index repositoryGoIndex,
	apiPackages map[string]apiPackage,
	watch *ast.CallExpr,
) (string, bool, bool) {
	toNamedCount := countCallsNamed(watch.Args[1:], "ToNamed")
	handlerCount := countCallsNamed(watch.Args[1:], "WithEventHandler")
	if toNamedCount != 1 || handlerCount != 1 {
		return "", toNamedCount > 0 || handlerCount > 0, false
	}
	present := false
	valid := false
	name := ""
	for _, argument := range watch.Args[1:] {
		wrapper, ok := argument.(*ast.CallExpr)
		if !ok || calledFunctionName(wrapper.Fun) != "WithEventHandler" {
			continue
		}
		if present {
			return "", true, false
		}
		present = true
		if len(wrapper.Args) != 1 || !isModuleHelperSelector(file, wrapper.Fun, "pkg/controller/reconciler", "WithEventHandler") {
			return "", true, false
		}
		toNamed, ok := wrapper.Args[0].(*ast.CallExpr)
		if !ok || len(toNamed.Args) != 1 ||
			!isModuleHelperSelector(file, toNamed.Fun, "pkg/controller/handlers", "ToNamed") {
			return "", true, false
		}
		name = resolveRepositoryString(index, file, toNamed.Args[0], map[string]bool{})
		if name == "" {
			return "", true, false
		}
		valid = true
	}
	if !present || !valid {
		return "", present, false
	}
	targetPackage, targetType := reconcilerTargetType(watch, file)
	if targetType == "" {
		return "", true, false
	}
	return behaviorGVK(targetPackage, targetType, file.modulePath, apiPackages) + "/" + name, true, true
}

func countCallsNamed(expressions []ast.Expr, name string) int {
	count := 0
	for _, expression := range expressions {
		ast.Inspect(expression, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && calledFunctionName(call.Fun) == name {
				count++
			}
			return true
		})
	}
	return count
}

func reconcilerTargetType(watch *ast.CallExpr, file sourceFile) (string, string) {
	selector, ok := watch.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Watches" {
		return "", ""
	}
	current := selector.X
	for {
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return "", ""
		}
		if len(call.Args) >= 2 && isSupportedReconcilerFor(file, call.Fun) {
			return watchedType(call.Args[1], file)
		}
		chained, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !supportedReconcilerChainMethod(chained.Sel.Name) {
			return "", ""
		}
		current = chained.X
	}
}

func supportedReconcilerChainMethod(name string) bool {
	switch name {
	case "Owns", "Watches", "WithAction":
		return true
	default:
		return false
	}
}

func isSupportedReconcilerFor(file sourceFile, expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "ReconcilerFor" {
		return false
	}
	return isModuleHelperSelector(file, expression, "pkg/controller/reconciler", "ReconcilerFor")
}

func moduleHelperPath(file sourceFile, relative string) string {
	return strings.TrimSuffix(file.modulePath, "/") + "/" + strings.TrimPrefix(relative, "/")
}

func isModuleHelperSelector(file sourceFile, expression ast.Expr, relative, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && file.imports[identifier.Name] == moduleHelperPath(file, relative)
}

func repositoryAPIPackages(files []sourceFile) map[string]apiPackage {
	result := map[string]apiPackage{}
	conflicts := map[string]bool{}
	for _, file := range files {
		key := packagePath(file)
		if conflicts[key] {
			continue
		}
		group, version := groupVersionLiteral(file.file)
		if group == "" && version == "" {
			continue
		}
		current := result[key]
		if (current.group != "" && group != "" && current.group != group) ||
			(current.version != "" && version != "" && current.version != version) {
			delete(result, key)
			conflicts[key] = true
			continue
		}
		if current.group == "" {
			current.group = group
		}
		if current.version == "" {
			current.version = version
		}
		result[key] = current
	}
	return result
}

func behaviorGVK(packagePathValue, typeName, modulePath string, apiPackages map[string]apiPackage) string {
	if metadata := apiPackages[packagePathValue]; metadata.group != "" && metadata.version != "" {
		return metadata.group + "/" + metadata.version + "/" + strings.TrimSuffix(typeName, "List")
	}
	return formatGVK(packagePathValue, typeName, modulePath)
}

func qualifiedFunctionIdentity(file sourceFile, function *ast.FuncDecl) string {
	receiver := receiverType(function)
	if receiver == "" {
		receiver = function.Name.Name
	}
	pkg := file.packageDir
	if pkg == "" {
		pkg = file.file.Name.Name
	}
	return pkg + "." + receiver
}

func sourceRangeAt(file sourceFile, start, end token.Pos) string {
	startLine := file.fileSet.Position(start).Line
	endLine := file.fileSet.Position(end).Line
	return file.path + ":" + lineRange(startLine, endLine)
}

func lineRange(start, end int) string {
	if end < start {
		end = start
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

func expressionSource(file sourceFile, expression ast.Expr) string {
	if expression == nil {
		return ""
	}
	var output bytes.Buffer
	if err := printer.Fprint(&output, file.fileSet, expression); err != nil {
		return ""
	}
	return strings.TrimSpace(output.String())
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func dedupeBehavioralEvidence(records []model.BehavioralEvidence) []model.BehavioralEvidence {
	seen := map[string]bool{}
	result := make([]model.BehavioralEvidence, 0, len(records))
	for _, record := range records {
		key := record.Kind + "\x00" + record.Identity + "\x00" + record.Source + "\x00" + strings.Join(record.LiteralValues, "\x00")
		if !seen[key] {
			seen[key] = true
			result = append(result, record)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].Identity != result[j].Identity {
			return result[i].Identity < result[j].Identity
		}
		return result[i].Source < result[j].Source
	})
	return result
}

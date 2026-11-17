package detector

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MissingIdentityInfo holds the results for a single resource.
type MissingIdentityInfo struct {
	// MissingCRUD lists which CRUD functions are missing resource identity population.
	MissingCRUD []string `json:"missingCRUD,omitempty"`
	// MissingImportTest is true if no step uses ImportStateKind: ImportBlockWithResourceIdentity.
	MissingImportTest bool `json:"missingImportTest"`
}

// DetectMissingIdentityCoverage scans the provided resource files.
// servicesDir may be either the root services directory containing <service>/
// subdirectories (e.g. <tpgb>/google-beta/services) or a flat directory
// containing resource_*.go files directly.
func DetectMissingIdentityCoverage(servicesDir string, changedResources []string) (map[string]*MissingIdentityInfo, error) {
	results := make(map[string]*MissingIdentityInfo)
	if len(changedResources) == 0 {
		return results, nil
	}

	resolver := newResourceFileResolver(servicesDir)

	for _, resourceName := range changedResources {
		resolved, err := resolver.resolve(resourceName)
		if err != nil {
			return nil, err
		}
		if resolved == nil {
			continue
		}

		hasIdentity, supportsImport, missingCRUD := analyzeResourceIdentity(resolved.file, resourceName)
		if !hasIdentity {
			continue
		}

		missingImportTest := false
		if supportsImport {
			missingImportTest, err = checkImportTest(resolved.path, resourceName)
			if err != nil {
				return nil, err
			}
		}

		if len(missingCRUD) > 0 || missingImportTest {
			results[resourceName] = &MissingIdentityInfo{
				MissingCRUD:       missingCRUD,
				MissingImportTest: missingImportTest,
			}
		}
	}
	return results, nil
}

type resolvedResourceFile struct {
	path string
	file *ast.File
}

type resourceFileResolver struct {
	servicesDir   string
	filesLoaded   bool
	filesByBase   map[string][]string
	indexBuilt    bool
	registryIndex map[string]string
}

func newResourceFileResolver(servicesDir string) *resourceFileResolver {
	return &resourceFileResolver{
		servicesDir: servicesDir,
	}
}

func (r *resourceFileResolver) loadFilesByBase() (map[string][]string, error) {
	if r.filesLoaded {
		return r.filesByBase, nil
	}
	r.filesLoaded = true
	r.filesByBase = make(map[string][]string)

	topEntries, err := os.ReadDir(r.servicesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return r.filesByBase, nil
		}
		return nil, err
	}

	// Record direct .go files in servicesDir first (flat layout), then 1-level <service>/ subdirs.
	var subDirs []string
	for _, entry := range topEntries {
		if entry.IsDir() {
			subDirs = append(subDirs, entry.Name())
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasSuffix(name, "_sweeper.go") {
			r.filesByBase[name] = append(r.filesByBase[name], filepath.Join(r.servicesDir, name))
		}
	}

	for _, sub := range subDirs {
		subPath := filepath.Join(r.servicesDir, sub)
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			return nil, err
		}
		for _, entry := range subEntries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasSuffix(name, "_sweeper.go") {
				r.filesByBase[name] = append(r.filesByBase[name], filepath.Join(subPath, name))
			}
		}
	}

	return r.filesByBase, nil
}

func (r *resourceFileResolver) resolve(resourceName string) (*resolvedResourceFile, error) {
	candidates := candidateFilenames(resourceName)
	filesByBase, err := r.loadFilesByBase()
	if err != nil {
		return nil, err
	}

	// 1. Check candidate basenames in servicesDir (flat layout) and <servicesDir>/<service>/ (nested CI layout).
	for _, cand := range candidates {
		for _, path := range filesByBase[cand] {
			if res, ok, err := tryLoadMatchingResourceFile(path, resourceName); err != nil {
				return nil, err
			} else if ok {
				return res, nil
			}
		}
	}

	// 2. Fallback: build an index from registry.Schema / registry.FrameworkResource
	// declarations to handle non-standard filenames (e.g. iam_bigquery_member_dataset.go,
	// workflow_template.go, resource_logging_exclusion.go).
	if err := r.buildRegistryIndex(); err != nil {
		return nil, err
	}
	for _, key := range []string{resourceName, "google_" + strings.TrimPrefix(resourceName, "google_")} {
		if indexedPath, ok := r.registryIndex[key]; ok {
			return parseGoFile(indexedPath)
		}
	}

	return nil, nil
}

func candidateFilenames(resourceName string) []string {
	shortName := strings.TrimPrefix(resourceName, "google_")
	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}

	add("resource_" + shortName + ".go")
	add("resource_" + resourceName + ".go")
	add("resource_" + shortName + "_resource.go")
	add("fw_resource_" + shortName + ".go")
	add("fw_resource_" + resourceName + ".go")

	if iamBase, ok := trimIAMResourceSuffix(shortName); ok {
		add("iam_" + iamBase + ".go")
	}
	if iamBase, ok := trimIAMResourceSuffix(resourceName); ok {
		add("iam_" + iamBase + ".go")
	}
	return out
}

func trimIAMResourceSuffix(name string) (string, bool) {
	for _, suffix := range []string{"_iam_member", "_iam_binding", "_iam_policy", "_iam_audit_config"} {
		if base, ok := strings.CutSuffix(name, suffix); ok && base != "" {
			return base, true
		}
	}
	return "", false
}

func tryLoadMatchingResourceFile(path string, resourceName string) (*resolvedResourceFile, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	res, err := parseGoFile(path)
	if err != nil {
		return nil, false, err
	}
	registered := registeredResourceNames(res.file)
	if len(registered) > 0 {
		for _, regName := range registered {
			if resourceNamesMatch(regName, resourceName) {
				return res, true, nil
			}
		}
		// File explicitly registers other resource(s), not resourceName.
		return nil, false, nil
	}
	return res, true, nil
}

func parseGoFile(path string) (*resolvedResourceFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	return &resolvedResourceFile{
		path: path,
		file: file,
	}, nil
}

func (r *resourceFileResolver) buildRegistryIndex() error {
	if r.indexBuilt {
		return nil
	}
	r.indexBuilt = true
	r.registryIndex = make(map[string]string)

	filesByBase, err := r.loadFilesByBase()
	if err != nil {
		return err
	}

	for _, paths := range filesByBase {
		for _, fullPath := range paths {
			src, err := os.ReadFile(fullPath)
			if err != nil {
				return err
			}
			if !bytes.Contains(src, []byte("registry.")) {
				continue
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, fullPath, src, 0)
			if err != nil {
				continue
			}
			for _, regName := range registeredResourceNames(file) {
				r.registryIndex[regName] = fullPath
			}
		}
	}
	return nil
}

type registryEntry struct {
	name       string
	kind       string // "SchemaTypeResource", "SchemaTypeIAMResource", "FrameworkResource"
	schemaExpr ast.Expr
	funcExpr   ast.Expr
}

func extractRegistryEntries(file *ast.File) []registryEntry {
	var entries []registryEntry
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := cl.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "registry" {
			return true
		}
		typeName := sel.Sel.Name
		if typeName != "Schema" && typeName != "FrameworkResource" {
			return true
		}

		var resName, schemaType string
		var schemaExpr, funcExpr ast.Expr
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch keyIdent.Name {
			case "Name":
				if s, ok := stringLitValue(kv.Value); ok {
					resName = s
				}
			case "Type":
				switch v := kv.Value.(type) {
				case *ast.SelectorExpr:
					schemaType = v.Sel.Name
				case *ast.Ident:
					schemaType = v.Name
				}
			case "Schema":
				schemaExpr = kv.Value
			case "Func":
				funcExpr = kv.Value
			}
		}

		if resName == "" {
			return true
		}
		if typeName == "FrameworkResource" {
			entries = append(entries, registryEntry{
				name:     resName,
				kind:     "FrameworkResource",
				funcExpr: funcExpr,
			})
		} else if schemaType == "SchemaTypeResource" || schemaType == "SchemaTypeIAMResource" {
			entries = append(entries, registryEntry{
				name:       resName,
				kind:       schemaType,
				schemaExpr: schemaExpr,
			})
		}
		return true
	})
	return entries
}

func registeredResourceNames(file *ast.File) []string {
	entries := extractRegistryEntries(file)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.name)
	}
	return names
}

func resourceNamesMatch(a, b string) bool {
	if a == b {
		return true
	}
	shortA := strings.TrimPrefix(a, "google_")
	shortB := strings.TrimPrefix(b, "google_")
	return shortA != "" && shortA == shortB
}

func stringLitValue(expr ast.Expr) (string, bool) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	val, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return val, true
}

func analyzeResourceIdentity(file *ast.File, resourceName string) (hasIdentity bool, supportsImport bool, missingCRUD []string) {
	topFuncs := make(map[string]*ast.FuncDecl)
	methodsByRecv := make(map[string]map[string]*ast.FuncDecl)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv == nil {
			topFuncs[fn.Name.Name] = fn
		} else if recvType := receiverTypeName(fn.Recv); recvType != "" {
			if methodsByRecv[recvType] == nil {
				methodsByRecv[recvType] = make(map[string]*ast.FuncDecl)
			}
			methodsByRecv[recvType][fn.Name.Name] = fn
		}
	}

	regEntries := extractRegistryEntries(file)
	var matchedReg *registryEntry
	for i := range regEntries {
		if resourceNamesMatch(regEntries[i].name, resourceName) {
			matchedReg = &regEntries[i]
			break
		}
	}

	// 1. IAM resources backed by tpgiamresource.ResourceIam*.
	if matchedReg != nil && matchedReg.kind == "SchemaTypeIAMResource" {
		hasId, supImp := analyzeIAMSchemaExpr(matchedReg.schemaExpr, topFuncs)
		return hasId, supImp, nil
	}
	if matchedReg == nil {
		if hasIAMCall, hasId, supImp := analyzeUnregisteredIAMFile(file, topFuncs); hasIAMCall {
			return hasId, supImp, nil
		}
	}

	// 2. Plugin Framework resources implementing IdentitySchema / ResourceWithIdentity.
	if fwRecv := findFrameworkIdentityReceiver(file, matchedReg, topFuncs, methodsByRecv); fwRecv != "" {
		supImp, missing := checkFrameworkCRUDCoverage(methodsByRecv[fwRecv], topFuncs)
		return true, supImp, missing
	}
	if matchedReg != nil && matchedReg.kind == "FrameworkResource" {
		return false, false, nil
	}

	// 3. SDKv2 resources (&schema.Resource{ Identity: &schema.ResourceIdentity{...} }).
	return analyzeSDKv2Resource(file, matchedReg, topFuncs)
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func analyzeIAMSchemaExpr(expr ast.Expr, topFuncs map[string]*ast.FuncDecl) (hasIdentity bool, supportsImport bool) {
	call := findIAMResourceCall(expr, topFuncs)
	if call == nil {
		return false, false
	}
	return inspectIAMResourceCall(call)
}

func analyzeUnregisteredIAMFile(file *ast.File, topFuncs map[string]*ast.FuncDecl) (isIAM bool, hasIdentity bool, supportsImport bool) {
	var foundCall *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if foundCall != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isIAMResourceConstructorCall(call) {
			foundCall = call
			return false
		}
		return true
	})
	if foundCall == nil {
		return false, false, false
	}
	hasId, supImp := inspectIAMResourceCall(foundCall)
	return true, hasId, supImp
}

func findIAMResourceCall(expr ast.Expr, topFuncs map[string]*ast.FuncDecl) *ast.CallExpr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if isIAMResourceConstructorCall(call) {
		return call
	}
	if ident, ok := call.Fun.(*ast.Ident); ok {
		if fn, exists := topFuncs[ident.Name]; exists && fn.Body != nil {
			var inner *ast.CallExpr
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if inner != nil {
					return false
				}
				if c, ok := n.(*ast.CallExpr); ok && isIAMResourceConstructorCall(c) {
					inner = c
					return false
				}
				return true
			})
			return inner
		}
	}
	return nil
}

func isIAMResourceConstructorCall(call *ast.CallExpr) bool {
	var fnName string
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fnName = fun.Sel.Name
	case *ast.Ident:
		fnName = fun.Name
	}
	switch fnName {
	case "ResourceIamMember", "ResourceIamBinding", "ResourceIamPolicy", "ResourceIamAuditConfig":
		return true
	}
	return false
}

func inspectIAMResourceCall(call *ast.CallExpr) (hasIdentity bool, supportsImport bool) {
	supportsImport = true
	if len(call.Args) >= 3 {
		if ident, ok := call.Args[2].(*ast.Ident); ok && ident.Name == "nil" {
			supportsImport = false
		}
	}
	for _, arg := range call.Args {
		ast.Inspect(arg, func(n ast.Node) bool {
			if hasIdentity {
				return false
			}
			switch v := n.(type) {
			case *ast.Ident:
				if v.Name == "IamWithParentResourceIdentity" {
					hasIdentity = true
					return false
				}
			case *ast.SelectorExpr:
				if v.Sel.Name == "IamWithParentResourceIdentity" {
					hasIdentity = true
					return false
				}
			}
			return true
		})
	}
	return hasIdentity, supportsImport
}

func findFrameworkIdentityReceiver(file *ast.File, matchedReg *registryEntry, topFuncs map[string]*ast.FuncDecl, methodsByRecv map[string]map[string]*ast.FuncDecl) string {
	var targetRecv string
	if matchedReg != nil && matchedReg.kind == "FrameworkResource" {
		if ident, ok := matchedReg.funcExpr.(*ast.Ident); ok {
			if fn, exists := topFuncs[ident.Name]; exists && fn.Body != nil {
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if cl, ok := n.(*ast.CompositeLit); ok {
						if tIdent, ok := cl.Type.(*ast.Ident); ok {
							targetRecv = tIdent.Name
							return false
						}
					}
					return true
				})
			}
		}
	}

	recvWithIdentity := make(map[string]bool)
	for recv, methods := range methodsByRecv {
		if _, ok := methods["IdentitySchema"]; ok {
			recvWithIdentity[recv] = true
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			return true
		}
		isResWithId := false
		switch t := vs.Type.(type) {
		case *ast.SelectorExpr:
			isResWithId = t.Sel.Name == "ResourceWithIdentity"
		case *ast.Ident:
			isResWithId = t.Name == "ResourceWithIdentity"
		}
		if !isResWithId {
			return true
		}
		for _, val := range vs.Values {
			ast.Inspect(val, func(inner ast.Node) bool {
				if cl, ok := inner.(*ast.CompositeLit); ok {
					if ident, ok := cl.Type.(*ast.Ident); ok {
						recvWithIdentity[ident.Name] = true
					}
				}
				return true
			})
		}
		return true
	})

	if targetRecv != "" {
		if recvWithIdentity[targetRecv] {
			return targetRecv
		}
		return ""
	}
	for recv := range recvWithIdentity {
		return recv
	}
	return ""
}

func checkFrameworkCRUDCoverage(methods map[string]*ast.FuncDecl, topFuncs map[string]*ast.FuncDecl) (supportsImport bool, missing []string) {
	supportsImport = true
	_, hasCreate := methods["Create"]
	_, hasRead := methods["Read"]
	if hasCreate || hasRead {
		_, supportsImport = methods["ImportState"]
	}

	for _, op := range []string{"Create", "Read", "Update"} {
		fn, ok := methods[op]
		if !ok || fn.Body == nil {
			continue
		}
		if op == "Update" && isFrameworkImmutableUpdateStub(fn, methods, topFuncs) {
			continue
		}
		if !frameworkFuncSetsIdentity(fn, methods, topFuncs, make(map[string]bool)) {
			missing = append(missing, op)
		}
	}
	return supportsImport, missing
}

func isFrameworkImmutableUpdateStub(fn *ast.FuncDecl, methods map[string]*ast.FuncDecl, topFuncs map[string]*ast.FuncDecl) bool {
	if len(fn.Body.List) == 0 {
		return true
	}
	if frameworkFuncSetsState(fn, methods, topFuncs, make(map[string]bool)) || frameworkFuncSetsIdentity(fn, methods, topFuncs, make(map[string]bool)) {
		return false
	}
	return bodyCallsAddError(fn.Body)
}

func bodyCallsAddError(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "AddError" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func frameworkFuncSetsState(fn *ast.FuncDecl, methods map[string]*ast.FuncDecl, topFuncs map[string]*ast.FuncDecl, visited map[string]bool) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	key := receiverTypeName(fn.Recv) + "." + fn.Name.Name
	if visited[key] {
		return false
	}
	visited[key] = true

	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isFrameworkStateSetCall(call) {
			found = true
			return false
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if m, exists := methods[fun.Sel.Name]; exists && frameworkFuncSetsState(m, methods, topFuncs, visited) {
				found = true
				return false
			}
		case *ast.Ident:
			if f, exists := topFuncs[fun.Name]; exists && frameworkFuncSetsState(f, methods, topFuncs, visited) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func isFrameworkStateSetCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Set" && sel.Sel.Name != "SetAttribute") {
		return false
	}
	switch x := sel.X.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "State"
	case *ast.Ident:
		return x.Name == "state"
	}
	return false
}

func frameworkFuncSetsIdentity(fn *ast.FuncDecl, methods map[string]*ast.FuncDecl, topFuncs map[string]*ast.FuncDecl, visited map[string]bool) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	key := receiverTypeName(fn.Recv) + "." + fn.Name.Name
	if visited[key] {
		return false
	}
	visited[key] = true

	if bodyDirectlySetsIdentity(fn.Body, fn.Type) {
		return true
	}

	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if m, exists := methods[fun.Sel.Name]; exists && frameworkFuncSetsIdentity(m, methods, topFuncs, visited) {
				found = true
				return false
			}
		case *ast.Ident:
			if f, exists := topFuncs[fun.Name]; exists && frameworkFuncSetsIdentity(f, methods, topFuncs, visited) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func analyzeSDKv2Resource(file *ast.File, matchedReg *registryEntry, topFuncs map[string]*ast.FuncDecl) (hasIdentity bool, supportsImport bool, missingCRUD []string) {
	var schemaFunc *ast.FuncDecl
	if matchedReg != nil && matchedReg.kind == "SchemaTypeResource" {
		if call, ok := matchedReg.schemaExpr.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				schemaFunc = topFuncs[ident.Name]
			}
		}
	}

	var resLit *ast.CompositeLit
	hasIdentityAssign := false

	inspectTarget := ast.Node(file)
	if schemaFunc != nil && schemaFunc.Body != nil {
		inspectTarget = schemaFunc.Body
	}

	// Locate the top-level &schema.Resource{...} literal (not nested Elem: &schema.Resource{...}).
	ast.Inspect(inspectTarget, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if keyIdent, ok := node.Key.(*ast.Ident); ok && keyIdent.Name == "Elem" {
				return false
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Identity" {
					if i < len(node.Rhs) && !isNilIdent(node.Rhs[i]) {
						hasIdentityAssign = true
					}
				}
			}
		case *ast.CompositeLit:
			if isSchemaResourceCompositeLit(node) {
				if resLit == nil || compositeLitHasIdentityField(node) {
					resLit = node
				}
			}
		}
		return true
	})

	if resLit == nil {
		return false, false, nil
	}

	hasIdentity = hasIdentityAssign || compositeLitHasIdentityField(resLit)
	if !hasIdentity {
		return false, false, nil
	}

	crudFuncs := make(map[string]string)
	crudInline := make(map[string]*ast.FuncLit)
	hasImporterField := false

	for _, elt := range resLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "Create", "CreateContext", "CreateWithoutTimeout":
			recordSDKCRUDHandler("Create", kv.Value, crudFuncs, crudInline)
		case "Read", "ReadContext", "ReadWithoutTimeout":
			recordSDKCRUDHandler("Read", kv.Value, crudFuncs, crudInline)
		case "Update", "UpdateContext", "UpdateWithoutTimeout":
			recordSDKCRUDHandler("Update", kv.Value, crudFuncs, crudInline)
		case "Importer":
			if !isNilIdent(kv.Value) {
				hasImporterField = true
			}
		}
	}

	if len(crudFuncs) > 0 || len(crudInline) > 0 {
		supportsImport = hasImporterField
	} else {
		// Fallback for minimal synthetic unit tests where &schema.Resource{Identity: ...}
		// omits Create/Read/Update/Importer fields.
		supportsImport = true
		for name := range topFuncs {
			if op := sdkFallbackCRUDType(name); op != "" {
				crudFuncs[op] = name
			}
		}
	}

	readFnName := crudFuncs["Read"]
	for _, op := range []string{"Create", "Read", "Update"} {
		if fnLit, ok := crudInline[op]; ok {
			if !sdkBlockCoversIdentity(fnLit.Body, fnLit.Type, topFuncs, readFnName, make(map[string]bool)) {
				missingCRUD = append(missingCRUD, op)
			}
			continue
		}
		fnName, ok := crudFuncs[op]
		if !ok {
			continue
		}
		fn := topFuncs[fnName]
		if fn == nil || fn.Body == nil {
			continue
		}
		if !sdkFuncCoversIdentity(fnName, topFuncs, readFnName, make(map[string]bool)) {
			missingCRUD = append(missingCRUD, op)
		}
	}

	return true, supportsImport, missingCRUD
}

func isSchemaResourceCompositeLit(cl *ast.CompositeLit) bool {
	switch t := cl.Type.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name == "Resource"
	case *ast.Ident:
		return t.Name == "Resource"
	}
	return false
}

func compositeLitHasIdentityField(cl *ast.CompositeLit) bool {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if keyIdent, ok := kv.Key.(*ast.Ident); ok && keyIdent.Name == "Identity" && !isNilIdent(kv.Value) {
			return true
		}
	}
	return false
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func recordSDKCRUDHandler(op string, val ast.Expr, crudFuncs map[string]string, crudInline map[string]*ast.FuncLit) {
	switch v := val.(type) {
	case *ast.Ident:
		if v.Name != "nil" {
			crudFuncs[op] = v.Name
		}
	case *ast.FuncLit:
		crudInline[op] = v
	}
}

func sdkFallbackCRUDType(funcName string) string {
	for _, ignorePrefix := range []string{"expand", "flatten", "test", "Test"} {
		if strings.HasPrefix(funcName, ignorePrefix) {
			return ""
		}
	}
	trimmed := funcName
	for _, suffix := range []string{"WithoutTimeout", "Context"} {
		if strings.HasSuffix(trimmed, suffix) {
			trimmed = strings.TrimSuffix(trimmed, suffix)
			break
		}
	}
	if strings.HasSuffix(trimmed, "PollRead") {
		return ""
	}
	switch {
	case strings.HasSuffix(trimmed, "Create"):
		return "Create"
	case strings.HasSuffix(trimmed, "Read"):
		return "Read"
	case strings.HasSuffix(trimmed, "Update"):
		return "Update"
	default:
		return ""
	}
}

func sdkFuncCoversIdentity(fnName string, topFuncs map[string]*ast.FuncDecl, readFnName string, visited map[string]bool) bool {
	if visited[fnName] {
		return false
	}
	visited[fnName] = true

	fn := topFuncs[fnName]
	if fn == nil || fn.Body == nil {
		return false
	}
	return sdkBlockCoversIdentity(fn.Body, fn.Type, topFuncs, readFnName, visited)
}

func sdkBlockCoversIdentity(body *ast.BlockStmt, fnType *ast.FuncType, topFuncs map[string]*ast.FuncDecl, readFnName string, visited map[string]bool) bool {
	if body == nil {
		return false
	}
	if bodyDirectlySetsIdentity(body, fnType) {
		return true
	}

	// 1. Check calls to non-Read helpers (e.g. Flatten..., set...Identity) anywhere in body.
	helperCovered := false
	ast.Inspect(body, func(n ast.Node) bool {
		if helperCovered {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name != readFnName {
			if _, exists := topFuncs[ident.Name]; exists {
				// Helper sets identity without relying on Read, OR helper is a shared Set
				// function called via top-level delegation (handled below).
				if sdkHelperDirectlyOrTransitivelySetsIdentity(ident.Name, topFuncs, readFnName, copyVisited(visited)) {
					helperCovered = true
					return false
				}
			}
		}
		return true
	})
	if helperCovered {
		return true
	}

	// 2. Check unconditional top-level delegation to Read (or a shared Set helper that delegates
	// to Read), such as `return resourceFooRead(d, meta)` at the end of Create/Update.
	// Ignoring conditional early exits like `if clientSideOnly { return resourceFooRead(d, meta) }`
	// ensures a CustomUpdate that forgets identity on its main path is still flagged.
	for _, stmt := range body.List {
		for _, callee := range topLevelCalledIdents(stmt) {
			if _, exists := topFuncs[callee]; exists {
				if sdkFuncCoversIdentity(callee, topFuncs, readFnName, copyVisited(visited)) {
					return true
				}
			}
		}
	}

	return false
}

func sdkHelperDirectlyOrTransitivelySetsIdentity(fnName string, topFuncs map[string]*ast.FuncDecl, readFnName string, visited map[string]bool) bool {
	if fnName == readFnName || visited[fnName] {
		return false
	}
	visited[fnName] = true
	fn := topFuncs[fnName]
	if fn == nil || fn.Body == nil {
		return false
	}
	if bodyDirectlySetsIdentity(fn.Body, fn.Type) {
		return true
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name != readFnName {
			if _, exists := topFuncs[ident.Name]; exists {
				if sdkHelperDirectlyOrTransitivelySetsIdentity(ident.Name, topFuncs, readFnName, visited) {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func topLevelCalledIdents(stmt ast.Stmt) []string {
	var idents []string
	collectFromExpr := func(expr ast.Expr) {
		if call, ok := expr.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				idents = append(idents, ident.Name)
			}
		}
	}
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		for _, res := range s.Results {
			collectFromExpr(res)
		}
	case *ast.ExprStmt:
		collectFromExpr(s.X)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			collectFromExpr(rhs)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			idents = append(idents, topLevelCalledIdents(s.Init)...)
		}
	}
	return idents
}

func copyVisited(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func bodyDirectlySetsIdentity(body *ast.BlockStmt, fnType *ast.FuncType) bool {
	identityVars := map[string]bool{
		"identity": true,
	}
	if fnType != nil && fnType.Params != nil {
		for _, field := range fnType.Params.List {
			if isIdentityParamType(field.Type) {
				for _, name := range field.Names {
					identityVars[name.Name] = true
				}
			}
		}
	}

	// First pass: collect local variables assigned from `<x>.Identity()`.
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			return true
		}
		if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Identity" && len(call.Args) == 0 {
				if ident, ok := assign.Lhs[0].(*ast.Ident); ok && ident.Name != "_" {
					identityVars[ident.Name] = true
				}
			}
		}
		return true
	})

	// Second pass: check for identity-setting calls.
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isIdentitySetCall(call, identityVars) {
			found = true
			return false
		}
		return true
	})
	return found
}

func isIdentityParamType(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name == "IdentityData" || t.Sel.Name == "ResourceIdentity"
	case *ast.Ident:
		return t.Name == "IdentityData" || t.Name == "ResourceIdentity"
	}
	return false
}

func isIdentitySetCall(call *ast.CallExpr, identityVars map[string]bool) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "SetResourceIdentityAttributes"
	case *ast.SelectorExpr:
		if fun.Sel.Name == "SetResourceIdentityAttributes" {
			return true
		}
		if fun.Sel.Name != "Set" && fun.Sel.Name != "SetAttribute" {
			return false
		}
		switch recv := fun.X.(type) {
		case *ast.Ident:
			return identityVars[recv.Name]
		case *ast.SelectorExpr:
			return recv.Sel.Name == "Identity"
		}
	}
	return false
}

func checkImportTest(resourcePath string, resourceName string) (bool, error) {
	dir := filepath.Dir(resourcePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, fmt.Errorf("error reading directory %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return true, err
		}
		if !bytes.Contains(content, []byte("ImportBlockWithResourceIdentity")) {
			continue
		}
		isDedicatedFile := isDedicatedTestFileForResource(entry.Name(), resourcePath, resourceName)
		if hasValidIdentityImportStepForResource(content, resourceName, isDedicatedFile) {
			return false, nil
		}
	}
	return true, nil
}

func isDedicatedTestFileForResource(testFileName, resourcePath, resourceName string) bool {
	shortName := strings.TrimPrefix(resourceName, "google_")
	prefixes := []string{
		"resource_" + shortName,
		"resource_" + resourceName,
		"fw_resource_" + shortName,
		"fw_resource_" + resourceName,
	}
	baseFile := filepath.Base(resourcePath)
	if !strings.HasPrefix(baseFile, "iam_") {
		prefixes = append(prefixes, strings.TrimSuffix(baseFile, ".go"))
	}
	for _, p := range prefixes {
		if testFileName == p+"_test.go" || testFileName == p+"_generated_test.go" || testFileName == p+"_internal_test.go" {
			return true
		}
	}
	return false
}

func hasValidIdentityImportStepForResource(content []byte, resourceName string, isDedicatedFile bool) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, 0)
	if err != nil {
		return false
	}

	fileMentionsResAddr := fileContainsResourceAddressLiteral(file, resourceName)

	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		hasIdentityKind := false
		hasImportState := false
		hasRefreshState := false
		var resourceNameExpr ast.Expr

		for _, elt := range compLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch keyIdent.Name {
			case "ImportStateKind":
				switch v := kv.Value.(type) {
				case *ast.SelectorExpr:
					if v.Sel.Name == "ImportBlockWithResourceIdentity" {
						hasIdentityKind = true
					}
				case *ast.Ident:
					if v.Name == "ImportBlockWithResourceIdentity" {
						hasIdentityKind = true
					}
				}
			case "ImportState":
				if ident, ok := kv.Value.(*ast.Ident); ok && ident.Name == "true" {
					hasImportState = true
				}
			case "RefreshState":
				if ident, ok := kv.Value.(*ast.Ident); ok && ident.Name == "true" {
					hasRefreshState = true
				}
			case "ResourceName":
				resourceNameExpr = kv.Value
			}
		}

		if !hasIdentityKind || !hasImportState || hasRefreshState {
			return true
		}

		if resourceNameExpr == nil {
			// Synthetic unit test step omitting ResourceName: match if in dedicated test file.
			if isDedicatedFile {
				found = true
				return false
			}
			return true
		}

		if addr, ok := stringLitValue(resourceNameExpr); ok {
			stepResType := strings.SplitN(addr, ".", 2)[0]
			if resourceNamesMatch(stepResType, resourceName) {
				found = true
				return false
			}
			return true
		}

		// Non-literal ResourceName expression (e.g. helper func taking `resourceName string`
		// in resource_google_project_iam_member_test.go): match if dedicated test file and
		// file explicitly references `<resourceName>.` in a string literal.
		if isDedicatedFile && fileMentionsResAddr {
			found = true
			return false
		}
		return true
	})

	return found
}

func fileContainsResourceAddressLiteral(file *ast.File, resourceName string) bool {
	shortName := strings.TrimPrefix(resourceName, "google_")
	fullName := "google_" + shortName
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if s, err := strconv.Unquote(bl.Value); err == nil {
				if strings.HasPrefix(s, fullName+".") || strings.HasPrefix(s, shortName+".") {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}
